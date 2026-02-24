package handler

import (
	"awesomeProject/internal/modelstate"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	openaiSDK "github.com/sashabaranov/go-openai"

	"awesomeProject/internal/combo"
	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/model"
	"awesomeProject/pkg/utils"
)

const codexDefaultBaseURL = "https://chatgpt.com/backend-api"

var (
	codexConversationModelMu sync.RWMutex
	codexConversationModel   = make(map[string]conversationModelEntry) // metadata.user_id -> (model_id,last_seen)
)

func init() {
	go func() {
		ticker := time.NewTicker(conversationModelCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-conversationModelTTL)
			codexConversationModelMu.Lock()
			for k, v := range codexConversationModel {
				if v.ModelID == "" || v.LastSeen.Before(cutoff) {
					delete(codexConversationModel, k)
				}
			}
			codexConversationModelMu.Unlock()
		}
	}()
}

// CodexProxyHandler 直接透传 OpenAI Responses API 到 Codex 上游。
type CodexProxyHandler struct {
	cfg *appconfig.Config
}

func NewCodexProxyHandler(cfg *appconfig.Config) *CodexProxyHandler {
	return &CodexProxyHandler{cfg: cfg}
}

// RegisterRoutes 注册 /v1/responses（挂到 /back 组后即 /back/v1/responses）。
func (h *CodexProxyHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/v1/responses", h.handleResponses)
	r.OPTIONS("/v1/responses", h.handleOptions)
}

// RegisterRoutesV1 注册到已带 /v1 前缀的路由组。
func (h *CodexProxyHandler) RegisterRoutesV1(r gin.IRoutes) {
	r.POST("/responses", h.handleResponses)
	r.OPTIONS("/responses", h.handleOptions)
}

