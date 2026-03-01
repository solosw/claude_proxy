package oldhandler

import (
	"awesomeProject/internal/translator/messages"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"io"
	"net/http"
	"strings"
	"time"
)

const codexDefaultBaseURL = "https://chatgpt.com/backend-api"

type CodexStrategy struct{}

func Start() {
	//messages.OperatorRegistry.Register("codex", &CodexStrategy{})
	//messages.Registry.Register("openai_responses", &CodexStrategy{})
}

func (s *CodexStrategy) Execute(ctx context.Context, payload map[string]any, opts messages.ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	messages.LogStep("operator codex: start stream=%v baseURL=%s model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

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
	translatedReq, err = normalizeCodexRequestPayload(translatedReq, opts.UpstreamModel)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: normalize translated payload failed: %w", err)
	}

	upstreamURL := buildCodexResponsesURL(opts.BaseURL)
	messages.LogStep("operator codex: dispatch url=%s", upstreamURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(translatedReq))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
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
	messages.LogStep("operator codex: upstream status=%d contentType=%s", statusCode, contentType)

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
			if errConv := translateCodexStreamToClaude(ctx, resp.Body, pw, opts.UpstreamModel, originalReq, translatedReq); errConv != nil {
				messages.LogStep("operator codex: stream translate error=%v", errConv)
			}
		}()
		return statusCode, "text/event-stream", nil, pr, nil
	}

	defer resp.Body.Close()
	completedJSON, err := readCodexCompletedEvent(resp.Body)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("operator codex: read completed event failed: %w", err)
	}

	var param any
	out := sdktranslator.TranslateNonStreamByFormatName(
		ctx,
		sdktranslator.FormatCodex,
		sdktranslator.FormatClaude,
		opts.UpstreamModel,
		originalReq,
		translatedReq,
		completedJSON,
		&param,
	)
	if strings.TrimSpace(out) == "" {
		return 0, "", nil, nil, fmt.Errorf("operator codex: non-stream translation returned empty output")
	}

	return statusCode, "application/json", []byte(out), nil, nil
}

func normalizeCodexRequestPayload(raw []byte, upstreamModel string) ([]byte, error) {
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
	payload["stream"] = true
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

func translateCodexStreamToClaude(ctx context.Context, reader io.Reader, writer io.Writer, model string, originalReq, translatedReq []byte) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var param any
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		chunks := sdktranslator.TranslateStreamByFormatName(
			ctx,
			sdktranslator.FormatCodex,
			sdktranslator.FormatClaude,
			model,
			originalReq,
			translatedReq,
			[]byte(line),
			&param,
		)
		for _, chunk := range chunks {
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			if _, err := io.WriteString(writer, chunk); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func readCodexCompletedEvent(reader io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[5:])
		}
		if bytes.Equal(line, []byte("[DONE]")) || !json.Valid(line) {
			continue
		}

		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Type == "response.completed" {
			return append([]byte(nil), line...), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("response.completed not found")
}
