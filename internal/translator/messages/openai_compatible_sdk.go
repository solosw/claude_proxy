package messages

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

// OpenAICompatibleSDKAdapter uses CLIProxyAPI SDK translator for Claude<->OpenAI-compatible conversion.
type OpenAICompatibleSDKAdapter struct{}

func (a *OpenAICompatibleSDKAdapter) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("openai_compatible sdk adapter: start stream=%v baseURL=%s model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

	originalReq, err := json.Marshal(payload)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("openai_compatible sdk adapter: marshal original payload failed: %w", err)
	}

	translatedReq := sdktranslator.TranslateRequestByFormatName(
		sdktranslator.FormatClaude,
		sdktranslator.FormatOpenAI,
		opts.UpstreamModel,
		originalReq,
		opts.Stream,
	)
	translatedReq, err = normalizeOpenAICompatibleRequestPayload(translatedReq, opts)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("openai_compatible sdk adapter: normalize translated payload failed: %w", err)
	}

	upstreamURL := buildOpenAICompatibleChatCompletionsURL(opts.BaseURL)
	logStep("openai_compatible sdk adapter: dispatch url=%s", upstreamURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(translatedReq))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("openai_compatible sdk adapter: create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(opts.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.APIKey))
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("openai_compatible sdk adapter: upstream request failed: %w", err)
	}

	statusCode = resp.StatusCode
	contentType = resp.Header.Get("Content-Type")
	logStep("openai_compatible sdk adapter: upstream status=%d contentType=%s", statusCode, contentType)

	if statusCode < 200 || statusCode >= 300 {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
		return statusCode, contentType, body, nil, fmt.Errorf("openai_compatible sdk adapter: upstream error status=%d", statusCode)
	}

	if opts.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			if errConv := translateOpenAICompatibleStreamToClaude(ctx, resp.Body, pw, opts.UpstreamModel, originalReq, translatedReq); errConv != nil {
				logStep("openai_compatible sdk adapter: stream translate error=%v", errConv)
			}
		}()
		return statusCode, "text/event-stream", nil, pr, nil
	}

	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("openai_compatible sdk adapter: read response body failed: %w", err)
	}

	var param any
	out := sdktranslator.TranslateNonStreamByFormatName(
		ctx,
		sdktranslator.FormatOpenAI,
		sdktranslator.FormatClaude,
		opts.UpstreamModel,
		originalReq,
		translatedReq,
		body,
		&param,
	)
	if strings.TrimSpace(out) == "" {
		return 0, "", nil, nil, fmt.Errorf("openai_compatible sdk adapter: non-stream translation returned empty output")
	}

	return statusCode, "application/json", []byte(out), nil, nil
}

func normalizeOpenAICompatibleRequestPayload(raw []byte, opts ExecuteOptions) ([]byte, error) {
	payload := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	}
	if payload == nil {
		payload = map[string]any{}
	}

	if strings.TrimSpace(opts.UpstreamModel) != "" {
		payload["model"] = strings.TrimSpace(opts.UpstreamModel)
	}
	payload["stream"] = opts.Stream

	if opts.MinimalOpenAI {
		delete(payload, "tools")
		delete(payload, "tool_choice")
		delete(payload, "functions")
		delete(payload, "function_call")
	}

	return json.Marshal(payload)
}

func buildOpenAICompatibleChatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.openai.com"
	}

	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, "/chat/completions"):
		return base
	case strings.HasSuffix(lower, "/v1"):
		return base + "/chat/completions"
	default:
		return base + "/v1/chat/completions"
	}
}

func translateOpenAICompatibleStreamToClaude(ctx context.Context, reader io.Reader, writer io.Writer, model string, originalReq, translatedReq []byte) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var param any
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}

		chunks := sdktranslator.TranslateStreamByFormatName(
			ctx,
			sdktranslator.FormatOpenAI,
			sdktranslator.FormatClaude,
			model,
			originalReq,
			translatedReq,
			bytes.Clone(line),
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
