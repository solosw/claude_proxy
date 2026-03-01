package oldhandler

import (
	"awesomeProject/internal/combo"
	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
	"awesomeProject/internal/modelstate"
	"awesomeProject/internal/translator/messages"

	"awesomeProject/pkg/utils"
	"bufio"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type MessagesHandler struct {
	cfg *appconfig.Config
}

func NewMessagesHandler(cfg *appconfig.Config) *MessagesHandler {
	return &MessagesHandler{cfg: cfg}
}

// RegisterRoutes 在给定路由组上注册 /v1/messages（完整路径 = 组前缀 + /v1/messages，如 /back/v1/messages）。
func (h *MessagesHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/gpt/v1/messages", h.handleMessages)
	r.POST("/gpt/v1/messages/count_tokens", h.handleCountTokens)
	r.OPTIONS("/gpt/v1/messages", h.handleOptions)
	r.OPTIONS("/gpt/v1/messages/count_tokens", h.handleOptions)
}

func (h *MessagesHandler) handleOptions(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// anthropicError 按 Claude API 文档返回错误体：{"type":"error","error":{"type":"...","message":"..."}}
func anthropicError(c *gin.Context, status int, errorType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errorType,
			"message": message,
		},
	})
}

// extractUpstreamErrorMessage 从上游错误 body 中解析出 message，用于日志与返回。
func extractUpstreamErrorMessage(body []byte) string {
	message := "Upstream request failed"
	if len(body) == 0 {
		return message
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		if len(body) <= 500 {
			return string(body)
		}
		return message
	}
	if errObj, _ := m["error"].(map[string]any); errObj != nil {
		if msg, _ := errObj["message"].(string); msg != "" {
			return msg
		}
	}
	if errObj, _ := m["errors"].(map[string]any); errObj != nil {
		if msg, _ := errObj["message"].(string); msg != "" {
			return msg
		}
	}
	if msg, _ := m["message"].(string); msg != "" {
		return msg
	}
	if msg, _ := m["error"].(string); msg != "" {
		return msg
	}
	return message
}

// anthropicErrorFromBody 根据上游状态码和 body 按 Claude 格式返回错误（对话过程中上游 4xx/5xx 时使用）。
func anthropicErrorFromBody(c *gin.Context, statusCode int, body []byte) {
	message := extractUpstreamErrorMessage(body)
	errorType := "api_error"
	if statusCode >= 400 && statusCode < 500 {
		errorType = "invalid_request_error"
	}
	if statusCode == 404 {
		errorType = "not_found_error"
	}
	if statusCode == 429 {
		errorType = "rate_limit_error"
	}
	anthropicError(c, statusCode, errorType, message)
}