func (h *CodexProxyHandler) handleOptions(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func (h *CodexProxyHandler) handleResponses(c *gin.Context) {
	ua := c.GetHeader("User-Agent")
	if len(ua) > 120 {
		ua = ua[:120] + "..."
	}
	utils.Logger.Printf("[ClaudeRouter] responses: request path=%s remote=%s ua=%s", c.Request.URL.Path, c.ClientIP(), ua)

	raw, err := c.GetRawData()
	if err != nil {
		utils.Logger.Printf("[ClaudeRouter] responses: step=read_body err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}
	utils.Logger.Printf("[ClaudeRouter] responses: step=request_raw len=%d body=%s", len(raw), raw)

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		utils.Logger.Printf("[ClaudeRouter] responses: step=parse_json err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	streamRequested := false
	if v, ok := payload["stream"].(bool); ok {
		streamRequested = v
	}

	requestedModel, _ := payload["model"].(string)
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		utils.Logger.Printf("[ClaudeRouter] responses: step=validate missing model")
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model"})
		return
	}

	conversationID := extractResponsesMetadataUserID(payload)
	inputText := extractResponsesInputText(payload)
	_, hasInput := payload["input"]
	_, hasMessages := payload["messages"]
	_, hasTools := payload["tools"]
	utils.Logger.Printf("[ClaudeRouter] responses: step=request_summary requested_model=%s stream=%v has_input=%v has_messages=%v has_tools=%v conv=%v input_chars=%d",
		requestedModel, streamRequested, hasInput, hasMessages, hasTools, conversationID != "", len(inputText))

	targetModel, usedCache, err := h.resolveResponseTargetModel(requestedModel, conversationID, inputText)
	if err != nil {
		utils.Logger.Printf("[ClaudeRouter] responses: step=resolve_model err=%v requested_model=%s", err, requestedModel)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	baseURL := ""
	apiKey := ""
	upstreamModel := requestedModel

	if targetModel != nil {
		baseURL, apiKey = h.resolveResponsesEndpoint(targetModel)
		upstreamModel = strings.TrimSpace(targetModel.UpstreamID)
		if upstreamModel == "" {
			upstreamModel = targetModel.ID
		}
	} else {
		// model 不在本地表时允许直接透传，使用 operators.codex 配置。
		baseURL, apiKey = h.resolveResponsesEndpoint(nil)
	}
	payloadToSend := applyResponsesAdapter(payload, upstreamModel, targetModel)
	adapterMode := "passthrough_unknown"
	if targetModel != nil {
		switch {
		case isDirectResponsesPassthroughModel(targetModel):
			adapterMode = "passthrough_direct"
		case isOpenAICompatibleModel(targetModel):
			adapterMode = "adapt_openai_compatible"
		default:
			adapterMode = "passthrough_other"
		}
	}
	targetID := ""
	targetInterface := ""
	targetOperator := ""
	if targetModel != nil {
		targetID = strings.TrimSpace(targetModel.ID)
		targetInterface = strings.TrimSpace(targetModel.Interface)
		targetOperator = strings.TrimSpace(targetModel.OperatorID)
	}
	_, sendHasInput := payloadToSend["input"]
	_, sendHasMessages := payloadToSend["messages"]
	_, sendHasTools := payloadToSend["tools"]
	utils.Logger.Printf("[ClaudeRouter] responses: step=resolved requested=%s target=%s upstream=%s interface=%s operator=%s used_cache=%v adapter=%s send_has_input=%v send_has_messages=%v send_has_tools=%v",
		requestedModel, targetID, upstreamModel, targetInterface, targetOperator, usedCache, adapterMode, sendHasInput, sendHasMessages, sendHasTools)

	reqBody, err := json.Marshal(payloadToSend)
	if err != nil {
		utils.Logger.Printf("[ClaudeRouter] responses: step=marshal_payload err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}
	utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_request_body len=%d body=%s", len(reqBody), reqBody)

	if adapterMode == "adapt_openai_compatible" {
		utils.Logger.Printf("[ClaudeRouter] responses: step=dispatch mode=sdk_openai_compatible base_url=%s api_key_set=%v", baseURL, apiKey != "")
		statusCode, contentType, body, streamBody := executeOpenAICompatibleResponsesViaSDK(
			c.Request.Context(), apiKey, baseURL, upstreamModel, reqBody, streamRequested,
		)

		if contentType == "" {
			contentType = "application/json"
		}

		if streamBody != nil && statusCode >= 200 && statusCode < 300 {
			utils.Logger.Printf("[ClaudeRouter] responses: step=proxy_stream mode=sdk_openai_compatible status=%d", statusCode)
			c.Header("Content-Type", "text/event-stream")
			utils.ProxySSE(c, trackUsageStream(c, streamBody, targetID))
			return
		}

		if statusCode < 200 || statusCode >= 300 {
			if conversationID != "" && usedCache {
				codexConversationModelMu.Lock()
				delete(codexConversationModel, conversationID)
				codexConversationModelMu.Unlock()
				utils.Logger.Printf("[ClaudeRouter] responses: step=clear_cache reason=upstream_status status=%d", statusCode)
			}
			if targetModel != nil {
				modelstate.DisableModelTemporarily(targetModel.ID, 15*time.Minute)
			}
			utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_error status=%d content_type=%s body_preview=%s",
				statusCode, contentType, debugBodySnippet(body, 600))
		} else {
			utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_success status=%d content_type=%s body_len=%d",
				statusCode, contentType, len(body))
		}
		c.Data(statusCode, contentType, body)
		return
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	upstreamURLs := buildResponsesUpstreamURLs(baseURL)
	utils.Logger.Printf("[ClaudeRouter] responses: step=dispatch mode=passthrough_responses base_url=%s candidates=%v api_key_set=%v", baseURL, upstreamURLs, apiKey != "")

	var resp *http.Response
	var lastErr error
	for i, upstreamURL := range upstreamURLs {
		attempt := i + 1
		utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_attempt idx=%d/%d url=%s", attempt, len(upstreamURLs), upstreamURL)
		req, reqErr := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
		if reqErr != nil {
			utils.Logger.Printf("[ClaudeRouter] responses: step=build_upstream_request err=%v idx=%d url=%s", reqErr, attempt, upstreamURL)
			lastErr = reqErr
			break
		}
		copyRequestHeaders(c.Request.Header, req.Header)
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, lastErr = client.Do(req)
		if lastErr != nil {
			utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_attempt err=%v idx=%d url=%s", lastErr, attempt, upstreamURL)
			continue
		}
		utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_attempt_done idx=%d status=%d content_type=%s url=%s", attempt, resp.StatusCode, resp.Header.Get("Content-Type"), upstreamURL)

		// 对不同网关做路径兜底：404 时尝试下一个候选 URL。
		if resp.StatusCode == http.StatusNotFound && i < len(upstreamURLs)-1 {
			preview, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_retry_on_404 idx=%d url=%s body_preview=%s", attempt, upstreamURL, debugBodySnippet(preview, 400))
			_ = resp.Body.Close()
			resp = nil
			continue
		}
		break
	}
	if lastErr != nil || resp == nil {
		if conversationID != "" && usedCache {
			codexConversationModelMu.Lock()
			delete(codexConversationModel, conversationID)
			codexConversationModelMu.Unlock()
			utils.Logger.Printf("[ClaudeRouter] responses: step=clear_cache reason=upstream_transport_error")
		}
		if targetModel != nil {
			modelstate.DisableModelTemporarily(targetModel.ID, 15*time.Minute)
		}
		if lastErr != nil {
			utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_failed err=%v", lastErr)
			c.JSON(http.StatusBadGateway, gin.H{"error": lastErr.Error()})
		} else {
			utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_failed err=nil_response")
			c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		}
		return
	}

	stream := streamRequested
	if v, ok := payloadToSend["stream"].(bool); ok {
		stream = v
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		stream = true
	}

	if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		utils.Logger.Printf("[ClaudeRouter] responses: step=proxy_stream status=%d content_type=%s", resp.StatusCode, contentType)
		utils.ProxySSE(c, resp.Body)
		return
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if contentType == "" {
		contentType = "application/json"
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if conversationID != "" && usedCache {
			codexConversationModelMu.Lock()
			delete(codexConversationModel, conversationID)
			codexConversationModelMu.Unlock()
			utils.Logger.Printf("[ClaudeRouter] responses: step=clear_cache reason=upstream_status status=%d", resp.StatusCode)
		}
		if targetModel != nil {
			modelstate.DisableModelTemporarily(targetModel.ID, 15*time.Minute)
		}
		utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_error status=%d content_type=%s body_preview=%s",
			resp.StatusCode, contentType, debugBodySnippet(body, 600))
	} else {
		utils.Logger.Printf("[ClaudeRouter] responses: step=upstream_success status=%d content_type=%s body_len=%d",
			resp.StatusCode, contentType, len(body))
	}

	c.Data(resp.StatusCode, contentType, body)
}

func (h *CodexProxyHandler) resolveResponseTargetModel(requestedModel, conversationID, inputText string) (*model.Model, bool, error) {
	// 1) 会话缓存优先（仅 combo 路由后写入）
	if conversationID != "" {
		now := time.Now()
		codexConversationModelMu.RLock()
		ent := codexConversationModel[conversationID]
		codexConversationModelMu.RUnlock()
		if ent.ModelID != "" {
			if !ent.LastSeen.IsZero() && now.Sub(ent.LastSeen) <= conversationModelTTL {
				m, err := model.GetModel(ent.ModelID)
				if err == nil && m.Enabled && !modelstate.IsModelTemporarilyDisabled(m.ID) && isCodexResponsesCandidate(m) {
					codexConversationModelMu.Lock()
					ent.LastSeen = now
					codexConversationModel[conversationID] = ent
					codexConversationModelMu.Unlock()
					return m, true, nil
				}
				codexConversationModelMu.Lock()
				delete(codexConversationModel, conversationID)
				codexConversationModelMu.Unlock()
			} else {
				codexConversationModelMu.Lock()
				delete(codexConversationModel, conversationID)
				codexConversationModelMu.Unlock()
			}
		}
	}

	// 2) combo 路由（关键词）
	if model.IsComboID(requestedModel) {
		cb, err := model.GetCombo(requestedModel)
		if err == nil && cb != nil {
			if !cb.Enabled {
				return nil, false, errors.New("model disabled: " + requestedModel)
			}

			filtered := make([]model.ComboItem, 0, len(cb.Items))
			for _, it := range cb.Items {
				modelID := strings.TrimSpace(it.ModelID)
				if modelID == "" || modelstate.IsModelTemporarilyDisabled(modelID) {
					continue
				}
				m, err := model.GetModel(modelID)
				if err != nil || m == nil || !m.Enabled || modelstate.IsModelTemporarilyDisabled(m.ID) || !isCodexResponsesCandidate(m) {
					continue
				}
				filtered = append(filtered, it)
			}
			if len(filtered) == 0 {
				return nil, false, errors.New("combo has no selectable codex/openai_responses items")
			}

			tmp := &model.Combo{ID: cb.ID, Name: cb.Name, Description: cb.Description, Enabled: cb.Enabled, Items: filtered}
			chosenID := combo.ChooseModelID(tmp, inputText)
			if strings.TrimSpace(chosenID) == "" {
				return nil, false, errors.New("combo has no selectable items")
			}
			m, err := model.GetModel(chosenID)
			if err != nil || m == nil || !m.Enabled || !isCodexResponsesCandidate(m) {
				return nil, false, errors.New("combo item model not found: " + chosenID)
			}

			if conversationID != "" {
				now := time.Now()
				codexConversationModelMu.Lock()
				if ent, ok := codexConversationModel[conversationID]; !ok || ent.ModelID == "" {
					codexConversationModel[conversationID] = conversationModelEntry{ModelID: m.ID, LastSeen: now}
				} else {
					ent.LastSeen = now
					codexConversationModel[conversationID] = ent
				}
				codexConversationModelMu.Unlock()
			}
			return m, true, nil
		}
		// combo 表里找不到时，继续走“本地模型表 -> operators.codex”的降级流程。
		if err != nil && !errors.Is(err, model.ErrNotFound) {
			return nil, false, errors.New("load combo failed")
		}
	}

	// 3) 直接模型
	m, err := model.GetModel(requestedModel)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			// 本地不存在请求模型时，尝试在本地候选模型中选择可用回退模型。
			if fm := pickFallbackResponsesModel(requestedModel); fm != nil {
				return fm, false, nil
			}
			// 最坏情况：允许直接透传到 operators.codex。
			return nil, false, nil
		}
		return nil, false, errors.New("load model failed")
	}
	if !m.Enabled {
		// 模型被永久禁用时，尝试选择回退模型
		if fm := pickFallbackResponsesModel(requestedModel); fm != nil {
			return fm, false, nil
		}
		return nil, false, errors.New("model disabled: " + m.ID)
	}
	if modelstate.IsModelTemporarilyDisabled(m.ID) {
		// 模型被临时禁用时，尝试选择回退模型
		if fm := pickFallbackResponsesModel(requestedModel); fm != nil {
			return fm, false, nil
		}
		return nil, false, errors.New("model temporarily disabled: " + m.ID)
	}
	if !isCodexResponsesCandidate(m) {
		return nil, false, errors.New("model must be operator_id=codex or interface_type in [openai_responses, openai_response, openai, openai_compatible]")
	}
	return m, false, nil
}

func isCodexResponsesCandidate(m *model.Model) bool {
	if m == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(m.OperatorID), "codex") {
		return true
	}
	it := strings.TrimSpace(m.Interface)
	return strings.EqualFold(it, "openai_responses") ||
		strings.EqualFold(it, "openai_response") ||
		strings.EqualFold(it, "openai") ||
		strings.EqualFold(it, "openai_compatible")
}

func pickFallbackResponsesModel(requestedModel string) *model.Model {
	candidates := make([]*model.Model, 0)
	for _, m := range model.ListModels() {
		if m == nil || !m.Enabled || modelstate.IsModelTemporarilyDisabled(m.ID) || !isCodexResponsesCandidate(m) {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) == 0 {
		return nil
	}

	// 优先：requestedModel 命中某个模型的 upstream_id。
	for _, m := range candidates {
		if strings.EqualFold(strings.TrimSpace(m.UpstreamID), strings.TrimSpace(requestedModel)) {
			return m
		}
	}

	// 其次：优先 operator_id=codex，其次按 id 字典序稳定选择。
	sort.Slice(candidates, func(i, j int) bool {
		ic := strings.EqualFold(strings.TrimSpace(candidates[i].OperatorID), "codex")
		jc := strings.EqualFold(strings.TrimSpace(candidates[j].OperatorID), "codex")
		if ic != jc {
			return ic
		}
		return strings.TrimSpace(candidates[i].ID) < strings.TrimSpace(candidates[j].ID)
	})
	return candidates[0]
}

func extractResponsesMetadataUserID(payload map[string]any) string {
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

func extractResponsesInputText(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	in := payload["input"]
	switch v := in.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		// 优先取最后一条 user 文本
		for i := len(v) - 1; i >= 0; i-- {
			m, ok := v[i].(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role != "" && !strings.EqualFold(strings.TrimSpace(role), "user") {
				continue
			}
			if s := extractResponseMessageContentText(m["content"]); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractResponseMessageContentText(content any) string {
	switch c := content.(type) {
	case string:
		return strings.TrimSpace(c)
	case []any:
		var sb strings.Builder
		for _, blk := range c {
			bm, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			t, _ := bm["type"].(string)
			if t != "" && !strings.EqualFold(t, "input_text") && !strings.EqualFold(t, "text") {
				continue
			}
			txt, _ := bm["text"].(string)
			txt = strings.TrimSpace(txt)
			if txt == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(txt)
		}
		return strings.TrimSpace(sb.String())
	default:
		return ""
	}
}

func applyResponsesAdapter(payload map[string]any, upstreamModel string, m *model.Model) map[string]any {
	out := cloneAnyMap(payload)
	out["model"] = upstreamModel
	if m == nil {
		// Unknown local model: keep historical fallback (direct pass-through to codex operator endpoint).
		return out
	}
	if isDirectResponsesPassthroughModel(m) {
		return out
	}
	if !isOpenAICompatibleModel(m) {
		return out
	}
	return normalizeOpenAICompatibleResponsesPayload(out)
}

func isDirectResponsesPassthroughModel(m *model.Model) bool {
	if m == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(m.OperatorID), "codex") {
		return true
	}
	it := strings.TrimSpace(m.Interface)
	return strings.EqualFold(it, "openai_responses") || strings.EqualFold(it, "openai_response")
}

func isOpenAICompatibleModel(m *model.Model) bool {
	if m == nil {
		return false
	}
	it := strings.TrimSpace(m.Interface)
	return strings.EqualFold(it, "openai") || strings.EqualFold(it, "openai_compatible")
}

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func normalizeOpenAICompatibleResponsesPayload(payload map[string]any) map[string]any {
	out := cloneAnyMap(payload)

	// ccx-style token field normalization.
	if _, ok := out["max_output_tokens"]; !ok {
		if v, ok := out["max_tokens"]; ok {
			out["max_output_tokens"] = v
		}
	}
	delete(out, "max_tokens")

	// ccx-style reasoning field compatibility.
	if rv, ok := out["reasoning"]; ok {
		if m, ok := rv.(map[string]any); ok {
			if _, hasEffort := m["effort"]; !hasEffort {
				if effort, ok := out["reasoning_effort"].(string); ok && strings.TrimSpace(effort) != "" {
					nm := cloneAnyMap(m)
					nm["effort"] = mapReasoningEffortToResponses(effort)
					out["reasoning"] = nm
				}
			}
		}
	} else if effort, ok := out["reasoning_effort"].(string); ok && strings.TrimSpace(effort) != "" {
		out["reasoning"] = map[string]any{
			"effort": mapReasoningEffortToResponses(effort),
		}
	}
	delete(out, "reasoning_effort")

	// Legacy chat-completions functions/function_call compatibility.
	if _, ok := out["tools"]; !ok {
		if functions, ok := out["functions"]; ok {
			out["tools"] = legacyFunctionsToResponsesTools(functions)
		}
	}
	delete(out, "functions")
	if _, ok := out["tool_choice"]; !ok {
		if functionCall, ok := out["function_call"]; ok {
			if tc := legacyFunctionCallToResponsesToolChoice(functionCall); tc != nil {
				out["tool_choice"] = tc
			}
		}
	}
	delete(out, "function_call")

	// Input canonicalization. Keep /v1/responses and normalize request shape only.
	if input, ok := out["input"]; ok {
		out["input"] = normalizeResponsesInput(input)
	} else if msgs, ok := out["messages"]; ok {
		input, instructions := chatMessagesToResponsesInput(msgs)
		if len(input) > 0 {
			out["input"] = input
		}
		if _, hasInstructions := out["instructions"]; !hasInstructions && instructions != "" {
			out["instructions"] = instructions
		}
	}
	delete(out, "messages")

	if tools, ok := out["tools"]; ok {
		out["tools"] = normalizeResponsesTools(tools)
	}
	if toolChoice, ok := out["tool_choice"]; ok {
		out["tool_choice"] = normalizeResponsesToolChoice(toolChoice)
	}
	return out
}

func mapReasoningEffortToResponses(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none":
		return "none"
	case "minimal":
		return "minimal"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high", "xhigh":
		return "high"
	case "auto":
		return "auto"
	default:
		return "auto"
	}
}

func chatMessagesToResponsesInput(messages any) ([]any, string) {
	rawMsgs, ok := messages.([]any)
	if !ok {
		return nil, ""
	}
	out := make([]any, 0, len(rawMsgs))
	instructions := ""
	for _, it := range rawMsgs {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(getStringMapValue(m, "role")))
		if role == "" {
			role = "user"
		}

		switch role {
		case "tool":
			item, ok := normalizeResponsesFunctionCallOutputItem(map[string]any{
				"call_id":      firstNonEmptyString(getStringMapValue(m, "tool_call_id"), getStringMapValue(m, "call_id")),
				"tool_call_id": getStringMapValue(m, "tool_call_id"),
				"content":      m["content"],
			})
			if ok {
				out = append(out, item)
			}
		case "assistant":
			content := normalizeResponsesMessageContent(m["content"])
			if !isEmptyResponsesContent(content) {
				out = append(out, map[string]any{
					"type":    "message",
					"role":    role,
					"content": content,
				})
			}
			if toolCalls, ok := m["tool_calls"].([]any); ok {
				for _, rawToolCall := range toolCalls {
					tc, ok := rawToolCall.(map[string]any)
					if !ok {
						continue
					}
					item, ok := normalizeResponsesFunctionCallItem(tc)
					if ok {
						out = append(out, item)
					}
				}
			}
		default:
			content := normalizeResponsesMessageContent(m["content"])
			if role == "system" && instructions == "" {
				if s := extractResponseMessageContentText(content); s != "" {
					instructions = s
					continue
				}
			}
			if isEmptyResponsesContent(content) {
				continue
			}
			out = append(out, map[string]any{
				"type":    "message",
				"role":    role,
				"content": content,
			})
		}
	}
	return out, instructions
}

func normalizeResponsesInput(input any) any {
	switch v := input.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, raw := range v {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.ToLower(strings.TrimSpace(getStringMapValue(item, "type")))
			if itemType == "" && strings.TrimSpace(getStringMapValue(item, "role")) != "" {
				itemType = "message"
			}
			switch itemType {
			case "message":
				out = append(out, normalizeResponsesMessageItem(item))
			case "function_call":
				if fc, ok := normalizeResponsesFunctionCallItem(item); ok {
					out = append(out, fc)
				}
			case "function_call_output":
				if fco, ok := normalizeResponsesFunctionCallOutputItem(item); ok {
					out = append(out, fco)
				}
			default:
				out = append(out, item)
			}
		}
		return out
	default:
		return input
	}
}

func normalizeResponsesMessageItem(item map[string]any) map[string]any {
	role := strings.ToLower(strings.TrimSpace(getStringMapValue(item, "role")))
	if role == "" {
		role = "user"
	}
	out := map[string]any{
		"type": "message",
		"role": role,
	}
	if content, ok := item["content"]; ok {
		out["content"] = normalizeResponsesMessageContent(content)
	} else {
		out["content"] = []any{}
	}
	return out
}

func normalizeResponsesFunctionCallItem(item map[string]any) (map[string]any, bool) {
	name := strings.TrimSpace(getStringMapValue(item, "name"))
	if name == "" {
		if fn, ok := item["function"].(map[string]any); ok {
			name = strings.TrimSpace(getStringMapValue(fn, "name"))
		}
	}
	if name == "" {
		return nil, false
	}
	out := map[string]any{
		"type": "function_call",
		"name": name,
	}

	callID := strings.TrimSpace(getStringMapValue(item, "call_id"))
	if callID == "" {
		callID = strings.TrimSpace(getStringMapValue(item, "id"))
	}
	if callID != "" {
		out["call_id"] = callID
	}

	if args, ok := item["arguments"]; ok {
		out["arguments"] = toJSONStringOrString(args)
	} else if fn, ok := item["function"].(map[string]any); ok {
		if args, ok := fn["arguments"]; ok {
			out["arguments"] = toJSONStringOrString(args)
		}
	}
	return out, true
}

func normalizeResponsesFunctionCallOutputItem(item map[string]any) (map[string]any, bool) {
	callID := strings.TrimSpace(getStringMapValue(item, "call_id"))
	if callID == "" {
		callID = strings.TrimSpace(getStringMapValue(item, "tool_call_id"))
	}
	if callID == "" {
		return nil, false
	}
	out := map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
	}
	if v, ok := item["output"]; ok {
		out["output"] = normalizeFunctionCallOutput(v)
		return out, true
	}
	if v, ok := item["content"]; ok {
		out["output"] = normalizeFunctionCallOutput(v)
		return out, true
	}
	out["output"] = ""
	return out, true
}

func normalizeFunctionCallOutput(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if text := extractResponseMessageContentText(t); text != "" {
			return text
		}
		return toJSONStringOrString(t)
	case map[string]any:
		if text := extractResponseMessageContentText(t); text != "" {
			return text
		}
		return toJSONStringOrString(t)
	default:
		return toJSONStringOrString(t)
	}
}

