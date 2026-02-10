package messages

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicAdapter 使用官方 anthropic-sdk-go 请求上游，不手写 HTTP。
type AnthropicAdapter struct{}

func (a *AnthropicAdapter) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("anthropic adapter: start, stream=%v, baseURL=%s, model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

	// 拷贝 payload 并替换 model 为上游模型
	reqBody := make(map[string]any)
	for k, v := range payload {
		reqBody[k] = v
	}
	reqBody["model"] = opts.UpstreamModel

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	logStep("anthropic adapter: creating client baseURL=%s", baseURL)

	clientOpts := []option.RequestOption{
		option.WithBaseURL(baseURL),
		option.WithRequestTimeout(600 * time.Second),
	}
	if opts.APIKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.APIKey))
	}
	client := anthropic.NewClient(clientOpts...)

	if opts.Stream {
		var rawResp *http.Response
		err = client.Post(ctx, "/v1/messages", reqBody, nil, option.WithResponseInto(&rawResp))
		logStep("anthropic adapter: stream POST done, err=%v rawResp=%v", err, rawResp != nil)
		if err != nil && rawResp != nil {
			// 有错误且拿到了响应（如 422）：读出 status/body 供上层按 Claude 格式返回
			statusCode = rawResp.StatusCode
			contentType = rawResp.Header.Get("Content-Type")
			if rawResp.Body != nil {
				body, _ = io.ReadAll(rawResp.Body)
				rawResp.Body.Close()
			}
			logStep("anthropic adapter: stream error response status=%d bodyLen=%d", statusCode, len(body))
			return statusCode, contentType, body, nil, err
		}
		if err != nil {
			if apiErr, ok := err.(*anthropic.Error); ok && apiErr.Request != nil && apiErr.Request.Response != nil {
				r := apiErr.Request.Response
				statusCode = r.StatusCode
				contentType = r.Header.Get("Content-Type")
				if r.Body != nil {
					body, _ = io.ReadAll(r.Body)
					r.Body.Close()
				}
				return statusCode, contentType, body, nil, err
			}
			return 0, "", nil, nil, err
		}
		if rawResp == nil {
			logStep("anthropic adapter: stream response is nil")
			return 0, "", nil, nil, err
		}
		statusCode = rawResp.StatusCode
		contentType = rawResp.Header.Get("Content-Type")
		if statusCode < 200 || statusCode >= 300 {
			body, _ = io.ReadAll(rawResp.Body)
			rawResp.Body.Close()
			logStep("anthropic adapter: stream upstream status=%d bodyLen=%d", statusCode, len(body))
			return statusCode, contentType, body, nil, nil
		}
		logStep("anthropic adapter: stream upstream status=%d, returning body stream", statusCode)
		return statusCode, contentType, nil, rawResp.Body, nil
	}

	// 非流式：用 WithResponseInto 拿原始响应再读 body
	var httpResp *http.Response
	err = client.Post(ctx, "/v1/messages", reqBody, nil, option.WithResponseInto(&httpResp))
	logStep("anthropic adapter: non-stream POST done, err=%v httpResp=%v", err, httpResp != nil)
	if err != nil && httpResp != nil {
		statusCode = httpResp.StatusCode
		contentType = httpResp.Header.Get("Content-Type")
		if httpResp.Body != nil {
			body, _ = io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
		}
		logStep("anthropic adapter: non-stream error response status=%d bodyLen=%d", statusCode, len(body))
		return statusCode, contentType, body, nil, err
	}
	if err != nil {
		if apiErr, ok := err.(*anthropic.Error); ok {
			logStep("anthropic adapter: api error status=%d", apiErr.StatusCode)
			statusCode = apiErr.StatusCode
			var respBody []byte
			if apiErr.Request != nil && apiErr.Request.Response != nil && apiErr.Request.Response.Body != nil {
				respBody, _ = io.ReadAll(apiErr.Request.Response.Body)
			}
			ct := ""
			if apiErr.Request != nil && apiErr.Request.Response != nil {
				ct = apiErr.Request.Response.Header.Get("Content-Type")
			}
			return statusCode, ct, respBody, nil, err
		}
		return 0, "", nil, nil, err
	}
	defer httpResp.Body.Close()
	statusCode = httpResp.StatusCode
	contentType = httpResp.Header.Get("Content-Type")
	respBody, _ := io.ReadAll(httpResp.Body)
	logStep("anthropic adapter: non-stream status=%d contentType=%s bodyLen=%d", statusCode, contentType, len(respBody))
	return statusCode, contentType, respBody, nil, nil
}
