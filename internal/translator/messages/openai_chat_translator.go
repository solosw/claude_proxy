package messages

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

// OpenAIChatTranslateOptions 定义 OpenAI Chat 转换上下文。
type OpenAIChatTranslateOptions struct {
	UpstreamModel string
	Stream        bool
}

type ResponsesCompletedEventError struct {
	EventType string
	Body      []byte
}

func (e *ResponsesCompletedEventError) Error() string {
	return fmt.Sprintf("upstream error event type=%s", e.EventType)
}

func ConvertOpenAIChatToAnthropicRequest(originalReq []byte, opts OpenAIChatTranslateOptions) ([]byte, error) {
	translatedReq := sdktranslator.TranslateRequestByFormatName(
		sdktranslator.FormatOpenAI,
		sdktranslator.FormatClaude,
		opts.UpstreamModel,
		originalReq,
		opts.Stream,
	)
	return normalizeClaudeRequestPayload(translatedReq, opts)
}

func ConvertAnthropicToOpenAIChatResponse(ctx context.Context, upstreamModel string, originalReq, translatedReq, upstreamResp []byte) ([]byte, error) {
	var param any
	out := sdktranslator.TranslateNonStreamByFormatName(
		ctx,
		sdktranslator.FormatClaude,
		sdktranslator.FormatOpenAI,
		upstreamModel,
		originalReq,
		translatedReq,
		upstreamResp,
		&param,
	)
	if strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("translate anthropic response to openai chat returned empty output")
	}
	if !json.Valid([]byte(out)) {
		return nil, fmt.Errorf("translate anthropic response to openai chat returned invalid json")
	}
	return []byte(out), nil
}

func TranslateAnthropicStreamToOpenAIChat(ctx context.Context, reader io.Reader, writer io.Writer, upstreamModel string, originalReq, translatedReq []byte) error {
	return translateOpenAIChatStream(ctx, sdktranslator.FormatClaude, sdktranslator.FormatOpenAI, reader, writer, upstreamModel, originalReq, translatedReq)
}

func ConvertOpenAIChatToOpenAIResponsesRequest(originalReq []byte, opts OpenAIChatTranslateOptions) ([]byte, error) {
	translatedReq := sdktranslator.TranslateRequestByFormatName(
		sdktranslator.FormatOpenAI,
		sdktranslator.FormatCodex,
		opts.UpstreamModel,
		originalReq,
		opts.Stream,
	)
	return normalizeOpenAIResponsesRequestPayload(translatedReq, opts)
}

func ConvertOpenAIResponsesToOpenAIChatResponse(ctx context.Context, upstreamModel string, originalReq, translatedReq, completedResp []byte) ([]byte, error) {
	payload := unwrapOpenAIResponsesEventPayload(completedResp)

	var param any
	out := sdktranslator.TranslateNonStreamByFormatName(
		ctx,
		sdktranslator.FormatCodex,
		sdktranslator.FormatOpenAI,
		upstreamModel,
		originalReq,
		translatedReq,
		payload,
		&param,
	)
	out = strings.TrimSpace(out)
	if (out == "" || !json.Valid([]byte(out))) && !bytes.Equal(bytes.TrimSpace(payload), bytes.TrimSpace(completedResp)) {
		out = sdktranslator.TranslateNonStreamByFormatName(
			ctx,
			sdktranslator.FormatCodex,
			sdktranslator.FormatOpenAI,
			upstreamModel,
			originalReq,
			translatedReq,
			completedResp,
			&param,
		)
		out = strings.TrimSpace(out)
	}
	if out == "" {
		logStep("openai_chat_translator: responses->openai empty output, upstream response=%s", string(completedResp))
		return nil, fmt.Errorf("translate openai responses to openai chat returned empty output")
	}
	if !json.Valid([]byte(out)) {
		logStep("openai_chat_translator: responses->openai invalid json, translated output=%s", out)
		return nil, fmt.Errorf("translate openai responses to openai chat returned invalid json")
	}
	return []byte(out), nil
}

func TranslateOpenAIResponsesStreamToOpenAIChat(ctx context.Context, reader io.Reader, writer io.Writer, upstreamModel string, originalReq, translatedReq []byte) error {
	return translateOpenAIChatStream(ctx, sdktranslator.FormatCodex, sdktranslator.FormatOpenAI, reader, writer, upstreamModel, originalReq, translatedReq)
}

func ReadOpenAIResponsesCompletedEvent(reader io.Reader) ([]byte, error) {
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
		switch event.Type {
		case "response.completed", "response.done", "response_stop":
			return append([]byte(nil), line...), nil
		case "response.failed", "error":
			return nil, &ResponsesCompletedEventError{EventType: event.Type, Body: append([]byte(nil), line...)}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("response.completed not found")
}

func unwrapOpenAIResponsesEventPayload(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return raw
	}

	var event map[string]any
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return raw
	}

	resp, hasResp := event["response"]
	if !hasResp {
		return raw
	}
	respMap, ok := resp.(map[string]any)
	if !ok || len(respMap) == 0 {
		return raw
	}
	payload, err := json.Marshal(respMap)
	if err != nil || !json.Valid(payload) {
		return raw
	}
	return payload
}