func normalizeResponsesMessageContent(content any) any {
	switch v := content.(type) {
	case string:
		return []any{
			map[string]any{"type": "input_text", "text": v},
		}
	case []any:
		out := make([]any, 0, len(v))
		for _, raw := range v {
			block, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			blockType := strings.ToLower(strings.TrimSpace(getStringMapValue(block, "type")))
			switch blockType {
			case "", "text", "input_text", "output_text":
				txt := getStringMapValue(block, "text")
				if blockType == "" && txt == "" {
					out = append(out, block)
					continue
				}
				out = append(out, map[string]any{
					"type": "input_text",
					"text": txt,
				})
			case "image_url":
				out = append(out, normalizeImageURLBlock(block))
			default:
				out = append(out, block)
			}
		}
		return out
	case map[string]any:
		blockType := strings.ToLower(strings.TrimSpace(getStringMapValue(v, "type")))
		if blockType == "" || blockType == "text" || blockType == "input_text" || blockType == "output_text" {
			return []any{
				map[string]any{
					"type": "input_text",
					"text": getStringMapValue(v, "text"),
				},
			}
		}
		if blockType == "image_url" {
			return []any{normalizeImageURLBlock(v)}
		}
		return []any{v}
	default:
		return content
	}
}

func normalizeImageURLBlock(block map[string]any) map[string]any {
	out := map[string]any{"type": "input_image"}
	switch v := block["image_url"].(type) {
	case string:
		u := strings.TrimSpace(v)
		if u != "" {
			out["image_url"] = u
		}
	case map[string]any:
		if u := strings.TrimSpace(getStringMapValue(v, "url")); u != "" {
			out["image_url"] = u
		}
	}
	if detail := strings.TrimSpace(getStringMapValue(block, "detail")); detail != "" {
		out["detail"] = detail
	}
	return out
}

