package handler

import (
	"awesomeProject/internal/combo"
	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
	"awesomeProject/internal/modelstate"
	"awesomeProject/internal/translator/messages"
	"awesomeProject/pkg/utils"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

const codexDefaultBaseURL = "https://chatgpt.com/backend-api"

// CodexProxyHandler 直接透传 OpenAI Responses API 到 Codex 上游。
type CodexProxyHandler struct {
	cfg *appconfig.Config
}

func NewCodexProxyHandler(cfg *appconfig.Config) *CodexProxyHandler {
	return &CodexProxyHandler{cfg: cfg}
}

func (h *CodexProxyHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/v1/responses", h.handleResponses)
	r.OPTIONS("/v1/responses", h.handleOptions)
}

func (h *CodexProxyHandler) RegisterRoutesV1(r gin.IRoutes) {
	r.POST("/responses", h.handleResponses)
	r.OPTIONS("/responses", h.handleOptions)
}

func (h *CodexProxyHandler) handleOptions(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func (h *CodexProxyHandler) handleResponses(c *gin.Context) {
	ua := c.GetHeader("User-Agent")
	if len(ua) > 80 {
		ua = ua[:80] + "..."
	}
	utils.Logger.Debugf("[ClaudeRouter] responses: request path=%s remote=%s user_agent=%s", c.Request.URL.Path, c.ClientIP(), ua)

	raw, err := c.GetRawData()
	if err != nil {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read body")
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}

	requestedModel, _ := payload["model"].(string)
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", "Missing model")
		return
	}

	stream := false
	if v, ok := payload["stream"].(bool); ok {
		stream = v
	}

	conversationID := extractResponsesMetadataUserID(payload)
	inputText := extractResponsesInputText(payload)

	// 校验用户是否有权限使用该 combo/model
	currentUser := middleware.CurrentUser(c)
	if currentUser != nil && !currentUser.IsAdmin {
		if err := checkUserModelPermission(currentUser, requestedModel); err != nil {
			openaiError(c, http.StatusForbidden, "permission_denied", err.Error())
			return
		}
	}

	targetModel, usedCache, err := resolveResponseTargetModel(requestedModel, conversationID, inputText)
	if err != nil {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	c.Set("real_model_id", targetModel.ID)
	if conversationID != "" {
		c.Set("real_conversation_id", conversationID)
	}

	interfaceType, baseURL, apiKey, resolveErr := h.resolveResponsesEndpoint(targetModel)
	if resolveErr != nil {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", resolveErr.Error())
		return
	}

	upstreamModel := strings.TrimSpace(targetModel.UpstreamID)
	if upstreamModel == "" {
		upstreamModel = targetModel.ID
	}

	payloadToSend := applyResponsesAdapter(payload, upstreamModel, targetModel)

	waitModelQPS(c.Request.Context(), targetModel.ID, targetModel.MaxQPS)
	if c.Request.Context().Err() != nil {
		return
	}

	adapterMode := "passthrough_unknown"
	if targetModel != nil {
		switch {
		case isDirectResponsesPassthroughModel(targetModel):
			adapterMode = "passthrough_direct"
		case isOpenAICompatibleModel(targetModel):
			adapterMode = "adapt_openai_compatible_sdk"
		case isAnthropicModel(targetModel):
			adapterMode = "adapt_anthropic_sdk"
		default:
			adapterMode = "passthrough_other"
		}
	}

	utils.Logger.Debugf("[ClaudeRouter] responses: step=execute interface=%s model=%s upstream=%s stream=%v adapter=%s",
		interfaceType, targetModel.ID, upstreamModel, stream, adapterMode)

	statusCode, contentType, body, streamBody, execErr := h.executeResponsesRequest(
		c.Request.Context(),
		payloadToSend,
		messages.ExecuteOptions{
			UpstreamModel: upstreamModel,
			APIKey:        apiKey,
			BaseURL:       baseURL,
			Stream:        stream,
		},
		adapterMode,
	)

	if execErr != nil {
		// 客户端主动取消请求，不记录错误日志，不封禁模型
		if c.Request.Context().Err() != nil {
			return
		}
		if conversationID != "" {
			modelstate.ClearConversationModel(conversationID)
		}
		if shouldTemporarilyDisableResponsesModel(statusCode, execErr) {
			modelstate.DisableModelTemporarily(targetModel.ID, modelstate.TemporaryModelDisableTTL)
		}
		// 写入错误日志
		username := ""
		if u := middleware.CurrentUser(c); u != nil {
			username = u.Username
		}
		_ = model.RecordErrorLog(targetModel.ID, username, statusCode, execErr.Error())
		if statusCode >= 400 {
			openaiErrorFromBody(c, statusCode, body)
			return
		}
		openaiError(c, http.StatusBadGateway, "api_error", execErr.Error())
		return
	}

	if c.Request.Context().Err() != nil {
		if streamBody != nil {
			_ = streamBody.Close()
		}
		return
	}

	if statusCode < 200 || statusCode >= 300 {
		if conversationID != "" {
			modelstate.ClearConversationModel(conversationID)
		}
		if shouldTemporarilyDisableResponsesModel(statusCode, nil) {
			modelstate.DisableModelTemporarily(targetModel.ID, modelstate.TemporaryModelDisableTTL)
		}
		// 写入错误日志
		username := ""
		if u := middleware.CurrentUser(c); u != nil {
			username = u.Username
		}
		_ = model.RecordErrorLog(targetModel.ID, username, statusCode, fmt.Sprintf("upstream error status=%d", statusCode))
		openaiErrorFromBody(c, statusCode, body)
		return
	}

	if stream && streamBody != nil {
		defer streamBody.Close()
		c.Header("Content-Type", "text/event-stream")
		utils.ProxySSE(c, trackUsageStream(c, streamBody, targetModel.ID, requestedModel))
		return
	}

	if usedCache {
		utils.Logger.Debugf("[ClaudeRouter] responses: step=cached_model model=%s conversation=%s", targetModel.ID, conversationID)
	}

	if contentType == "" {
		contentType = "application/json"
	}
	recordUsageFromBodyWithModel(c, body, targetModel.ID, requestedModel)
	c.Data(statusCode, contentType, body)
}

func resolveResponseTargetModel(requestedModel, conversationID, inputText string) (*model.Model, bool, error) {
	// 1) 会话缓存优先（仅 combo 路由后写入）
	if conversationID != "" {
		if modelID, ok := modelstate.GetConversationModel(conversationID); ok {
			m, err := model.GetModel(modelID)
			if err == nil && m.Enabled && !modelstate.IsModelTemporarilyDisabled(m.ID) && isCodexResponsesCandidate(m) {
				return m, true, nil
			}
			modelstate.ClearConversationModel(conversationID)
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
				modelstate.SetConversationModel(conversationID, m.ID)

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
		return nil, false, errors.New("model must be operator_id=codex or interface_type in [openai_responses, openai_response, openai, openai_compatible, anthropic]")
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
		strings.EqualFold(it, "openai_compatible") ||
		strings.EqualFold(it, "anthropic")
}

func isAnthropicModel(m *model.Model) bool {
	if m == nil {
		return false
	}
	it := strings.TrimSpace(m.Interface)
	return strings.EqualFold(it, "anthropic")
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

func shouldTemporarilyDisableResponsesModel(statusCode int, err error) bool {
	if statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError {
		return true
	}
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

func (h *CodexProxyHandler) resolveResponsesEndpoint(m *model.Model) (interfaceType, baseURL, apiKey string, err error) {
	interfaceType = "openai_responses"
	if m != nil {
		baseURL = strings.TrimSpace(m.BaseURL)
		apiKey = strings.TrimSpace(m.APIKey)
		interfaceType = strings.TrimSpace(m.Interface)
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
	return interfaceType, baseURL, apiKey, nil
}

func (h *CodexProxyHandler) executeResponsesRequest(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions, adapterMode string) (int, string, []byte, io.ReadCloser, error) {
	switch {
	case strings.HasPrefix(adapterMode, "adapt_"):
		return executeResponsesViaSDKAdapter(ctx, payload, opts, adapterMode)
	case strings.HasPrefix(adapterMode, "passthrough"):
		return executeResponsesPassthrough(ctx, payload, opts)
	default:
		return http.StatusBadRequest, "application/json", nil, nil, fmt.Errorf("unsupported adapter_mode: %s", adapterMode)
	}
}

func executeResponsesPassthrough(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions) (int, string, []byte, io.ReadCloser, error) {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("responses: marshal payload: %w", err)
	}

	upstreamURLs := buildResponsesUpstreamURLs(opts.BaseURL)
	utils.Logger.Debugf("[ClaudeRouter] responses: passthrough urls=%v stream=%v", upstreamURLs, opts.Stream)

	client := &http.Client{Timeout: 30 * time.Minute}

	var resp *http.Response
	var lastErr error
	for i, upstreamURL := range upstreamURLs {
		attempt := i + 1
		utils.Logger.Debugf("[ClaudeRouter] responses: passthrough attempt idx=%d/%d url=%s", attempt, len(upstreamURLs), upstreamURL)

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
		if reqErr != nil {
			lastErr = reqErr
			break
		}
		req.Header.Set("Content-Type", "application/json")
		if opts.Stream {
			req.Header.Set("Accept", "text/event-stream")
		} else {
			req.Header.Set("Accept", "application/json")
		}
		if opts.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+opts.APIKey)
		}

		resp, lastErr = client.Do(req)
		if lastErr != nil {
			continue
		}

		// 404 时尝试下一个候选 URL
		if resp.StatusCode == http.StatusNotFound && i < len(upstreamURLs)-1 {
			_ = resp.Body.Close()
			resp = nil
			continue
		}
		break
	}

	if lastErr != nil || resp == nil {
		return 0, "", nil, nil, fmt.Errorf("responses: upstream request: %w", lastErr)
	}

	if opts.Stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, "text/event-stream", nil, resp.Body, nil
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	bodyPreview := string(body)
	if len(bodyPreview) > 2000 {
		bodyPreview = bodyPreview[:2000] + "...(truncated)"
	}
	utils.Logger.Debugf("[ClaudeRouter] responses: passthrough upstream status=%d body=%s", resp.StatusCode, bodyPreview)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header.Get("Content-Type"), body, nil, fmt.Errorf("responses: upstream error status=%d", resp.StatusCode)
	}
	return resp.StatusCode, "application/json", body, nil, nil
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

func executeResponsesViaSDKAdapter(
	ctx context.Context,
	payload map[string]any,
	opts messages.ExecuteOptions,
	adapterMode string,
) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	originalReqRaw, translatedReqRaw, translateErr := messages.TranslateResponsesRequestForAdapter(payload, opts.UpstreamModel, opts.Stream, adapterMode)
	if translateErr != nil {
		return http.StatusBadRequest, "application/json", nil, nil, fmt.Errorf("responses: invalid translated request payload: %w", translateErr)
	}
	utils.Logger.Debugf("[ClaudeRouter] responses: step=sdk_translated_request mode=%s len=%d body=%s", adapterMode, len(translatedReqRaw), debugBodySnippet(translatedReqRaw, 500))

	upstreamURL, reqHeaders := buildSDKUpstreamRequest(opts.BaseURL, opts.APIKey, adapterMode)
	utils.Logger.Debugf("[ClaudeRouter] responses: step=sdk_dispatch mode=%s upstream_url=%s model=%s stream=%v api_key_set=%v",
		adapterMode, upstreamURL, opts.UpstreamModel, opts.Stream, strings.TrimSpace(opts.APIKey) != "")

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(translatedReqRaw))
	if reqErr != nil {
		return 0, "", nil, nil, fmt.Errorf("responses: create request: %w", reqErr)
	}
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, respErr := client.Do(req)
	if respErr != nil {
		return 0, "", nil, nil, fmt.Errorf("responses: upstream request: %w", respErr)
	}

	statusCode = resp.StatusCode
	contentType = resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	if statusCode < 200 || statusCode >= 300 {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
		if len(body) == 0 {
			body, _ = json.Marshal(gin.H{"error": fmt.Sprintf("upstream error status=%d", statusCode)})
		}
		return statusCode, contentType, body, nil, fmt.Errorf("responses: upstream error status=%d", statusCode)
	}

	if opts.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			proxySDKStreamAsResponses(ctx, resp.Body, pw, opts.UpstreamModel, originalReqRaw, translatedReqRaw, adapterMode)
		}()
		return statusCode, "text/event-stream", nil, pr, nil
	}

	defer resp.Body.Close()
	respBodyRaw, readErr := readSDKNonStreamResponseBody(resp.Body)
	if readErr != nil {
		return 0, "", nil, nil, fmt.Errorf("responses: read response body: %w", readErr)
	}
	utils.Logger.Infof("[ClaudeRouter] responses: step=sdk_upstream_response mode=%s model=%s len=%d body=%s",
		adapterMode, opts.UpstreamModel, len(respBodyRaw), debugBodySnippet(respBodyRaw, 2000))

	responsesRespRaw, translateRespErr := messages.TranslateResponsesNonStreamForClient(
		ctx,
		adapterMode,
		opts.UpstreamModel,
		originalReqRaw,
		translatedReqRaw,
		respBodyRaw,
	)
	if translateRespErr != nil {
		utils.Logger.Errorf("[ClaudeRouter] responses: step=sdk_response_translate_error mode=%s model=%s upstream_body=%s err=%v",
			adapterMode, opts.UpstreamModel, debugBodySnippet(respBodyRaw, 2000), translateRespErr)
		return http.StatusBadGateway, "application/json", nil, nil, fmt.Errorf("responses: %w", translateRespErr)
	}
	utils.Logger.Infof("[ClaudeRouter] responses: step=sdk_translated_response mode=%s model=%s len=%d body=%s",
		adapterMode, opts.UpstreamModel, len(responsesRespRaw), debugBodySnippet(responsesRespRaw, 2000))

	return statusCode, "application/json", responsesRespRaw, nil, nil
}