func (h *MessagesHandler) handleMessages(c *gin.Context) {
	// 每次请求打一条日志，便于确认「重启后持续请求」来自客户端自动重试（如 Claude Code/Cursor）；关闭对应对话会话即可停止重试。
	ua := c.GetHeader("User-Agent")
	if len(ua) > 80 {
		ua = ua[:80] + "..."
	}
	utils.Logger.Printf("[ClaudeRouter] messages: request path=%s remote=%s user_agent=%s", c.Request.URL.Path, c.ClientIP(), ua)
	raw, err := c.GetRawData()
	if err != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: step=read_body err=%v", err)
		anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read body")
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: step=parse_json err=%v", err)
		anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}

	requestedModel, _ := payload["model"].(string)
	requestedModel = strings.TrimSpace(requestedModel)

	conversationID := extractMetadataUserID(payload)
	var cachedModelID string
	//conversationID = "" //禁用缓存
	if conversationID != "" {
		if modelID, ok := modelstate.GetConversationModel(conversationID); ok {
			cachedModelID = modelID
			requestedModel = cachedModelID
			c.Set("real_conversation_id", conversationID)
			utils.Logger.Printf("[ClaudeRouter] messages: step=conversation_model cached user_id=%s model=%s", conversationID, requestedModel)

		}

	}

	if requestedModel == "" {
		utils.Logger.Printf("[ClaudeRouter] messages: step=validate missing model")
		anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Missing model")
		return
	}

	stream := false
	if v, ok := payload["stream"].(bool); ok {
		stream = v
	}
	utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model requested=%s stream=%v", requestedModel, stream)

	inputText := extractAnthropicInputText(payload)
	var targetModel *model.Model

	if cachedModelID != "" {
		// 同一对话后续请求：直接用缓存的模型 id 取模型，不再查 combo
		m, err := model.GetModel(requestedModel)
		if err != nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=cached_model_gone model=%s", requestedModel)
			anthropicError(c, http.StatusNotFound, "not_found_error", "Unknown model: "+requestedModel)

			return
		}
		if modelstate.IsModelTemporarilyDisabled(m.ID) {
			utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=model_temp_disabled model=%s", m.ID)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Model temporarily disabled: "+m.ID)
			return
		}
		targetModel = m
		utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model using_cached model=%s", targetModel.ID)
	} else {
		// 对话首条：requestedModel 必须是 combo id，智能选模型后缓存
		if !model.IsComboID(requestedModel) {
			utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=unknown_model model=%s", requestedModel)
			anthropicError(c, http.StatusNotFound, "not_found_error", "Unknown model: "+requestedModel)
			return
		}
		cb, cbErr := model.GetCombo(requestedModel)
		if cbErr != nil || cb == nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=combo_not_found model=%s", requestedModel)
			anthropicError(c, http.StatusNotFound, "not_found_error", "Unknown model: "+requestedModel)
			return
		}
		if !cb.Enabled {
			utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=combo_disabled model=%s", requestedModel)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Model disabled: "+requestedModel)
			return
		}

		// 过滤掉被禁用和临时禁用的模型
		filtered := make([]model.ComboItem, 0, len(cb.Items))
		for _, it := range cb.Items {
			modelID := strings.TrimSpace(it.ModelID)
			if modelID == "" || modelstate.IsModelTemporarilyDisabled(modelID) {
				continue
			}
			m, err := model.GetModel(modelID)
			if err != nil || m == nil || !m.Enabled || modelstate.IsModelTemporarilyDisabled(m.ID) {
				continue
			}
			filtered = append(filtered, it)
		}
		if len(filtered) == 0 {
			utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=no_available_models combo=%s", requestedModel)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Combo has no available models")
			return
		}

		// 使用过滤后的 combo 进行选择
		tmp := &model.Combo{ID: cb.ID, Name: cb.Name, Description: cb.Description, Enabled: cb.Enabled, Items: filtered}
		chosenID := combo.ChooseModelID(tmp, inputText)
		if chosenID == "" {
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Combo has no selectable items")
			return
		}
		m, err := model.GetModel(chosenID)
		if err != nil {
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Combo item model not found: "+chosenID)
			return
		}
		targetModel = m
		utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model combo chosen=%s", chosenID)
		if conversationID != "" {
			modelstate.SetConversationModel(conversationID, targetModel.ID)
			c.Set("real_conversation_id", conversationID)
		}
	}
	c.Set("real_model_id", targetModel.ID)
	if !targetModel.Enabled {
		anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Model disabled: "+targetModel.ID)
		return
	}
	if modelstate.IsModelTemporarilyDisabled(targetModel.ID) {
		utils.Logger.Printf("[ClaudeRouter] messages: step=resolve_model err=model_temp_disabled model=%s", targetModel.ID)
		anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Model temporarily disabled: "+targetModel.ID)
		return
	}

	upstreamID := strings.TrimSpace(targetModel.UpstreamID)
	if upstreamID == "" {
		upstreamID = requestedModel
	}

	interfaceType := strings.TrimSpace(targetModel.Interface)
	apiKey := strings.TrimSpace(targetModel.APIKey)
	baseURL := strings.TrimRight(strings.TrimSpace(targetModel.BaseURL), "/")

	// 若模型归属运营商，仅使用该运营商的转发逻辑；BaseURL/APIKey 优先用模型自身的，缺省时才用运营商配置
	if operatorID := strings.TrimSpace(targetModel.OperatorID); operatorID != "" {
		if h.cfg == nil || h.cfg.Operators == nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=operator err=no_config operator=%s", operatorID)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Operator config not available")
			return
		}
		ep, ok := h.cfg.Operators[operatorID]
		if !ok {
			utils.Logger.Printf("[ClaudeRouter] messages: step=operator err=not_found operator=%s", operatorID)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Operator not found: "+operatorID)
			return
		}
		if !ep.Enabled {
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Operator disabled: "+operatorID)
			return
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(ep.APIKey)
		}
		if baseURL == "" {
			baseURL = strings.TrimRight(strings.TrimSpace(ep.BaseURL), "/")
		}
		if t := strings.TrimSpace(ep.Interface); t != "" {
			interfaceType = t
		}
		utils.Logger.Printf("[ClaudeRouter] messages: step=operator using operator=%s (forwarding only, url/key from model when set)", operatorID)
	}
	if interfaceType == "" {
		interfaceType = "anthropic"
	}
	if baseURL == "" {
		switch interfaceType {
		case "openai", "openai_compatible":
			baseURL = "https://api.openai.com"
		case "openai_responses":
			baseURL = "https://api.openai.com"
		default:
			baseURL = "https://api.anthropic.com"
		}
	}

	// 按模型配置决定是否保留扩展字段（metadata、thinking），避免上游 422
	payloadToSend := applyForwardExtendedFields(payload, targetModel.ForwardMetadata, targetModel.ForwardThinking)

	// 功能1：去掉关键字，将 payload 中的 model 替换为实际的模型 ID
	// 这样上游收到的是真实模型 ID，而不是 combo ID
	if cachedModelID == "" && requestedModel != targetModel.ID {
		// 首次请求：requestedModel 是 combo ID，需要替换为实际的 targetModel.ID
		payloadToSend["model"] = targetModel.ID
		utils.Logger.Printf("[ClaudeRouter] messages: step=replace_model_in_payload from=%s to=%s", requestedModel, targetModel.ID)
	} else if cachedModelID != "" {
		// 后续请求：requestedModel 已经是缓存的模型 ID，保持不变
		payloadToSend["model"] = targetModel.ID
	}

	// 调试输出：Anthropic 转换后的消息
	if payloadJSON, err := json.Marshal(payloadToSend); err == nil {
		utils.Logger.Printf("[ClaudeRouter] messages: payload_to_send=%s", string(payloadJSON))
	} else {
		utils.Logger.Printf("[ClaudeRouter] messages: payload_to_send marshal err=%v", err)
	}

	// 按模型配置的 QPS 限流
	//waitModelQPS(c.Request.Context(), targetModel.ID, targetModel.MaxQPS)
	if c.Request.Context().Err() != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: client_gone during qps wait")
		return
	}

	opts := messages.ExecuteOptions{
		UpstreamModel: upstreamID,
		APIKey:        apiKey,
		BaseURL:       baseURL,
		Stream:        stream,
	}

	// 策略分发：有运营商则走该运营商的独立转发策略，否则走 interface_type 适配器（openai/anthropic）
	var statusCode int
	var contentType string
	var body []byte
	var streamBody io.ReadCloser

	operatorID := strings.TrimSpace(targetModel.OperatorID)
	if operatorID != "" {
		strategy := messages.OperatorRegistry.Get(operatorID)
		if strategy == nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=operator err=strategy_not_registered operator=%s", operatorID)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Operator strategy not registered: "+operatorID)
			return
		}
		if strings.EqualFold(operatorID, "codex") {
			utils.Logger.Printf("[ClaudeRouter] messages: step=execute_call operator=%s mode=sdk_translator_claude_to_codex", operatorID)
		} else {
			utils.Logger.Printf("[ClaudeRouter] messages: step=execute_call operator=%s", operatorID)
		}
		statusCode, contentType, body, streamBody, err = strategy.Execute(c.Request.Context(), payloadToSend, opts)
	} else {
		adapter := messages.Registry.GetOrDefault(interfaceType)
		if adapter == nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=adapter err=unsupported type=%s", interfaceType)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Unsupported interface_type: "+interfaceType)
			return
		}
		if strings.EqualFold(interfaceType, "openai_compatible") {
			utils.Logger.Printf("[ClaudeRouter] messages: step=execute_call adapter=%s mode=sdk_translator_claude_to_openai upstream_model=%s", interfaceType, upstreamID)
		} else {
			utils.Logger.Printf("[ClaudeRouter] messages: step=execute_call adapter=%s upstream_model=%s", interfaceType, upstreamID)
		}
		statusCode, contentType, body, streamBody, err = adapter.Execute(c.Request.Context(), payloadToSend, opts)
	}
	utils.Logger.Printf("[ClaudeRouter] messages: step=execute_done status=%d contentType=%s bodyLen=%d streamBody=%v err=%v", statusCode, contentType, len(body), streamBody != nil, err)

	if err != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: step=execute_err err=%v", err)

		// 功能2：模型报错时删除缓存
		if conversationID != "" {
			modelstate.ClearConversationModel(conversationID)
		}
		modelstate.DisableModelTemporarily(targetModel.ID, modelstate.TemporaryModelDisableTTL)

		if c.Request.Context().Err() != nil {
			utils.Logger.Printf("[ClaudeRouter] messages: client_gone, skip error response")
			return
		}
		if statusCode >= 400 {
			upstreamMsg := extractUpstreamErrorMessage(body)
			utils.Logger.Printf("[ClaudeRouter] messages: step=upstream_error status=%d message=%s", statusCode, upstreamMsg)
			anthropicErrorFromBody(c, statusCode, body)
			return
		}
		anthropicError(c, http.StatusBadGateway, "api_error", err.Error())
		return
	}

	if c.Request.Context().Err() != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: client_gone, skip response")
		if streamBody != nil {
			_ = streamBody.Close()
		}
		return
	}

	if statusCode < 200 || statusCode >= 300 {
		// 功能2：上游返回错误状态码时也删除缓存
		if conversationID != "" {
			modelstate.ClearConversationModel(conversationID)
		}
		modelstate.DisableModelTemporarily(targetModel.ID, modelstate.TemporaryModelDisableTTL)

		upstreamMsg := extractUpstreamErrorMessage(body)
		utils.Logger.Printf("[ClaudeRouter] messages: step=upstream_error status=%d message=%s", statusCode, upstreamMsg)
		anthropicErrorFromBody(c, statusCode, body)
		return
	}

	if stream && streamBody != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: step=stream_write interface_type=%s response_format=%s", interfaceType, targetModel.ResponseFormat)
		defer streamBody.Close()

		if strings.EqualFold(interfaceType, "openai_responses") && !strings.EqualFold(targetModel.ResponseFormat, "openai_responses") {
			utils.Logger.Printf("[ClaudeRouter] messages: converting OpenAI Responses stream to Anthropic")
			pr, pw := io.Pipe()
			go func() {
				defer pw.Close()
				ConvertOpenAIResponsesStreamToAnthropic(c.Request.Context(), streamBody, pw)
			}()
			c.Header("Content-Type", "text/event-stream")
			utils.ProxySSE(c, trackUsageStream(c, pr, targetModel.ID))
			return
		}
		if strings.EqualFold(targetModel.ResponseFormat, "openai_responses") {
			utils.Logger.Printf("[ClaudeRouter] messages: converting Anthropic stream to OpenAI Responses format")
			pr, pw := io.Pipe()
			go func() {
				defer pw.Close()
				ConvertAnthropicStreamToOpenAIResponses(c.Request.Context(), streamBody, pw)
			}()
			c.Header("Content-Type", "text/event-stream")
			utils.ProxySSE(c, trackUsageStream(c, pr, targetModel.ID))
			return
		}
		c.Header("Content-Type", "text/event-stream")
		utils.ProxySSE(c, trackUsageStream(c, streamBody, targetModel.ID))
		return
	}

	ct := contentType
	if ct == "" {
		ct = "application/json"
	}
	// 非流式：上游为 openai_responses 且未要求返回 OpenAI 格式时，将 OpenAI Responses JSON 转为 Anthropic
	if strings.EqualFold(interfaceType, "openai_responses") && !strings.EqualFold(targetModel.ResponseFormat, "openai_responses") {
		converted, convErr := ConvertOpenAIResponsesMessageToAnthropic(body)
		if convErr != nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=convert_openai_responses_to_anthropic err=%v", convErr)
			anthropicError(c, http.StatusInternalServerError, "api_error", "Failed to convert response format")
			return
		}
		body = converted
		ct = "application/json"
		utils.Logger.Printf("[ClaudeRouter] messages: step=write_response converted openai_responses->anthropic len=%d", len(body))
	} else if strings.EqualFold(targetModel.ResponseFormat, "openai_responses") {
		// 上游为 Anthropic，需返回 OpenAI Responses 格式
		converted, convErr := ConvertAnthropicMessageToOpenAIResponses(body)
		if convErr != nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=convert_anthropic_to_openai_responses err=%v", convErr)
			anthropicError(c, http.StatusInternalServerError, "api_error", "Failed to convert response format")
			return
		}
		body = converted
		ct = "application/json"
		utils.Logger.Printf("[ClaudeRouter] messages: step=write_response converted to openai_responses len=%d", len(body))
	}

	utils.Logger.Printf("[ClaudeRouter] messages: step=write_response status=%d len=%d", statusCode, len(body))
	recordUsageFromBodyWithModel(c, body, targetModel.ID)
	c.Data(statusCode, ct, body)
}

