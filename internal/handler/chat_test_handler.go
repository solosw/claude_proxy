package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"awesomeProject/internal/model"
	"awesomeProject/internal/provider"
	"awesomeProject/pkg/utils"
)

// ChatTestHandler 用于前端对单个模型进行聊天测试（支持 SSE）。
type ChatTestHandler struct {
}

func NewChatTestHandler() *ChatTestHandler {
	return &ChatTestHandler{}
}

func (h *ChatTestHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/api/chat/test", h.handleChatTest)
}

type chatTestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatTestRequest struct {
	ModelID     string            `json:"model_id"`
	Messages    []chatTestMessage `json:"messages"`
	Stream      bool              `json:"stream"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
}

func (h *ChatTestHandler) handleChatTest(c *gin.Context) {
	var req chatTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.ModelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model_id"})
		return
	}

	m, err := model.GetModel(req.ModelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	if !m.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model disabled"})
		return
	}

	interfaceType := strings.TrimSpace(m.Interface)
	if interfaceType == "" {
		// 默认按 OpenAI 协议走
		interfaceType = "openai"
	}

	// 从模型配置读取 api_key / base_url
	apiKey := strings.TrimSpace(m.APIKey)
	baseURL := strings.TrimRight(strings.TrimSpace(m.BaseURL), "/")

	timeout := 600 * time.Second

	upstreamModel := strings.TrimSpace(m.UpstreamID)
	if upstreamModel == "" {
		upstreamModel = m.ID
	}

	switch interfaceType {
	case "anthropic":
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		if apiKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model api_key not configured"})
			return
		}
		client := &http.Client{Timeout: timeout}
		h.proxyAnthropicMessages(c, client, baseURL, apiKey, upstreamModel, req)
		return
	case "openai_responses":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		if apiKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model api_key not configured"})
			return
		}
		httpClient := &http.Client{Timeout: timeout}
		h.proxyOpenAIResponsesWithSDK(c, httpClient, baseURL, apiKey, upstreamModel, req)
		return
	case "openai", "openai_compatible":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		oai := provider.NewOpenAI(provider.OpenAIConfig{
			Name:           "chat-test",
			APIKey:         apiKey,
			BaseURL:        baseURL,
			TimeoutSeconds: 600,
		})

		log.Printf("model:%v", upstreamModel)
		chatReq := &provider.ChatRequest{
			Model:     upstreamModel,
			MaxTokens: req.MaxTokens,
			Stream:    req.Stream,
		}
		if req.Temperature != nil {
			temp := float32(*req.Temperature)
			chatReq.Temperature = &temp
		}
		msgs := make([]provider.ChatMessage, 0, len(req.Messages))
		for _, m := range req.Messages {
			msgs = append(msgs, provider.ChatMessage{Role: m.Role, Content: m.Content})
		}
		chatReq.Messages = msgs

		if req.Stream {
			body, err := oai.ChatStream(c.Request.Context(), chatReq)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
				return
			}
			utils.ProxySSE(c, body)
			return
		}

		resp, err := oai.Chat(c.Request.Context(), chatReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
		return
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported interface_type: " + interfaceType})
		return
	}
}

func (h *ChatTestHandler) proxyOpenAIChatCompletions(
	c *gin.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
	upstreamModel string,
	req chatTestRequest,
) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type openaiReq struct {
		Model       string   `json:"model"`
		Messages    []msg    `json:"messages"`
		Stream      bool     `json:"stream,omitempty"`
		MaxTokens   int      `json:"max_tokens,omitempty"`
		Temperature *float64 `json:"temperature,omitempty"`
	}

	msgs := make([]msg, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, msg{Role: m.Role, Content: m.Content})
	}
	payload := openaiReq{
		Model:       upstreamModel,
		Messages:    msgs,
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encode payload failed"})
		return
	}

	url := baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create upstream request failed"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	if req.Stream {
		utils.ProxySSE(c, resp.Body)
		return
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	c.Data(resp.StatusCode, ct, body)
}

func (h *ChatTestHandler) proxyAnthropicMessages(
	c *gin.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
	upstreamModel string,
	req chatTestRequest,
) {
	type block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type amsg struct {
		Role    string  `json:"role"`
		Content []block `json:"content"`
	}
	type anthropicReq struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []amsg `json:"messages"`
		Stream    bool   `json:"stream,omitempty"`
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	msgs := make([]amsg, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, amsg{
			Role: m.Role,
			Content: []block{
				{Type: "text", Text: m.Content},
			},
		})
	}
	payload := anthropicReq{
		Model:     upstreamModel,
		MaxTokens: maxTokens,
		Messages:  msgs,
		Stream:    req.Stream,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encode payload failed"})
		return
	}

	url := baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create upstream request failed"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if apiKey != "" {
		httpReq.Header.Set("x-api-key", apiKey)
	}
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	if req.Stream {
		utils.ProxySSE(c, resp.Body)
		return
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	c.Data(resp.StatusCode, ct, body)
}

// proxyOpenAIResponsesWithSDK 使用 OpenAI 官方 go 包调用 Responses API（/v1/responses）。
func (h *ChatTestHandler) proxyOpenAIResponsesWithSDK(
	c *gin.Context,
	httpClient *http.Client,
	baseURL string,
	apiKey string,
	upstreamModel string,
	req chatTestRequest,
) {
	ctx := c.Request.Context()
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(httpClient),
	)

	// 将 messages 转为 Responses API 的 input（OfInputItemList）
	inputList := make(responses.ResponseInputParam, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "user"
		}
		msgRole := responses.EasyInputMessageRoleUser
		switch strings.ToLower(role) {
		case "assistant":
			msgRole = responses.EasyInputMessageRoleAssistant
		case "system":
			msgRole = responses.EasyInputMessageRoleSystem
		case "developer":
			msgRole = responses.EasyInputMessageRoleDeveloper
		default:
			msgRole = responses.EasyInputMessageRoleUser
		}
		inputList = append(inputList, responses.ResponseInputItemParamOfMessage(m.Content, msgRole))
	}
	inputUnion := responses.ResponseNewParamsInputUnion{}
	inputUnion.OfInputItemList = inputList

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(upstreamModel),
		Input: inputUnion,
	}
	if req.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
	} else {
		params.MaxOutputTokens = openai.Int(1024)
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	if req.Stream {
		h.streamOpenAIResponses(c, &client, params)
		return
	}

	resp, err := client.Responses.New(ctx, params)
	if err != nil {
		h.writeOpenAIError(c, err)
		return
	}
	// 前端 openai_responses 非流式期望 message.content
	c.JSON(http.StatusOK, gin.H{"message": gin.H{"content": resp.OutputText()}})
}

func (h *ChatTestHandler) streamOpenAIResponses(c *gin.Context, client *openai.Client, params responses.ResponseNewParams) {
	ctx := c.Request.Context()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	stream := client.Responses.NewStreaming(ctx, params)
	for stream.Next() {
		event := stream.Current()
		if event.Type == "response.output_text.delta" && event.Delta != "" {
			data := map[string]any{
				"type": "content_delta",
				"delta": map[string]any{"type": "text_delta", "text": event.Delta},
			}
			b, _ := json.Marshal(data)
			_, _ = c.Writer.Write([]byte("event: content_delta\n"))
			_, _ = c.Writer.Write(append(append([]byte("data: "), b...), '\n', '\n'))
			flusher.Flush()
		}
	}
	if err := stream.Err(); err != nil {
		log.Printf("[chat_test] openai_responses stream err: %v", err)
		// 已开始写流，只能写一条错误事件
		errData, _ := json.Marshal(map[string]any{"type": "error", "message": err.Error()})
		_, _ = c.Writer.Write([]byte("event: error\n"))
		_, _ = c.Writer.Write(append(append([]byte("data: "), errData...), '\n', '\n'))
		flusher.Flush()
	}
}

func (h *ChatTestHandler) writeOpenAIError(c *gin.Context, err error) {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		body := apiErr.DumpResponse(false)
		var errObj struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &errObj)
		msg := errObj.Error.Message
		if msg == "" {
			msg = string(body)
		}
		if msg == "" {
			msg = err.Error()
		}
		c.JSON(status, gin.H{"error": gin.H{"message": msg, "type": "upstream_error"}})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": err.Error(), "type": "upstream_error"}})
}
