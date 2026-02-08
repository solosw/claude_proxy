package messages

import (
	"context"
	"errors"
	"io"
	"strings"
)

// NewAPI 使用 OpenAI Chat Completions 格式，文档：https://docs.newapi.pro/zh/docs/api/ai-model/chat/openai/createchatcompletion
const newAPIDefaultBaseURL = "https://api.newapi.pro"

// NewAPIStrategy NewAPI 运营商：使用专用 newapi 适配器，按文档格式组包并携带 tools/tool_choice。
type NewAPIStrategy struct{}

func init() {
	OperatorRegistry.Register("newapi", &NewAPIStrategy{})
}

// Execute 使用 newapi 适配器（与 openai 区分），按 NewAPI 文档格式发 /v1/chat/completions，含 tools。
func (s *NewAPIStrategy) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("operator newapi: using newapi adapter (doc format with tools)")
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = newAPIDefaultBaseURL
	}
	adapter := Registry.Get("newapi")
	if adapter == nil {
		return 0, "", nil, nil, errors.New("newapi adapter not registered")
	}
	return adapter.Execute(ctx, payload, opts)
}