func isEmptyResponsesContent(content any) bool {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		if len(v) == 0 {
			return true
		}
		for _, raw := range v {
			block, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			blockType := strings.ToLower(strings.TrimSpace(getStringMapValue(block, "type")))
			switch blockType {
			case "input_text", "text", "output_text", "":
				if strings.TrimSpace(getStringMapValue(block, "text")) != "" {
					return false
				}
			default:
				return false
			}
		}
		return true
	default:
		return content == nil
	}
}

func legacyFunctionsToResponsesTools(functions any) any {
	rawFunctions, ok := functions.([]any)
	if !ok {
		return functions
	}
	tools := make([]any, 0, len(rawFunctions))
	for _, raw := range rawFunctions {
		fn, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(getStringMapValue(fn, "name"))
		if name == "" {
			continue
		}
		tool := map[string]any{
			"type": "function",
			"name": name,
		}
		if desc := strings.TrimSpace(getStringMapValue(fn, "description")); desc != "" {
			tool["description"] = desc
		}
		if params, ok := fn["parameters"]; ok {
			tool["parameters"] = params
		}
		tools = append(tools, tool)
	}
	return tools
}

func legacyFunctionCallToResponsesToolChoice(functionCall any) any {
	switch v := functionCall.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "none":
			return "none"
		case "auto":
			return "auto"
		case "required":
			return "required"
		default:
			return nil
		}
	case map[string]any:
		if name := strings.TrimSpace(getStringMapValue(v, "name")); name != "" {
			return map[string]any{
				"type": "function",
				"name": name,
			}
		}
	}
	return nil
}

