package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"awesomeProject/internal/combo"
	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/model"
	"awesomeProject/internal/translator/messages"
	"awesomeProject/pkg/utils"
)

// 对话级模型只选一次：依据 payload.metadata.user_id 判断是否同一对话，首次解析并缓存模型，后续同 user_id 使用缓存模型。
var (
	conversationModelMu sync.RWMutex
	conversationModel   = make(map[string]conversationModelEntry) // metadata.user_id -> (model_id,last_seen)
)

type conversationModelEntry struct {
	ModelID  string
	LastSeen time.Time
}

const (
	conversationModelTTL             = 10 * time.Minute
	conversationModelCleanupInterval = 15 * time.Minute
)

func init() {
	// 定期清理对话级模型缓存，避免 metadata.user_id 无限增长导致内存泄漏。
	go func() {
		ticker := time.NewTicker(conversationModelCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-conversationModelTTL)
			conversationModelMu.Lock()
			for k, v := range conversationModel {
				if v.ModelID == "" || v.LastSeen.Before(cutoff) {
					delete(conversationModel, k)
				}
			}
			conversationModelMu.Unlock()
		}
	}()
}

// MessagesHandler 提供 Anthropic 兼容的 /v1/messages 入口，供 Claude Code 调用。
type MessagesHandler struct {
	cfg *appconfig.Config
}

func NewMessagesHandler(cfg *appconfig.Config) *MessagesHandler {
	return &MessagesHandler{cfg: cfg}
}

// RegisterRoutes 在给定路由组上注册 /v1/messages（完整路径 = 组前缀 + /v1/messages，如 /back/v1/messages）。
func (h *MessagesHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/v1/messages", h.handleMessages)
	r.POST("/v1/messages/count_tokens", h.handleCountTokens)
	r.OPTIONS("/v1/messages", h.handleOptions)
	r.OPTIONS("/v1/messages/count_tokens", h.handleOptions)
}

// RegisterRoutesV1 在「已带 /v1 前缀」的路由组上注册 /messages 与 /messages/count_tokens（完整路径 = 组前缀 + /messages，如 /v1/messages）。
func (h *MessagesHandler) RegisterRoutesV1(r gin.IRoutes) {
	r.POST("/messages", h.handleMessages)
	r.POST("/messages/count_tokens", h.handleCountTokens)
	r.OPTIONS("/messages", h.handleOptions)
	r.OPTIONS("/messages/count_tokens", h.handleOptions)
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

// anthropicErrorFromBody 根据上游状态码和 body 按 Claude 格式返回错误（对话过程中上游 4xx/5xx 时使用）。
func anthropicErrorFromBody(c *gin.Context, statusCode int, body []byte) {
	message := "Upstream request failed"
	if len(body) > 0 {
		var m map[string]any
		if json.Unmarshal(body, &m) == nil {
			if errObj, _ := m["error"].(map[string]any); errObj != nil {
				if msg, _ := errObj["message"].(string); msg != "" {
					message = msg
				}
			} else if errObj, _ := m["errors"].(map[string]any); errObj != nil {
				if msg, _ := errObj["message"].(string); msg != "" {
					message = msg
				}
			} else if msg, _ := m["message"].(string); msg != "" {
				message = msg
			} else if msg, _ := m["error"].(string); msg != "" {
				message = msg
			}
		} else if len(body) <= 500 {
			message = string(body)
		}
	}
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
		now := time.Now()
		conversationModelMu.RLock()
		ent := conversationModel[conversationID]
		conversationModelMu.RUnlock()
		if ent.ModelID != "" {
			// TTL 过期视为未缓存
			if !ent.LastSeen.IsZero() && now.Sub(ent.LastSeen) <= conversationModelTTL {
				cachedModelID = ent.ModelID
				requestedModel = cachedModelID
				utils.Logger.Printf("[ClaudeRouter] messages: step=conversation_model cached user_id=%s model=%s", conversationID, requestedModel)

				// 更新 last_seen
				conversationModelMu.Lock()
				ent.LastSeen = now
				conversationModel[conversationID] = ent
				conversationModelMu.Unlock()
			} else {
				conversationModelMu.Lock()
				delete(conversationModel, conversationID)
				conversationModelMu.Unlock()
			}
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
		chosenID := combo.ChooseModelID(cb, inputText)
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
			now := time.Now()
			conversationModelMu.Lock()
			if ent, ok := conversationModel[conversationID]; !ok || ent.ModelID == "" {
				conversationModel[conversationID] = conversationModelEntry{ModelID: targetModel.ID, LastSeen: now}
				utils.Logger.Printf("[ClaudeRouter] messages: step=conversation_model set user_id=%s model=%s", conversationID, targetModel.ID)
			} else {
				ent.LastSeen = now
				conversationModel[conversationID] = ent
			}
			conversationModelMu.Unlock()
		}
	}

	if !targetModel.Enabled {
		anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Model disabled: "+targetModel.ID)
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
		default:
			baseURL = "https://api.anthropic.com"
		}
	}

	// 按模型配置决定是否保留扩展字段（metadata、thinking），避免上游 422
	payloadToSend := applyForwardExtendedFields(payload, targetModel.ForwardMetadata, targetModel.ForwardThinking)

	// 按模型配置的 QPS 限流
	waitModelQPS(c.Request.Context(), targetModel.ID, targetModel.MaxQPS)
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
		utils.Logger.Printf("[ClaudeRouter] messages: step=execute_call operator=%s", operatorID)
		statusCode, contentType, body, streamBody, err = strategy.Execute(c.Request.Context(), payloadToSend, opts)
	} else {
		adapter := messages.Registry.GetOrDefault(interfaceType)
		if adapter == nil {
			utils.Logger.Printf("[ClaudeRouter] messages: step=adapter err=unsupported type=%s", interfaceType)
			anthropicError(c, http.StatusBadRequest, "invalid_request_error", "Unsupported interface_type: "+interfaceType)
			return
		}
		utils.Logger.Printf("[ClaudeRouter] messages: step=execute_call adapter=%s upstream_model=%s", interfaceType, upstreamID)
		statusCode, contentType, body, streamBody, err = adapter.Execute(c.Request.Context(), payloadToSend, opts)
	}
	utils.Logger.Printf("[ClaudeRouter] messages: step=execute_done status=%d contentType=%s bodyLen=%d streamBody=%v err=%v", statusCode, contentType, len(body), streamBody != nil, err)

	if err != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: step=execute_err err=%v", err)
		if c.Request.Context().Err() != nil {
			utils.Logger.Printf("[ClaudeRouter] messages: client_gone, skip error response")
			return
		}
		if statusCode >= 400 {
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
		utils.Logger.Printf("[ClaudeRouter] messages: step=upstream_non_2xx status=%d", statusCode)
		anthropicErrorFromBody(c, statusCode, body)
		return
	}

	if stream && streamBody != nil {
		utils.Logger.Printf("[ClaudeRouter] messages: step=stream_write")
		defer streamBody.Close()
		utils.ProxySSE(c, streamBody)
		return
	}

	utils.Logger.Printf("[ClaudeRouter] messages: step=write_response status=%d len=%d", statusCode, len(body))
	ct := contentType
	if ct == "" {
		ct = "application/json"
	}
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

	// 仅取 messages 中 role=user 的最新一条，作为关键词路由的输入文本（避免 assistant/system 内容干扰）。
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
