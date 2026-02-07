package provider

import (
	"context"
	"io"
)

// Provider 定义了模型服务商的最小接口。
//
// - Chat: 非流式请求，返回结构化响应
// - ChatStream: 流式请求，返回服务商的原始 SSE body（由上层负责转发与关闭）
type Provider interface {
	Name() string
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (io.ReadCloser, error)
}

type ChatMessage struct {
	Role    string `json:"role"`    // system | user | assistant | tool ...
	Content string `json:"content"` // 简化为纯文本，后续可扩展为多段/多模态
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float32      `json:"temperature,omitempty"`
	TopP        *float32      `json:"top_p,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// ChatResponse 尽量对齐 OpenAI Chat Completions 响应结构，方便后续 translator 与 proxy。
type ChatResponse struct {
	ID      string       `json:"id,omitempty"`
	Object  string       `json:"object,omitempty"`
	Created int64        `json:"created,omitempty"`
	Model   string       `json:"model,omitempty"`
	Choices []ChatChoice `json:"choices,omitempty"`
	Usage   ChatUsage    `json:"usage,omitempty"`
}