func toJSONStringOrString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func normalizeResponsesTools(tools any) any {
	rawTools, ok := tools.([]any)
	if !ok {
		return tools
	}
	out := make([]any, 0, len(rawTools))
	for _, t := range rawTools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}

		// chat 风格：{type:function,function:{name,description,parameters}}
		if fn, ok := m["function"].(map[string]any); ok && fn != nil {
			name := strings.TrimSpace(getStringMapValue(fn, "name"))
			if name == "" {
				out = append(out, m)
				continue
			}
			norm := map[string]any{
				"type": "function",
				"name": name,
			}
			if desc := strings.TrimSpace(getStringMapValue(fn, "description")); desc != "" {
				norm["description"] = desc
			}
			if p, ok := fn["parameters"]; ok {
				norm["parameters"] = p
			}
			if strict, ok := fn["strict"].(bool); ok {
				norm["strict"] = strict
			} else if strict, ok := m["strict"].(bool); ok {
				norm["strict"] = strict
			}
			out = append(out, norm)
			continue
		}

		// anthropic 风格：{name,description,input_schema}
		if name := strings.TrimSpace(getStringMapValue(m, "name")); name != "" {
			norm := map[string]any{
				"type": "function",
				"name": name,
			}
			if desc := strings.TrimSpace(getStringMapValue(m, "description")); desc != "" {
				norm["description"] = desc
			}
			if schema, ok := m["input_schema"]; ok {
				norm["parameters"] = schema
			} else if params, ok := m["parameters"]; ok {
				norm["parameters"] = params
			}
			if strict, ok := m["strict"].(bool); ok {
				norm["strict"] = strict
			}
			out = append(out, norm)
			continue
		}

		// Already a non-function Responses built-in tool (for example web_search).
		if t := strings.TrimSpace(getStringMapValue(m, "type")); t != "" && !strings.EqualFold(t, "function") {
			out = append(out, m)
			continue
		}

		out = append(out, m)
	}
	return out
}

