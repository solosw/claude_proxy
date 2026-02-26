package handler

import (
	"awesomeProject/internal/combo"
	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/model"
	"awesomeProject/internal/modelstate"
	"awesomeProject/internal/translator/messages"
	"awesomeProject/pkg/utils"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultOpenAIBaseURL    = "https://api.openai.com"
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultCodexBaseURL     = "https://chatgpt.com/backend-api"
)

// ChatHandler 提供 OpenAI 兼容的 /v1/chat/completions 入口。
type ChatHandler struct {
	cfg *appconfig.Config
}

func NewChatHandler(cfg *appconfig.Config) *ChatHandler {
	return &ChatHandler{cfg: cfg}
}

func (h *ChatHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/v1/chat/completions", h.handleChatCompletions)
	r.OPTIONS("/v1/chat/completions", h.handleOptions)
}

func (h *ChatHandler) RegisterRoutesV1(r gin.IRoutes) {
	r.POST("/chat/completions", h.handleChatCompletions)
	r.OPTIONS("/chat/completions", h.handleOptions)
}

func (h *ChatHandler) handleOptions(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func openaiError(c *gin.Context, status int, errorType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errorType,
			"message": message,
		},
	})
}

func openaiErrorFromBody(c *gin.Context, statusCode int, body []byte) {
	message := extractUpstreamErrorMessage(body)
	errorType := "api_error"
	switch {
	case statusCode == 404:
		errorType = "not_found_error"
	case statusCode == 429:
		errorType = "rate_limit_error"
	case statusCode >= 400 && statusCode < 500:
		errorType = "invalid_request_error"
	}
	openaiError(c, statusCode, errorType, message)
}

