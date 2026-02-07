package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

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

	timeout := 60 * time.Second

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
	case "openai", "openai_compatible":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		oai := provider.NewOpenAI(provider.OpenAIConfig{
			Name:           "chat-test",
			APIKey:         apiKey,
			BaseURL:        baseURL,
			TimeoutSeconds: 60,
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
