package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type OpenAIConfig struct {
	Name           string
	APIKey         string
	BaseURL        string // e.g. https://api.openai.com/v1
	TimeoutSeconds int
}

type OpenAI struct {
	name   string
	client *openai.Client
}

func NewOpenAI(cfg OpenAIConfig) *OpenAI {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	timeout := 6000 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	oaiCfg := openai.DefaultConfig(cfg.APIKey)
	oaiCfg.BaseURL = base
	oaiCfg.HTTPClient = &http.Client{
		Timeout: timeout,
	}

	return &OpenAI{
		name:   cfg.Name,
		client: openai.NewClientWithConfig(oaiCfg),
	}
}

func (p *OpenAI) Name() string { return p.name }

func (p *OpenAI) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	oaiReq := openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: toOpenAIMessages(req.Messages),
	}
	if req.MaxTokens > 0 {
		oaiReq.MaxTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		oaiReq.Temperature = float32(*req.Temperature)
	}
	if req.TopP != nil {
		oaiReq.TopP = float32(*req.TopP)
	}

	resp, err := p.client.CreateChatCompletion(ctx, oaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion: %w", err)
	}

	out := &ChatResponse{
		ID:      resp.ID,
		Object:  resp.Object,
		Created: resp.Created,
		Model:   resp.Model,
		Usage: ChatUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	for i, ch := range resp.Choices {
		out.Choices = append(out.Choices, ChatChoice{
			Index: i,
			Message: ChatMessage{
				Role:    ch.Message.Role,
				Content: ch.Message.Content,
			},
			FinishReason: string(ch.FinishReason),
		})
	}
	return out, nil
}

func (p *OpenAI) ChatStream(ctx context.Context, req *ChatRequest) (io.ReadCloser, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	oaiReq := openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: toOpenAIMessages(req.Messages),
		Stream:   true,
	}
	if req.MaxTokens > 0 {
		oaiReq.MaxTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		oaiReq.Temperature = float32(*req.Temperature)
	}
	if req.TopP != nil {
		oaiReq.TopP = float32(*req.TopP)
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, oaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion stream: %w", err)
	}

	// 将 go-openai 的流包装成 ReadCloser，按 OpenAI SSE 规范输出 data: 行
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer stream.Close()

		for {
			chunk, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					_, _ = io.WriteString(pw, "data: [DONE]\n\n")
				}
				return
			}

			b, err := json.Marshal(chunk)
			if err != nil {
				return
			}
			_, _ = io.WriteString(pw, "data: "+string(b)+"\n\n")
		}
	}()

	return pr, nil
}

func toOpenAIMessages(msgs []ChatMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return out
}