// handleCountTokens 实现 Anthropic 兼容的 POST /v1/messages/count_tokens（桩实现）。
func (h *MessagesHandler) handleCountTokens(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"input_tokens": 0})
}

// applyForwardExtendedFields 根据模型配置返回一份 payload 副本，未开启转发的扩展字段（metadata、thinking）会被移除，避免上游 422。
func applyForwardExtendedFields(payload map[string]any, forwardMetadata, forwardThinking bool) map[string]any {
	if payload == nil {
		return nil
	}
	out := make(map[string]any, len(payload))
	for k, v := range payload {
		out[k] = v
	}
	if !forwardMetadata {
		delete(out, "metadata")
	}
	if !forwardThinking {
		delete(out, "thinking")
	}
	return out
}

// extractAnthropicInputText 从 Anthropic messages 请求 payload 中提取可用于关键词判断的输入文本。
// extractMetadataUserID 从 payload.metadata.user_id 取出对话标识，用于对话级模型缓存（同一 user_id 只选一次模型）。
func extractMetadataUserID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	meta, _ := payload["metadata"].(map[string]any)
	if meta == nil {
		return ""
	}
	uid, _ := meta["user_id"].(string)
	return strings.TrimSpace(uid)
}

func extractAnthropicInputText(payload map[string]any) string {
	msgs, ok := payload["messages"].([]any)
	if !ok {
		return ""
	}

	for i := len(msgs) - 1; i >= 0; i-- {
		mm, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		if strings.TrimSpace(role) != "user" {
			continue
		}
		switch content := mm["content"].(type) {
		case string:
			return strings.TrimSpace(content)
		case []any:
			var sb strings.Builder
			for _, blk := range content {
				bm, ok := blk.(map[string]any)
				if !ok {
					continue
				}
				if t, _ := bm["type"].(string); t != "" && t != "text" {
					continue
				}
				if txt, ok := bm["text"].(string); ok && strings.TrimSpace(txt) != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(strings.TrimSpace(txt))
				}
			}
			return strings.TrimSpace(sb.String())
		default:
			return ""
		}
	}
	return ""
}