func (h *ChatHandler) handleChatCompletions(c *gin.Context) {
	ua := c.GetHeader("User-Agent")
	if len(ua) > 80 {
		ua = ua[:80] + "..."
	}
	utils.Logger.Printf("[ClaudeRouter] chat: request path=%s remote=%s user_agent=%s", c.Request.URL.Path, c.ClientIP(), ua)

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

	conversationID := extractChatMetadataUserID(payload)
	inputText := extractChatInputText(payload)

	targetModel, usedCache, err := resolveChatTargetModel(requestedModel, conversationID, inputText)
	if err != nil {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	c.Set("real_model_id", targetModel.ID)
	if conversationID != "" {
		c.Set("real_conversation_id", conversationID)
	}

	upstreamModel := strings.TrimSpace(targetModel.UpstreamID)
	if upstreamModel == "" {
		upstreamModel = targetModel.ID
	}

	interfaceType, baseURL, apiKey, resolveErr := h.resolveChatEndpoint(targetModel)
	if resolveErr != nil {
		openaiError(c, http.StatusBadRequest, "invalid_request_error", resolveErr.Error())
		return
	}

	payloadToSend := applyChatForwardExtendedFields(payload, targetModel.ForwardMetadata, targetModel.ForwardThinking)
	payloadToSend["model"] = upstreamModel

	waitModelQPS(c.Request.Context(), targetModel.ID, targetModel.MaxQPS)
	if c.Request.Context().Err() != nil {
		return
	}

	utils.Logger.Printf("[ClaudeRouter] chat: step=execute interface=%s model=%s upstream=%s stream=%v", interfaceType, targetModel.ID, upstreamModel, stream)

	statusCode, contentType, body, streamBody, execErr := h.executeChatRequest(
		c.Request.Context(),
		payloadToSend,
		messages.ExecuteOptions{
			UpstreamModel: upstreamModel,
			APIKey:        apiKey,
			BaseURL:       baseURL,
			Stream:        stream,
		},
		interfaceType,
	)

	if execErr != nil {
		if conversationID != "" {
			modelstate.ClearConversationModel(conversationID)
		}
		if shouldTemporarilyDisableChatModel(statusCode, execErr) {
			modelstate.DisableModelTemporarily(targetModel.ID, modelstate.TemporaryModelDisableTTL)
		}
		if c.Request.Context().Err() != nil {
			return
		}
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
		if shouldTemporarilyDisableChatModel(statusCode, nil) {
			modelstate.DisableModelTemporarily(targetModel.ID, modelstate.TemporaryModelDisableTTL)
		}
		openaiErrorFromBody(c, statusCode, body)
		return
	}

	if stream && streamBody != nil {
		defer streamBody.Close()
		c.Header("Content-Type", "text/event-stream")
		utils.ProxySSE(c, trackUsageStream(c, streamBody, targetModel.ID))
		return
	}

	if usedCache {
		utils.Logger.Printf("[ClaudeRouter] chat: step=cached_model model=%s conversation=%s", targetModel.ID, conversationID)
	}

	if contentType == "" {
		contentType = "application/json"
	}
	recordUsageFromBodyWithModel(c, body, targetModel.ID)
	c.Data(statusCode, contentType, body)
}

func resolveChatTargetModel(requestedModel, conversationID, inputText string) (*model.Model, bool, error) {
	if model.IsComboID(requestedModel) && conversationID != "" {
		if cachedID, ok := modelstate.GetConversationModel(conversationID); ok {
			cb, cbErr := model.GetCombo(requestedModel)
			m, err := model.GetModel(cachedID)
			if cbErr == nil && cb != nil && comboContainsModelID(cb, cachedID) && err == nil && m != nil && m.Enabled && !modelstate.IsModelTemporarilyDisabled(m.ID) {
				return m, true, nil
			}
			modelstate.ClearConversationModel(conversationID)
		}
	}

	if !model.IsComboID(requestedModel) {
		m, err := model.GetModel(requestedModel)
		if err != nil || m == nil {
			return nil, false, fmt.Errorf("unknown model: %s", requestedModel)
		}
		if !m.Enabled {
			return nil, false, fmt.Errorf("model disabled: %s", m.ID)
		}
		if modelstate.IsModelTemporarilyDisabled(m.ID) {
			return nil, false, fmt.Errorf("model temporarily disabled: %s", m.ID)
		}
		return m, false, nil
	}

	cb, err := model.GetCombo(requestedModel)
	if err != nil || cb == nil {
		return nil, false, fmt.Errorf("unknown model: %s", requestedModel)
	}
	if !cb.Enabled {
		return nil, false, fmt.Errorf("model disabled: %s", requestedModel)
	}

	filtered := make([]model.ComboItem, 0, len(cb.Items))
	for _, it := range cb.Items {
		modelID := strings.TrimSpace(it.ModelID)
		if modelID == "" {
			continue
		}
		m, err := model.GetModel(modelID)
		if err != nil || m == nil || !m.Enabled || modelstate.IsModelTemporarilyDisabled(m.ID) {
			continue
		}
		filtered = append(filtered, it)
	}
	if len(filtered) == 0 {
		return nil, false, fmt.Errorf("combo has no available models")
	}

	tmp := &model.Combo{ID: cb.ID, Name: cb.Name, Description: cb.Description, Enabled: cb.Enabled, Items: filtered}
	chosenID := combo.ChooseModelID(tmp, inputText)
	if chosenID == "" {
		return nil, false, fmt.Errorf("combo has no selectable items")
	}

	m, err := model.GetModel(chosenID)
	if err != nil || m == nil {
		return nil, false, fmt.Errorf("combo item model not found: %s", chosenID)
	}
	if conversationID != "" {
		modelstate.SetConversationModel(conversationID, m.ID)
	}
	return m, false, nil
}

func (h *ChatHandler) resolveChatEndpoint(targetModel *model.Model) (interfaceType, baseURL, apiKey string, err error) {
	if targetModel == nil {
		return "", "", "", fmt.Errorf("model not found")
	}

	interfaceType = strings.TrimSpace(targetModel.Interface)
	baseURL = strings.TrimRight(strings.TrimSpace(targetModel.BaseURL), "/")
	apiKey = strings.TrimSpace(targetModel.APIKey)

	if operatorID := strings.TrimSpace(targetModel.OperatorID); operatorID != "" {
		if h.cfg == nil || h.cfg.Operators == nil {
			return "", "", "", fmt.Errorf("operator config not available")
		}
		ep, ok := h.cfg.Operators[operatorID]
		if !ok {
			return "", "", "", fmt.Errorf("operator not found: %s", operatorID)
		}
		if !ep.Enabled {
			return "", "", "", fmt.Errorf("operator disabled: %s", operatorID)
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
	}

	if interfaceType == "" {
		interfaceType = "openai"
	}
	switch {
	case strings.EqualFold(interfaceType, "openai_response"):
		interfaceType = "openai_responses"
	}

	if baseURL == "" {
		switch {
		case strings.EqualFold(interfaceType, "anthropic"):
			baseURL = defaultAnthropicBaseURL
		case strings.EqualFold(interfaceType, "openai_responses"):
			baseURL = defaultCodexBaseURL
		default:
			baseURL = defaultOpenAIBaseURL
		}
	}

	return interfaceType, baseURL, apiKey, nil
}

func (h *ChatHandler) executeChatRequest(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions, interfaceType string) (int, string, []byte, io.ReadCloser, error) {
	switch {
	case strings.EqualFold(interfaceType, "openai") || strings.EqualFold(interfaceType, "openai_compatible"):
		return executeOpenAIChatPassthrough(ctx, payload, opts)
	case strings.EqualFold(interfaceType, "anthropic"):
		return executeOpenAIChatViaAnthropic(ctx, payload, opts)
	case strings.EqualFold(interfaceType, "openai_responses"):
		return executeOpenAIChatViaOpenAIResponses(ctx, payload, opts)
	default:
		return http.StatusBadRequest, "application/json", nil, nil, fmt.Errorf("unsupported interface_type: %s", interfaceType)
	}
}

func executeOpenAIChatPassthrough(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions) (int, string, []byte, io.ReadCloser, error) {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: marshal payload: %w", err)
	}

	upstreamURL := buildOpenAIChatCompletionsURL(opts.BaseURL)
	utils.Logger.Printf("[ClaudeRouter] chat: passthrough url=%s stream=%v", upstreamURL, opts.Stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: upstream request: %w", err)
	}

	if opts.Stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, "text/event-stream", nil, resp.Body, nil
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// 打印上游原始响应（用于调试）
	bodyPreview := string(body)
	if len(bodyPreview) > 2000 {
		bodyPreview = bodyPreview[:2000] + "...(truncated)"
	}
	utils.Logger.Printf("[ClaudeRouter] chat: passthrough upstream status=%d body=%s", resp.StatusCode, bodyPreview)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header.Get("Content-Type"), body, nil, fmt.Errorf("chat: upstream error status=%d", resp.StatusCode)
	}
	return resp.StatusCode, "application/json", body, nil, nil
}

