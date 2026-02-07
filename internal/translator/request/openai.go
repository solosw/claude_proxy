package request

import "awesomeProject/internal/provider"

// OpenAIChatCompletionMessage 对应 OpenAI Chat Completions 的消息结构（简化版）。
type OpenAIChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIChatCompletionRequest 对应 /v1/chat/completions 的请求体（只保留当前需要的字段）。
type OpenAIChatCompletionRequest struct {
	Model       string                        `json:"model"`
	Messages    []OpenAIChatCompletionMessage `json:"messages"`
	MaxTokens   int                           `json:"max_tokens,omitempty"`
	Temperature *float32                      `json:"temperature,omitempty"`
	TopP        *float32                      `json:"top_p,omitempty"`
	Stream      bool                          `json:"stream,omitempty"`
}

// ToChatRequest 转换为内部统一的 ChatRequest。
func (r *OpenAIChatCompletionRequest) ToChatRequest() *provider.ChatRequest {
	if r == nil {
		return nil
	}
	msgs := make([]provider.ChatMessage, 0, len(r.Messages))
	for _, m := range r.Messages {
		msgs = append(msgs, provider.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return &provider.ChatRequest{
		Model:       r.Model,
		Messages:    msgs,
		MaxTokens:   r.MaxTokens,
		Temperature: r.Temperature,
		TopP:        r.TopP,
		Stream:      r.Stream,
	}
}
