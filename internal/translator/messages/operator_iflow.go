package messages

import (
	"context"
	"io"
	"strings"
)

// iFlow 官方 API：https://apis.iflow.cn/v1/chat/completions
// 认证：Authorization: Bearer <api key>
// 参考：https://platform.iflow.cn/docs/api-reference、https://github.com/nofendian17/iflow-adapter

const iflowDefaultBaseURL = "https://apis.iflow.cn"

// IFlowStrategy iFlow（心流）运营商：使用 OpenAI 兼容的 /v1/chat/completions，做 Anthropic↔OpenAI 协议转换后请求 iFlow。
type IFlowStrategy struct{}

func init() {
	OperatorRegistry.Register("iflow", &IFlowStrategy{})
}

// Execute 默认请求 apis.iflow.cn，使用模型配置的 BaseURL/APIKey 缺省时用默认地址；通过 openai_compatible 适配器做协议转换。
func (s *IFlowStrategy) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("operator iflow: using openai_compatible adapter for chat/completions")
	opts2 := opts
	if strings.TrimSpace(opts2.BaseURL) == "" {
		opts2.BaseURL = iflowDefaultBaseURL
	}
	return Registry.GetOrDefault("openai_compatible").Execute(ctx, payload, opts2)
}
