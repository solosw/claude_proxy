package messages

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// NewAPIAdapter 按 NewAPI 文档格式组包并发送，与通用 openai 适配器分离；携带 tools/tool_choice。
// 文档：https://docs.newapi.pro/zh/docs/api/ai-model/chat/openai/createchatcompletion
type NewAPIAdapter struct{}

func init() {
	Registry.Register("newapi", &NewAPIAdapter{})
}

// Execute 按 NewAPI 请求体格式（model, messages, max_tokens, stream, temperature, top_p, tools, tool_choice）组包，POST /v1/chat/completions。
func (a *NewAPIAdapter) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("newapi adapter: start, stream=%v, baseURL=%s, model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

	messages := anthropicToOpenAIMessages(payload)
	if len(messages) == 0 {
		logStep("newapi adapter: no messages, err=empty")
		return 0, "", nil, nil, errEmptyMessages
	}

	maxTokens := 4096
	if v, ok := payload["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}

	req := openAIReq{
		Model:     opts.UpstreamModel,
		Messages:  messages,
		MaxTokens: maxTokens,
		Stream:    opts.Stream,
	}
	if v, ok := payload["temperature"].(float64); ok {
		f := float32(v)
		req.Temperature = &f
	}
	if v, ok := payload["top_p"].(float64); ok {
		f := float32(v)
		req.TopP = &f
	}
	if toolsIn, ok := payload["tools"].([]any); ok && len(toolsIn) > 0 {
		req.Tools = anthropicToolsToOpenAI(toolsIn)
		req.ToolChoice = "auto"
		if tc, ok := payload["tool_choice"].(map[string]any); ok {
			if v, _ := tc["type"].(string); v == "none" {
				req.ToolChoice = "none"
			} else if name, _ := tc["name"].(string); v == "tool" && name != "" {
				req.ToolChoice = map[string]any{"type": "function", "function": map[string]any{"name": name}}
			}
		}
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		logStep("newapi adapter: marshal err=%v", err)
		return 0, "", nil, nil, err
	}

	// 调试输出：发送给上游的请求体
	logStep("newapi adapter: payload_to_send=%s", string(reqBody))

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.newapi.pro"
	}
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}
	url := baseURL + "/chat/completions"
	logStep("newapi adapter: POST %s bodyLen=%d", url, len(reqBody))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, "", nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}
	if opts.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		logStep("newapi adapter: do err=%v", err)
		return 0, "", nil, nil, err
	}
	statusCode = resp.StatusCode
	contentType = resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	if statusCode < 200 || statusCode >= 300 {
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		logStep("newapi adapter: status=%d bodyLen=%d", statusCode, len(body))
		return statusCode, contentType, body, nil, nil
	}

	if opts.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			ConvertOpenAIStreamReaderToAnthropic(ctx, resp.Body, pw)
		}()
		return http.StatusOK, "text/event-stream", nil, pr, nil
	}

	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return statusCode, contentType, nil, nil, err
	}
	var oaiResp openai.ChatCompletionResponse
	if json.Unmarshal(body, &oaiResp) != nil {
		logStep("newapi adapter: unmarshal response err")
		return statusCode, contentType, body, nil, nil
	}
	out, err := openAIRespToAnthropic(oaiResp)
	if err != nil {
		logStep("newapi adapter: to anthropic err=%v", err)
		return 0, "", nil, nil, err
	}
	body, _ = json.Marshal(out)
	logStep("newapi adapter: success status=200 bodyLen=%d", len(body))
	return http.StatusOK, "application/json", body, nil, nil
}