func translateOpenAIChatStream(ctx context.Context, from, to sdktranslator.Format, reader io.Reader, writer io.Writer, upstreamModel string, originalReq, translatedReq []byte) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var param any
	lineCount := 0
	chunkCount := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			logStep("openai_chat_translator: stream context cancelled after %d lines", lineCount)
			return ctx.Err()
		default:
		}

		line := bytes.TrimSpace(scanner.Bytes())
		lineCount++

		// 打印上游原始 SSE 行（前几条用于调试）
		if lineCount <= 3 {
			linePreview := string(line)
			if len(linePreview) > 300 {
				linePreview = linePreview[:300] + "...(truncated)"
			}
			logStep("openai_chat_translator: upstream line[%d]: %s", lineCount, linePreview)
		}

		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}

		chunks := sdktranslator.TranslateStreamByFormatName(
			ctx,
			from,
			to,
			upstreamModel,
			originalReq,
			translatedReq,
			bytes.Clone(line),
			&param,
		)
		for _, chunk := range chunks {
			chunkCount++
			// 打印前几条转换后的 chunk
			if from == sdktranslator.FormatCodex || chunkCount <= 5 {
				chunkPreview := string(chunk)
				if len(chunkPreview) > 500 {
					chunkPreview = chunkPreview[:500] + "...(truncated)"
				}
				logStep("openai_chat_translator: translated chunk[%d]: %s", chunkCount, chunkPreview)
			}
			if strings.TrimSpace(chunk) == "" {
				if _, err := io.WriteString(writer, "\n"); err != nil {
					return err
				}
				continue
			}

			// SDK translator 返回的是裸 JSON，需要包装成 SSE 格式
			trimmedChunk := strings.TrimSpace(chunk)
			isSSEChunk := strings.HasPrefix(trimmedChunk, "data:") ||
				strings.HasPrefix(trimmedChunk, "event:") ||
				strings.HasPrefix(trimmedChunk, "id:") ||
				strings.HasPrefix(trimmedChunk, "retry:")
			if isSSEChunk {
				if !strings.HasSuffix(chunk, "\n") {
					chunk = chunk + "\n"
				}
				if _, err := io.WriteString(writer, chunk); err != nil {
					logStep("openai_chat_translator: stream write error after %d lines: %v", lineCount, err)
					return err
				}
				continue
			}

			if _, err := io.WriteString(writer, "data: "+trimmedChunk+"\n\n"); err != nil {
				logStep("openai_chat_translator: stream write error after %d lines: %v", lineCount, err)
				return err
			}

		}
	}

	if err := scanner.Err(); err != nil {
		logStep("openai_chat_translator: stream scanner error after %d lines: %v", lineCount, err)
		return err
	}

	logStep("openai_chat_translator: stream completed successfully, total upstream lines: %d, translated chunks: %d", lineCount, chunkCount)
	return nil
}

func normalizeClaudeRequestPayload(raw []byte, opts OpenAIChatTranslateOptions) ([]byte, error) {
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
	return json.Marshal(payload)
}

func normalizeOpenAIResponsesRequestPayload(raw []byte, opts OpenAIChatTranslateOptions) ([]byte, error) {
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
	if _, ok := payload["instructions"]; !ok {
		payload["instructions"] = ""
	}
	delete(payload, "previous_response_id")
	delete(payload, "prompt_cache_retention")
	delete(payload, "safety_identifier")
	payload["tools"] = ensureToolSearchForDeferredTools(payload["tools"])
	if tools, ok := payload["tools"].([]any); ok && len(tools) == 0 {
		delete(payload, "tools")
		delete(payload, "tool_choice")
		delete(payload, "parallel_tool_calls")
		delete(payload, "tool_constraint")
	}
	return json.Marshal(payload)
}

func ensureToolSearchForDeferredTools(raw any) []any {
	tools, ok := raw.([]any)
	if !ok || len(tools) == 0 {
		return nil
	}

	hasDeferred := false
	hasToolSearch := false
	out := make([]any, 0, len(tools)+1)
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			logStep("openai_chat_translator: drop unsupported tool item type=%T", item)
			continue
		}
		cleaned := make(map[string]any, len(tool))
		for k, v := range tool {
			key := strings.TrimSpace(strings.ToLower(k))
			if key == "defer_loading" {
				if b, ok := v.(bool); ok && b {
					hasDeferred = true
				}
			}
			cleaned[k] = v
		}
		if typ, _ := cleaned["type"].(string); strings.EqualFold(strings.TrimSpace(typ), "tool_search") {
			hasToolSearch = true
		}
		out = append(out, cleaned)
	}

	if hasDeferred && !hasToolSearch {
		logStep("openai_chat_translator: detected defer_loading tools without tool_search, auto-injecting tool_search")
		out = append(out, map[string]any{"type": "tool_search"})
	}
	return out
}