func recordUsageFromBody(c *gin.Context, body []byte) {
	input, output := extractUsageFromJSON(body)
	recordUsage(c, input, output)
}

func recordUsageFromBodyWithModel(c *gin.Context, body []byte, modelID string) {
	input, output := extractUsageFromJSON(body)
	recordUsageWithModel(c, input, output, modelID)
}

func trackUsageStream(c *gin.Context, src io.ReadCloser, modelID string) io.ReadCloser {
	if src == nil {
		return nil
	}
	pr, pw := io.Pipe()
	go func() {
		defer src.Close()
		defer pw.Close()

		scanner := bufio.NewScanner(src)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		var inputTokens int64
		var outputTokens int64

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if payload != "" && payload != "[DONE]" {
					in, out := extractUsageFromJSON([]byte(payload))
					inputTokens += in
					outputTokens += out
				}
			}
			if _, err := pw.Write([]byte(line + "\n")); err != nil {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			return
		}
		recordUsageWithModel(c, inputTokens, outputTokens, modelID)
	}()
	return pr
}

func extractUsageFromJSON(body []byte) (int64, int64) {
	if len(body) == 0 {
		return 0, 0
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0
	}
	usage, _ := payload["usage"].(map[string]any)
	if usage == nil {
		return 0, 0
	}
	input := numToInt64(usage["input_tokens"])
	if input == 0 {
		input = numToInt64(usage["prompt_tokens"])
	}
	output := numToInt64(usage["output_tokens"])
	if output == 0 {
		output = numToInt64(usage["completion_tokens"])
	}
	if output == 0 {
		output = numToInt64(usage["output_tokens_details"])
	}
	return input, output
}

func numToInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case int32:
		return int64(n)
	case uint64:
		return int64(n)
	case json.Number:
		x, _ := n.Int64()
		return x
	case string:
		x, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return x
	default:
		return 0
	}
}

func recordUsage(c *gin.Context, input, output int64) {
	recordUsageWithModel(c, input, output, "")
}

func recordUsageWithModel(c *gin.Context, input, output int64, modelID string) {
	u := middleware.CurrentUser(c)
	if u == nil || strings.TrimSpace(u.Username) == "" {
		return
	}
	utils.Logger.Printf("user:%v", u.Username)
	if input <= 0 && output <= 0 {
		return
	}

	//if err := model.AddUserUsage(u.Username, input, output); err == nil {
	//	u.InputTokens += input
	//	u.OutputTokens += output
	//	u.TotalTokens += input + output
	//}

	// 记录使用日志（包含模型单价信息）
	//if strings.TrimSpace(modelID) != "" {
	//	m, err := model.GetModel(modelID)
	//	if err == nil && m != nil {
	//		_ = model.RecordUsageLog(u.Username, modelID, input, output, m.InputPrice, m.OutputPrice)
	//	}
	//}
}