func executeOpenAIChatViaAnthropic(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions) (int, string, []byte, io.ReadCloser, error) {
	originalReq, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: marshal payload: %w", err)
	}

	translatedReq, err := messages.ConvertOpenAIChatToAnthropicRequest(originalReq, messages.OpenAIChatTranslateOptions{
		UpstreamModel: opts.UpstreamModel,
		Stream:        opts.Stream,
	})
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: convert openai->anthropic request: %w", err)
	}

	upstreamURL := buildAnthropicMessagesURL(opts.BaseURL)
	utils.Logger.Printf("[ClaudeRouter] chat: anthropic url=%s stream=%v", upstreamURL, opts.Stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(translatedReq))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	if opts.APIKey != "" {
		req.Header.Set("x-api-key", opts.APIKey)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: upstream request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		bodyPreview := string(body)
		if len(bodyPreview) > 2000 {
			bodyPreview = bodyPreview[:2000] + "...(truncated)"
		}
		utils.Logger.Printf("[ClaudeRouter] chat: anthropic upstream error status=%d body=%s", resp.StatusCode, bodyPreview)
		return resp.StatusCode, resp.Header.Get("Content-Type"), body, nil, fmt.Errorf("chat: upstream error status=%d", resp.StatusCode)
	}

	if opts.Stream {
		utils.Logger.Printf("[ClaudeRouter] chat: anthropic stream started, piping to client")
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			utils.Logger.Printf("[ClaudeRouter] chat: anthropic stream goroutine started")
			if err := messages.TranslateAnthropicStreamToOpenAIChat(ctx, resp.Body, pw, opts.UpstreamModel, originalReq, translatedReq); err != nil {
				utils.Logger.Printf("[ClaudeRouter] chat: stream anthropic->openai translate error=%v", err)
			} else {
				utils.Logger.Printf("[ClaudeRouter] chat: anthropic stream translation completed successfully")
			}
		}()
		return resp.StatusCode, "text/event-stream", nil, pr, nil
	}

	defer resp.Body.Close()
	upstreamBody, _ := io.ReadAll(resp.Body)

	// 打印 Anthropic 上游原始响应
	upstreamPreview := string(upstreamBody)
	if len(upstreamPreview) > 2000 {
		upstreamPreview = upstreamPreview[:2000] + "...(truncated)"
	}
	utils.Logger.Printf("[ClaudeRouter] chat: anthropic upstream status=%d body=%s", resp.StatusCode, upstreamPreview)

	convertedBody, err := messages.ConvertAnthropicToOpenAIChatResponse(ctx, opts.UpstreamModel, originalReq, translatedReq, upstreamBody)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: convert anthropic->openai response: %w", err)
	}

	// 打印转换后的 OpenAI 响应
	convertedPreview := string(convertedBody)
	if len(convertedPreview) > 2000 {
		convertedPreview = convertedPreview[:2000] + "...(truncated)"
	}
	utils.Logger.Printf("[ClaudeRouter] chat: anthropic converted body=%s", convertedPreview)

	return resp.StatusCode, "application/json", convertedBody, nil, nil
}

