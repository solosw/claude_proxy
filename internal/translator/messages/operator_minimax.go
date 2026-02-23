package messages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// MinimaxStrategy Minimax 运营商转发：Minimax 本身支持 Claude Code，直接 HTTP 转发请求并替换请求头中的 API Key 即可。
type MinimaxStrategy struct{}

func init() {
	OperatorRegistry.Register("minimax", &MinimaxStrategy{})
	OperatorRegistry.Register("glm", &MinimaxStrategy{})
	OperatorRegistry.Register("kimi", &MinimaxStrategy{})
	OperatorRegistry.Register("proxy", &MinimaxStrategy{})
}

// Execute 直接 POST 到模型 BaseURL/v1/messages，替换请求头中的 API Key，并将请求体中的 model 改为上游模型 ID。
func (s *MinimaxStrategy) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("operator minimax: http forward, replace api key and model")

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		logStep("operator minimax: err=missing base_url")
		return 0, "", nil, nil, errMissingBaseURL
	}
	url := baseURL + "/v1/messages"

	// 代理时把请求里的 model 改成上游模型 ID，不修改原 payload
	bodyPayload := make(map[string]any, len(payload))
	for k, v := range payload {
		bodyPayload[k] = v
	}
	if opts.UpstreamModel != "" {
		bodyPayload["model"] = opts.UpstreamModel
	}

	reqBody, err := json.Marshal(bodyPayload)
	if err != nil {
		logStep("operator minimax: err=json marshal %v", err)
		return 0, "", nil, nil, err
	}
	// 调试输出：发送给上游的请求体
	logStep("operator minimax: payload_to_send=%s", string(reqBody))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, "", nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}
	if opts.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logStep("operator minimax: err=do request %v", err)
		return 0, "", nil, nil, err
	}
	statusCode = resp.StatusCode
	contentType = resp.Header.Get("Content-Type")

	if opts.Stream && resp.StatusCode == http.StatusOK {
		logStep("operator minimax: stream response")
		return statusCode, contentType, nil, resp.Body, nil
	}

	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return statusCode, contentType, nil, nil, err
	}
	return statusCode, contentType, body, nil, nil
}

var errMissingBaseURL = errors.New("minimax: model base_url required")
