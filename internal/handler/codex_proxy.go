package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

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
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	requestedModel, _ := payload["model"].(string)
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model"})
		return
	}

	conversationID := extractResponsesMetadataUserID(payload)
	inputText := extractResponsesInputText(payload)

	targetModel, usedCache, err := h.resolveResponseTargetModel(requestedModel, conversationID, inputText)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	baseURL := ""
	apiKey := ""
	upstreamModel := requestedModel

	if targetModel != nil {
		baseURL, apiKey = h.resolveCodexEndpoint(targetModel)
		upstreamModel = strings.TrimSpace(targetModel.UpstreamID)
		if upstreamModel == "" {
			upstreamModel = targetModel.ID
		}
	} else {
		// model 不在本地表时允许直接透传，使用 operators.codex 配置。
		baseURL, apiKey = h.resolveCodexEndpoint(nil)
	}
	payloadToSend := applyResponsesAdapter(payload, upstreamModel, targetModel)

	reqBody, err := json.Marshal(payloadToSend)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	upstreamURL := strings.TrimRight(baseURL, "/") + "/v1/responses"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create upstream request failed"})
		return
	}
	copyRequestHeaders(c.Request.Header, req.Header)
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		if conversationID != "" && usedCache {
			codexConversationModelMu.Lock()
			delete(codexConversationModel, conversationID)
			codexConversationModelMu.Unlock()
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	stream := false
	if v, ok := payloadToSend["stream"].(bool); ok {
		stream = v
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		stream = true
	}

	if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
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
		}
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
				if err == nil && m.Enabled && isCodexResponsesCandidate(m) {
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
				m, err := model.GetModel(strings.TrimSpace(it.ModelID))
				if err != nil || m == nil || !m.Enabled || !isCodexResponsesCandidate(m) {
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
		return nil, false, errors.New("model disabled: " + m.ID)
	}
	if !isCodexResponsesCandidate(m) {
		return nil, false, errors.New("model must be operator_id=codex or interface_type in [openai_responses, openai]")
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
		if m == nil || !m.Enabled || !isCodexResponsesCandidate(m) {
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
	if !isOpenAICompatibleModel(m) {
		return out
	}
	return normalizeOpenAICompatibleResponsesPayload(out)
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

	// 兼容 max_tokens 字段。
	if _, ok := out["max_output_tokens"]; !ok {
		if v, ok := out["max_tokens"]; ok {
			out["max_output_tokens"] = v
		}
	}

	// 兼容 chat 风格的 messages 输入。
	if _, ok := out["input"]; !ok {
		if msgs, ok := out["messages"]; ok {
			out["input"] = messagesToResponsesInput(msgs)
		}
	}

	if tools, ok := out["tools"]; ok {
		out["tools"] = normalizeResponsesTools(tools)
	}
	if toolChoice, ok := out["tool_choice"]; ok {
		out["tool_choice"] = normalizeResponsesToolChoice(toolChoice)
	}
	return out
}

func messagesToResponsesInput(messages any) any {
	rawMsgs, ok := messages.([]any)
	if !ok {
		return messages
	}
	out := make([]any, 0, len(rawMsgs))
	for _, it := range rawMsgs {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(getStringMapValue(m, "role"))
		if role == "" {
			role = "user"
		}
		switch v := m["content"].(type) {
		case string:
			txt := strings.TrimSpace(v)
			if txt == "" {
				txt = v
			}
			out = append(out, map[string]any{
				"role": role,
				"content": []any{
					map[string]any{"type": "input_text", "text": txt},
				},
			})
		default:
			out = append(out, map[string]any{
				"role":    role,
				"content": v,
			})
		}
	}
	return out
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
			out = append(out, norm)
			continue
		}

		out = append(out, m)
	}
	return out
}

func normalizeResponsesToolChoice(toolChoice any) any {
	m, ok := toolChoice.(map[string]any)
	if !ok {
		return toolChoice
	}
	if !strings.EqualFold(strings.TrimSpace(getStringMapValue(m, "type")), "function") {
		return toolChoice
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

func (h *CodexProxyHandler) resolveCodexEndpoint(m *model.Model) (baseURL, apiKey string) {
	if m != nil {
		baseURL = strings.TrimSpace(m.BaseURL)
		apiKey = strings.TrimSpace(m.APIKey)
	}
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
