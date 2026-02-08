package messages

import (
	"context"
	"io"
	"log"
)

// ExecuteOptions 上游请求所需配置（由 handler 从模型配置填入）。
type ExecuteOptions struct {
	UpstreamModel string
	APIKey        string
	BaseURL       string
	Stream        bool
	// MinimalOpenAI 为 true 时，OpenAI 适配器只发 model/messages/max_tokens/stream/temperature/top_p，不带 tools、tool_choice，避免部分网关（如 NewAPI）WAF 误拦。
	MinimalOpenAI bool
}

// Adapter 协议适配器：入口为 Anthropic /v1/messages 格式，通过 SDK 请求上游并返回 Anthropic 格式。
// 使用工具包（anthropic-sdk-go / go-openai）发起请求，不手写 HTTP 协议。
type Adapter interface {
	// Execute 使用对应 SDK 完成一次请求，返回状态码、Content-Type、非流式 body 或流式 reader。
	// 非流式时 streamBody 为 nil；流式时 body 为空、streamBody 非 nil，调用方负责 Close。
	Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error)
}

// logStep 统一打印步骤日志，便于排查 404/协议问题。
func logStep(step string, args ...any) {
	log.Printf("[ClaudeRouter] messages: "+step, args...)
}

// Registry 按 interface_type 获取适配器。默认使用 anthropic。
var Registry = NewRegistryMap()

// RegistryMap 维护 interface_type -> Adapter 的映射。
type RegistryMap struct {
	adapters map[string]Adapter
	default_ string
}

// NewRegistryMap 创建注册表并注册内置适配器。
func NewRegistryMap() *RegistryMap {
	r := &RegistryMap{
		adapters: make(map[string]Adapter),
		default_: "anthropic",
	}
	r.Register("anthropic", &AnthropicAdapter{})
	r.Register("openai", &OpenAIAdapter{})
	r.Register("openai_compatible", &OpenAIAdapter{})
	return r
}

// Register 注册 interfaceType 对应的适配器。
func (r *RegistryMap) Register(interfaceType string, a Adapter) {
	if r.adapters == nil {
		r.adapters = make(map[string]Adapter)
	}
	r.adapters[interfaceType] = a
}

// Get 获取 interfaceType 对应的适配器，若未注册则返回 nil。
func (r *RegistryMap) Get(interfaceType string) Adapter {
	return r.adapters[interfaceType]
}

// GetOrDefault 获取适配器，若未注册则返回默认（anthropic）适配器。
func (r *RegistryMap) GetOrDefault(interfaceType string) Adapter {
	if a := r.adapters[interfaceType]; a != nil {
		return a
	}
	return r.adapters[r.default_]
}