func normalizeResponsesToolChoice(toolChoice any) any {
	if s, ok := toolChoice.(string); ok {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "none":
			return "none"
		case "auto":
			return "auto"
		case "required":
			return "required"
		default:
			return toolChoice
		}
	}

	m, ok := toolChoice.(map[string]any)
	if !ok {
		return toolChoice
	}
	tt := strings.ToLower(strings.TrimSpace(getStringMapValue(m, "type")))
	switch tt {
	case "none", "auto", "required":
		return tt
	case "tool":
		if name := strings.TrimSpace(getStringMapValue(m, "name")); name != "" {
			return map[string]any{
				"type": "function",
				"name": name,
			}
		}
		if fn, ok := m["function"].(map[string]any); ok && fn != nil {
			name := strings.TrimSpace(getStringMapValue(fn, "name"))
			if name != "" {
				return map[string]any{
					"type": "function",
					"name": name,
				}
			}
		}
		return "auto"
	case "function", "":
		// continue below
	default:
		return toolChoice
	}

	if name := strings.TrimSpace(getStringMapValue(m, "name")); name != "" {
		return map[string]any{
			"type": "function",
			"name": name,
		}
	}

	// chat 风格：{type:function,function:{name:...}}
	if fn, ok := m["function"].(map[string]any); ok && fn != nil {
		name := strings.TrimSpace(getStringMapValue(fn, "name"))
		if name != "" {
			return map[string]any{
				"type": "function",
				"name": name,
			}
		}
	}
	return toolChoice
}

func getStringMapValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func debugBodySnippet(body []byte, max int) string {
	if len(body) == 0 {
		return ""
	}
	if max <= 0 {
		max = 400
	}
	s := strings.TrimSpace(string(body))
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

var responsesVersionSuffixPattern = regexp.MustCompile(`/v\d+[a-z]*$`)

func (h *CodexProxyHandler) resolveResponsesEndpoint(m *model.Model) (baseURL, apiKey string) {
	if m != nil {
		baseURL = strings.TrimSpace(m.BaseURL)
		apiKey = strings.TrimSpace(m.APIKey)
	}

	// 优先使用模型绑定的 operator 配置（如 iflow/newapi/codex）。
	if m != nil && h.cfg != nil && h.cfg.Operators != nil {
		if opID := strings.TrimSpace(m.OperatorID); opID != "" {
			if op, ok := h.cfg.Operators[opID]; ok {
				if baseURL == "" {
					baseURL = strings.TrimSpace(op.BaseURL)
				}
				if apiKey == "" {
					apiKey = strings.TrimSpace(op.APIKey)
				}
			}
		}
	}

	// 兼容旧行为：兜底使用 codex 运营商配置。
	if h.cfg != nil && h.cfg.Operators != nil {
		if op, ok := h.cfg.Operators["codex"]; ok {
			if baseURL == "" {
				baseURL = strings.TrimSpace(op.BaseURL)
			}
			if apiKey == "" {
				apiKey = strings.TrimSpace(op.APIKey)
			}
		}
	}
	if baseURL == "" {
		baseURL = codexDefaultBaseURL
	}
	return baseURL, apiKey
}

func buildResponsesUpstreamURLs(baseURL string) []string {
	base := strings.TrimSpace(baseURL)
	if strings.HasSuffix(base, "#") {
		base = strings.TrimSuffix(base, "#")
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		base = codexDefaultBaseURL
	}

	addUnique := func(urls []string, u string) []string {
		for _, x := range urls {
			if x == u {
				return urls
			}
		}
		return append(urls, u)
	}

	lower := strings.ToLower(base)
	urls := make([]string, 0, 4)
	switch {
	case strings.HasSuffix(lower, "/v1/responses"), strings.HasSuffix(lower, "/responses"):
		urls = addUnique(urls, base)
	case strings.HasSuffix(lower, "/v1"):
		urls = addUnique(urls, base+"/responses")
		urls = addUnique(urls, strings.TrimSuffix(base, "/v1")+"/v1/responses")
		urls = addUnique(urls, strings.TrimSuffix(base, "/v1")+"/responses")
	case responsesVersionSuffixPattern.MatchString(lower):
		urls = addUnique(urls, base+"/responses")
		if idx := strings.LastIndex(base, "/"); idx > 0 {
			urls = addUnique(urls, base[:idx]+"/v1/responses")
		}
	default:
		urls = addUnique(urls, base+"/v1/responses")
		urls = addUnique(urls, base+"/responses")
	}
	return urls
}

func executeOpenAICompatibleResponsesViaSDK(
	ctx context.Context,
	apiKey, baseURL, upstreamModel string,
	responsesRequestRawJSON []byte,
	streamRequested bool,
) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser) {
	chatReqRaw := ConvertResponsesToOpenAIChatRequest(upstreamModel, responsesRequestRawJSON, streamRequested)
	utils.Logger.Printf("[ClaudeRouter] responses: step=sdk_chat_request len=%d body=%s", len(chatReqRaw), chatReqRaw)

	var chatReq openaiSDK.ChatCompletionRequest
	if err := json.Unmarshal(chatReqRaw, &chatReq); err != nil {
		b, _ := json.Marshal(gin.H{"error": "invalid adapted chat completions payload"})
		return http.StatusBadRequest, "application/json", b, nil
	}
	if streamRequested {
		chatReq.Stream = true
	}

	sdkBaseURL := normalizeOpenAICompatibleSDKBaseURL(baseURL)
	utils.Logger.Printf(
		"[ClaudeRouter] responses: step=sdk_dispatch sdk_base_url=%s model=%s stream=%v api_key_set=%v",
		sdkBaseURL, chatReq.Model, chatReq.Stream, strings.TrimSpace(apiKey) != "",
	)

	cfg := openaiSDK.DefaultConfig(strings.TrimSpace(apiKey))
	cfg.BaseURL = sdkBaseURL
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Minute}
	client := openaiSDK.NewClientWithConfig(cfg)

	if chatReq.Stream {
		stream, err := client.CreateChatCompletionStream(ctx, chatReq)
		if err != nil {
			status, ct, b := openAICompatibleSDKErrorResponse(err)
			utils.Logger.Printf("[ClaudeRouter] responses: step=sdk_stream_open_error status=%d body_preview=%s err=%v", status, debugBodySnippet(b, 600), err)
			return status, ct, b, nil
		}
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer stream.Close()
			proxyOpenAICompatibleSDKStreamAsResponses(ctx, stream, pw, upstreamModel, responsesRequestRawJSON, chatReqRaw)
		}()
		return http.StatusOK, "text/event-stream", nil, pr
	}

	resp, err := client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		status, ct, b := openAICompatibleSDKErrorResponse(err)
		utils.Logger.Printf("[ClaudeRouter] responses: step=sdk_nonstream_error status=%d body_preview=%s err=%v", status, debugBodySnippet(b, 600), err)
		return status, ct, b, nil
	}

	chatRespRaw, err := json.Marshal(resp)
	if err != nil {
		b, _ := json.Marshal(gin.H{"error": "failed to encode upstream response"})
		return http.StatusBadGateway, "application/json", b, nil
	}
	converted := ConvertOpenAIChatToResponsesNonStream(ctx, upstreamModel, responsesRequestRawJSON, chatReqRaw, chatRespRaw, nil)
	return http.StatusOK, "application/json", []byte(converted), nil
}

