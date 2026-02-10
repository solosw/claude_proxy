package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AnthropicConfig struct {
	Name           string
	APIKey         string
	BaseURL        string // e.g. https://api.anthropic.com
	TimeoutSeconds int
	APIVersion     string // e.g. 2023-06-01
}

type Anthropic struct {
	name       string
	apiKey     string
	baseURL    string
	apiVersion string
	client     *http.Client
}

func NewAnthropic(cfg AnthropicConfig) *Anthropic {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	timeout := 600 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	version := cfg.APIVersion
	if version == "" {
		version = "2023-06-01"
	}

	return &Anthropic{
		name:       cfg.Name,
		apiKey:     cfg.APIKey,
		baseURL:    base,
		apiVersion: version,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *Anthropic) Name() string { return p.name }

func (p *Anthropic) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	payload := anthropicMessagesRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  toAnthropicMessages(req.Messages),
	}

	return p.doMessages(ctx, payload)
}

func (p *Anthropic) ChatStream(ctx context.Context, req *ChatRequest) (io.ReadCloser, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	payload := anthropicMessagesRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  toAnthropicMessages(req.Messages),
		Stream:    true,
	}

	return p.doMessagesStream(ctx, payload)
}

type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
	Extra     map[string]any     `json:"-"`
	Metadata  map[string]string  `json:"metadata,omitempty"`
	StopSeq   []string           `json:"stop_sequences,omitempty"`
}

type anthropicMessage struct {
	Role    string                 `json:"role"`
	Content []anthropicTextContent `json:"content"`
}

type anthropicTextContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

func toAnthropicMessages(msgs []ChatMessage) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		// Anthropic 要求 content 为块数组，这里简化为单一 text 块。
		out = append(out, anthropicMessage{
			Role: m.Role,
			Content: []anthropicTextContent{
				{Type: "text", Text: m.Content},
			},
		})
	}
	return out
}

type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicMessageResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage anthropicUsage `json:"usage"`
}

func (p *Anthropic) doMessages(ctx context.Context, payload anthropicMessagesRequest) (*ChatResponse, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.addHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request anthropic messages: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, p.decodeAnthropicError(resp.StatusCode, body)
	}

	var ar anthropicMessageResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var text string
	for _, c := range ar.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	cr := &ChatResponse{
		ID:     ar.ID,
		Object: ar.Type,
		Model:  ar.Model,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    ar.Role,
					Content: text,
				},
			},
		},
		Usage: ChatUsage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		},
	}

	return cr, nil
}

func (p *Anthropic) doMessagesStream(ctx context.Context, payload anthropicMessagesRequest) (io.ReadCloser, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.addHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request anthropic messages (stream): %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("anthropic stream status=%d and failed reading body: %v", resp.StatusCode, readErr)
		}
		return nil, p.decodeAnthropicError(resp.StatusCode, body)
	}

	// 上层负责读取并关闭 body。
	return resp.Body, nil
}

func (p *Anthropic) addHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		r.Header.Set("x-api-key", p.apiKey)
	}
	if p.apiVersion != "" {
		r.Header.Set("anthropic-version", p.apiVersion)
	}
}

func (p *Anthropic) decodeAnthropicError(status int, body []byte) error {
	var er anthropicErrorResponse
	if err := json.Unmarshal(body, &er); err == nil && er.Error.Message != "" {
		return fmt.Errorf("anthropic error (status=%d type=%s): %s", status, er.Error.Type, er.Error.Message)
	}
	trim := strings.TrimSpace(string(body))
	if trim == "" {
		trim = "<empty body>"
	}
	return fmt.Errorf("anthropic error (status=%d): %s", status, trim)
}