func buildSDKUpstreamRequest(baseURL, apiKey, adapterMode string) (string, map[string]string) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "text/event-stream",
	}
	base := strings.TrimSpace(baseURL)
	base = strings.TrimSuffix(base, "#")
	base = strings.TrimRight(base, "/")

	switch adapterMode {
	case "adapt_anthropic_sdk":
		if base == "" {
			base = "https://api.anthropic.com"
		}
		lower := strings.ToLower(base)
		switch {
		case strings.HasSuffix(lower, "/v1/messages"):
		case strings.HasSuffix(lower, "/messages"):
			base = strings.TrimSuffix(base, "/messages") + "/v1/messages"
		case strings.HasSuffix(lower, "/v1"):
			base = base + "/messages"
		default:
			base = base + "/v1/messages"
		}
		headers["anthropic-version"] = "2023-06-01"
		if strings.TrimSpace(apiKey) != "" {
			headers["x-api-key"] = strings.TrimSpace(apiKey)
		}
		return base, headers
	default:
		sdkBaseURL := normalizeOpenAICompatibleSDKBaseURL(base)
		if strings.HasSuffix(strings.ToLower(sdkBaseURL), "/v1") {
			sdkBaseURL += "/chat/completions"
		}
		if strings.TrimSpace(apiKey) != "" {
			headers["Authorization"] = "Bearer " + strings.TrimSpace(apiKey)
		}
		return sdkBaseURL, headers
	}
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