func normalizeOpenAICompatibleSDKBaseURL(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	base = strings.TrimSuffix(base, "#")
	base = strings.TrimRight(base, "/")
	if base == "" {
		return "https://api.openai.com/v1"
	}

	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "/v1/chat/completions"):
		base = base[:len(base)-len("/chat/completions")]
	case strings.HasSuffix(lower, "/chat/completions"):
		base = base[:len(base)-len("/chat/completions")]
	case strings.HasSuffix(lower, "/v1/responses"):
		base = base[:len(base)-len("/responses")]
	case strings.HasSuffix(lower, "/responses"):
		base = base[:len(base)-len("/responses")]
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return "https://api.openai.com/v1"
	}

	lower = strings.ToLower(base)
	if strings.HasSuffix(lower, "/v1") {
		return base
	}
	if responsesVersionSuffixPattern.MatchString(lower) {
		return responsesVersionSuffixPattern.ReplaceAllString(base, "/v1")
	}
	return base + "/v1"
}

func openAICompatibleSDKErrorResponse(err error) (statusCode int, contentType string, body []byte) {
	statusCode = http.StatusBadGateway
	contentType = "application/json"
	msg := "upstream request failed"

	var apiErr *openaiSDK.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode > 0 {
			statusCode = apiErr.HTTPStatusCode
		} else {
			statusCode = http.StatusBadRequest
		}
		if s := strings.TrimSpace(apiErr.Message); s != "" {
			msg = s
		}
		payload := map[string]any{"error": msg}
		if t := strings.TrimSpace(apiErr.Type); t != "" {
			payload["type"] = t
		}
		body, _ = json.Marshal(payload)
		return
	}

	var reqErr *openaiSDK.RequestError
	if errors.As(err, &reqErr) {
		if reqErr.HTTPStatusCode > 0 {
			statusCode = reqErr.HTTPStatusCode
		}
		if len(reqErr.Body) > 0 {
			if json.Valid(reqErr.Body) {
				return statusCode, contentType, reqErr.Body
			}
			if s := strings.TrimSpace(extractUpstreamErrorMessage(reqErr.Body)); s != "" {
				msg = s
			}
		} else if reqErr.Err != nil && strings.TrimSpace(reqErr.Err.Error()) != "" {
			msg = reqErr.Err.Error()
		}
		body, _ = json.Marshal(map[string]any{"error": msg})
		return
	}

	if err != nil && strings.TrimSpace(err.Error()) != "" {
		msg = err.Error()
	}
	body, _ = json.Marshal(map[string]any{"error": msg})
	return
}

func proxyOpenAICompatibleSDKStreamAsResponses(
	ctx context.Context,
	stream *openaiSDK.ChatCompletionStream,
	out io.Writer,
	model string,
	originalRequestRawJSON []byte,
	chatRequestRawJSON []byte,
) {
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	var state any
	for {
		if ctx.Err() != nil {
			return
		}
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				events := ConvertOpenAIChatToResponses(ctx, model, originalRequestRawJSON, chatRequestRawJSON, []byte("data: [DONE]"), &state)
				for _, event := range events {
					_, _ = writer.WriteString(event)
				}
				_ = writer.Flush()
				return
			}
			utils.Logger.Printf("[ClaudeRouter] responses: step=sdk_stream_recv_error err=%v", err)
			return
		}

		chunkRaw, marshalErr := json.Marshal(chunk)
		if marshalErr != nil {
			utils.Logger.Printf("[ClaudeRouter] responses: step=sdk_stream_chunk_marshal_error err=%v", marshalErr)
			continue
		}

		line := "data: " + string(chunkRaw)
		events := ConvertOpenAIChatToResponses(ctx, model, originalRequestRawJSON, chatRequestRawJSON, []byte(line), &state)
		for _, event := range events {
			_, _ = writer.WriteString(event)
		}
		_ = writer.Flush()
	}
}

func copyRequestHeaders(src http.Header, dst http.Header) {
	for k, vals := range src {
		lk := strings.ToLower(k)
		switch lk {
		case "host", "content-length", "authorization":
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}