func executeOpenAIChatViaOpenAIResponses(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions) (int, string, []byte, io.ReadCloser, error) {
	originalReq, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: marshal payload: %w", err)
	}

	translatedReq, err := messages.ConvertOpenAIChatToOpenAIResponsesRequest(originalReq, messages.OpenAIChatTranslateOptions{
		UpstreamModel: opts.UpstreamModel,
		Stream:        opts.Stream,
	})
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: convert openai->responses request: %w", err)
	}

	upstreamURL := buildCodexResponsesURL(opts.BaseURL)
	utils.Logger.Printf("[ClaudeRouter] chat: responses url=%s stream=%v", upstreamURL, opts.Stream)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(translatedReq))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: create request: %w", err)
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

	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: upstream request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		bodyPreview := string(body)
		if len(bodyPreview) > 2000 {
			bodyPreview = bodyPreview[:2000] + "...(truncated)"
		}
		utils.Logger.Printf("[ClaudeRouter] chat: responses upstream error status=%d body=%s", resp.StatusCode, bodyPreview)
		return resp.StatusCode, resp.Header.Get("Content-Type"), body, nil, fmt.Errorf("chat: upstream error status=%d", resp.StatusCode)
	}

	if opts.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			if err := messages.TranslateOpenAIResponsesStreamToOpenAIChat(ctx, resp.Body, pw, opts.UpstreamModel, originalReq, translatedReq); err != nil {
				utils.Logger.Printf("[ClaudeRouter] chat: stream responses->openai translate error=%v", err)
			}
		}()
		return resp.StatusCode, "text/event-stream", nil, pr, nil
	}

	defer resp.Body.Close()
	upstreamBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))

	// 打印上游原始响应
	upstreamPreview := string(upstreamBody)
	if len(upstreamPreview) > 2000 {
		upstreamPreview = upstreamPreview[:2000] + "...(truncated)"
	}
	utils.Logger.Printf("[ClaudeRouter] chat: responses upstream status=%d body=%s", resp.StatusCode, upstreamPreview)

	translateBody := upstreamBody
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") || looksLikeSSEPayload(upstreamBody) {
		completedJSON, err := messages.ReadOpenAIResponsesCompletedEvent(bytes.NewReader(upstreamBody))
		if err != nil {
			var eventErr *messages.ResponsesCompletedEventError
			if errors.As(err, &eventErr) {
				return http.StatusBadRequest, "application/json", eventErr.Body, nil, fmt.Errorf("chat: responses upstream event error: %s", eventErr.EventType)
			}
			return 0, "", nil, nil, fmt.Errorf("chat: read responses completed event: %w", err)
		}
		translateBody = completedJSON
	}

	convertedBody, err := messages.ConvertOpenAIResponsesToOpenAIChatResponse(ctx, opts.UpstreamModel, originalReq, translatedReq, translateBody)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("chat: convert responses->openai response: %w", err)
	}

	// 打印转换后的 OpenAI 响应
	convertedPreview := string(convertedBody)
	if len(convertedPreview) > 2000 {
		convertedPreview = convertedPreview[:2000] + "...(truncated)"
	}
	utils.Logger.Printf("[ClaudeRouter] chat: responses converted body=%s", convertedPreview)

	return resp.StatusCode, "application/json", convertedBody, nil, nil
}

func buildOpenAIChatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "/chat/completions"):
		return base
	case strings.HasSuffix(lower, "/v1"):
		return base + "/chat/completions"
	default:
		return base + "/v1/chat/completions"
	}
}

func buildAnthropicMessagesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultAnthropicBaseURL
	}
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "/messages"):
		return base
	case strings.HasSuffix(lower, "/v1"):
		return base + "/messages"
	default:
		return base + "/v1/messages"
	}
}

func buildCodexResponsesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultCodexBaseURL
	}
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "/responses"):
		return base
	case strings.HasSuffix(lower, "/v1"):
		return base + "/responses"
	case strings.HasSuffix(lower, "/backend-api/v1"):
		return base + "/responses"
	default:
		return base + "/v1/responses"
	}
}

func applyChatForwardExtendedFields(payload map[string]any, forwardMetadata, forwardThinking bool) map[string]any {
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

func extractChatInputText(payload map[string]any) string {
	msgs, ok := payload["messages"].([]any)
	if !ok {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		msg, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msg["role"].(string); !strings.EqualFold(strings.TrimSpace(role), "user") {
			continue
		}
		switch content := msg["content"].(type) {
		case string:
			return strings.TrimSpace(content)
		case []any:
			for _, item := range content {
				blk, ok := item.(map[string]any)
				if !ok {
					continue
				}
				t, _ := blk["type"].(string)
				if !strings.EqualFold(strings.TrimSpace(t), "text") {
					continue
				}
				if text, _ := blk["text"].(string); strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	return ""
}

func extractChatMetadataUserID(payload map[string]any) string {
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

func shouldTemporarilyDisableChatModel(statusCode int, err error) bool {
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

func comboContainsModelID(cb *model.Combo, modelID string) bool {
	if cb == nil {
		return false
	}
	needle := strings.TrimSpace(modelID)
	if needle == "" {
		return false
	}
	for _, it := range cb.Items {
		if strings.EqualFold(strings.TrimSpace(it.ModelID), needle) {
			return true
		}
	}
	return false
}

func looksLikeSSEPayload(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) || bytes.HasPrefix(trimmed, []byte("event:")) {
		return true
	}
	return bytes.Contains(trimmed, []byte("\ndata:")) || bytes.Contains(trimmed, []byte("\nevent:"))
}