func readSDKNonStreamResponseBody(reader io.Reader) ([]byte, error) {
	return io.ReadAll(reader)
}

func proxySDKStreamAsResponses(
	ctx context.Context,
	reader io.Reader,
	out io.Writer,
	model string,
	originalRequestRawJSON []byte,
	translatedRequestRawJSON []byte,
	adapterMode string,
) {
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var state any
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		utils.Logger.Infof("[ClaudeRouter] responses: step=sdk_stream_upstream_chunk mode=%s model=%s chunk=%s",
			adapterMode, model, debugBodySnippet(line, 500))

		chunks, err := messages.TranslateResponsesStreamChunkForClient(
			ctx,
			adapterMode,
			model,
			originalRequestRawJSON,
			translatedRequestRawJSON,
			bytes.Clone(line),
			&state,
		)
		if err != nil {
			utils.Logger.Errorf("[ClaudeRouter] responses: step=sdk_stream_translate_error err=%v", err)
			continue
		}
		for _, chunk := range chunks {
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			normalizedChunk := normalizeResponsesSSEChunk(chunk)
			utils.Logger.Infof("[ClaudeRouter] responses: step=sdk_stream_translated_chunk mode=%s model=%s chunk=%s",
				adapterMode, model, debugBodySnippet([]byte(normalizedChunk), 500))
			_, _ = writer.WriteString(normalizedChunk)
			_ = writer.Flush()
		}
	}

	if scanner.Err() != nil {
		utils.Logger.Errorf("[ClaudeRouter] responses: step=sdk_stream_scan_error err=%v", scanner.Err())
	}
	_, _ = writer.WriteString("data: [DONE]\n\n")
	_ = writer.Flush()
}

func normalizeResponsesSSEChunk(chunk string) string {
	normalized := strings.ReplaceAll(chunk, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, "}event:", "}\n\nevent:")
	normalized = strings.ReplaceAll(normalized, "]event:", "]\n\nevent:")
	normalized = strings.ReplaceAll(normalized, "}data:", "}\n\ndata:")
	normalized = strings.ReplaceAll(normalized, "]data:", "]\n\ndata:")

	lines := strings.Split(normalized, "\n")
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(line, "event:") && strings.Contains(line, " data:") {
			idx := strings.Index(line, " data:")
			out = append(out, strings.TrimSpace(line[:idx]))
			out = append(out, strings.TrimSpace(line[idx+1:]))
			out = append(out, "")
			continue
		}
		out = append(out, line)
	}

	normalized = strings.Join(out, "\n")
	normalized = strings.TrimRight(normalized, "\n")
	return normalized + "\n\n"
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
