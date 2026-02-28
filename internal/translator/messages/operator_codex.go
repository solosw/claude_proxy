package messages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

const codexDefaultBaseURL = "https://chatgpt.com/backend-api"

type CodexStrategy struct{}

func init() {
	OperatorRegistry.Register("codex", &CodexStrategy{})
}

func (s *CodexStrategy) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("operator codex: start stream=%v baseURL=%s model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

	originalReq, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: marshal original payload failed: %w", err)
	}

	translatedReq := sdktranslator.TranslateRequestByFormatName(
		sdktranslator.FormatClaude,
		sdktranslator.FormatCodex,
		opts.UpstreamModel,
		originalReq,
		opts.Stream,
	)
	translatedReq, err = normalizeCodexRequestPayload(translatedReq, opts.UpstreamModel, opts.Stream)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: normalize translated payload failed: %w", err)
	}

	upstreamURL := buildCodexResponsesURL(opts.BaseURL)
	logStep("operator codex: dispatch url=%s", upstreamURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(translatedReq))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.Stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	if strings.TrimSpace(opts.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.APIKey))
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: upstream request failed: %w", err)
	}

	statusCode = resp.StatusCode
	contentType = resp.Header.Get("Content-Type")
	logStep("operator codex: upstream status=%d contentType=%s", statusCode, contentType)

	if statusCode < 200 || statusCode >= 300 {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
		return statusCode, contentType, body, nil, fmt.Errorf("operator codex: upstream error status=%d", statusCode)
	}

	if opts.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			if errConv := translateOpenAIChatStream(
				ctx,
				sdktranslator.FormatCodex,
				sdktranslator.FormatClaude,
				resp.Body,
				pw,
				opts.UpstreamModel,
				originalReq,
				translatedReq,
			); errConv != nil {
				logStep("operator codex: stream translate error=%v", errConv)
			}
		}()
		return statusCode, "text/event-stream", nil, pr, nil
	}

	defer resp.Body.Close()
	upstreamBody, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: read upstream body failed: %w", err)
	}

	translateBody := upstreamBody
	lowerContentType := strings.ToLower(contentType)
	if strings.Contains(lowerContentType, "text/event-stream") || looksLikeSSEPayload(upstreamBody) {
		completedJSON, err := ReadOpenAIResponsesCompletedEvent(bytes.NewReader(upstreamBody))
		if err != nil {
			var eventErr *ResponsesCompletedEventError
			if errors.As(err, &eventErr) {
				return http.StatusBadRequest, "application/json", eventErr.Body, nil, fmt.Errorf("operator codex: upstream event error: %s", eventErr.EventType)
			}
			trimmed := bytes.TrimSpace(upstreamBody)
			if json.Valid(trimmed) {
				translateBody = append([]byte(nil), trimmed...)
			} else {
				return 0, "", nil, nil, fmt.Errorf("operator codex: read completed event failed: %w", err)
			}
		} else {
			translateBody = completedJSON
		}
	}

	var param any
	out := sdktranslator.TranslateNonStreamByFormatName(
		ctx,
		sdktranslator.FormatCodex,
		sdktranslator.FormatClaude,
		opts.UpstreamModel,
		originalReq,
		translatedReq,
		translateBody,
		&param,
	)
	if strings.TrimSpace(out) == "" {
		return 0, "", nil, nil, fmt.Errorf("operator codex: non-stream translation returned empty output")
	}

	return statusCode, "application/json", []byte(out), nil, nil
}

func normalizeCodexRequestPayload(raw []byte, upstreamModel string, stream bool) ([]byte, error) {
	payload := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	}
	if payload == nil {
		payload = map[string]any{}
	}

	if strings.TrimSpace(upstreamModel) != "" {
		payload["model"] = strings.TrimSpace(upstreamModel)
	}
	payload["stream"] = stream
	if _, ok := payload["instructions"]; !ok {
		payload["instructions"] = ""
	}

	delete(payload, "previous_response_id")
	delete(payload, "prompt_cache_retention")
	delete(payload, "safety_identifier")

	return json.Marshal(payload)
}

func buildCodexResponsesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = codexDefaultBaseURL
	}

	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "/responses"):
		return base
	case strings.HasSuffix(lower, "/v1"):
		return base + "/responses"
	case strings.HasSuffix(lower, "/backend-api/v1"):
		return base + "/responses"
	default:
		return base + "/v1/responses"
	}
}

func looksLikeSSEPayload(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) || bytes.HasPrefix(trimmed, []byte("event:")) {
		return true
	}
	return bytes.Contains(trimmed, []byte("\ndata:")) || bytes.Contains(trimmed, []byte("\nevent:"))
}
