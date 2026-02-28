package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TranslateResponsesRequestForAdapter(payload map[string]any, upstreamModel string, streamRequested bool, adapterMode string) (originalRaw, translatedRaw []byte, err error) {
	originalRaw, err = json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal responses payload: %w", err)
	}

	toFormat, err := responsesAdapterTargetFormat(adapterMode)
	if err != nil {
		return nil, nil, err
	}

	translatedRaw = sdktranslator.TranslateRequestByFormatName(
		sdktranslator.FormatOpenAIResponse,
		toFormat,
		upstreamModel,
		originalRaw,
		streamRequested,
	)

	translatedRaw, err = normalizeResponsesBridgeTranslatedRequestPayload(translatedRaw, upstreamModel, streamRequested, toFormat)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize translated payload: %w", err)
	}

	return originalRaw, translatedRaw, nil
}

func TranslateResponsesNonStreamForClient(
	ctx context.Context,
	adapterMode string,
	upstreamModel string,
	originalRequestRawJSON []byte,
	translatedRequestRawJSON []byte,
	upstreamResponseRawJSON []byte,
) ([]byte, error) {
	fromFormat, err := responsesAdapterTargetFormat(adapterMode)
	if err != nil {
		return nil, err
	}

	var state any
	out := sdktranslator.TranslateNonStreamByFormatName(
		ctx,
		fromFormat,
		sdktranslator.FormatOpenAIResponse,
		upstreamModel,
		originalRequestRawJSON,
		translatedRequestRawJSON,
		upstreamResponseRawJSON,
		&state,
	)
	if strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("translated responses response is empty")
	}
	return []byte(out), nil
}

func TranslateResponsesStreamChunkForClient(
	ctx context.Context,
	adapterMode string,
	upstreamModel string,
	originalRequestRawJSON []byte,
	translatedRequestRawJSON []byte,
	upstreamChunk []byte,
	state *any,
) ([]string, error) {
	fromFormat, err := responsesAdapterTargetFormat(adapterMode)
	if err != nil {
		return nil, err
	}

	chunks := sdktranslator.TranslateStreamByFormatName(
		ctx,
		fromFormat,
		sdktranslator.FormatOpenAIResponse,
		upstreamModel,
		originalRequestRawJSON,
		translatedRequestRawJSON,
		upstreamChunk,
		state,
	)
	return chunks, nil
}

func responsesAdapterTargetFormat(adapterMode string) (sdktranslator.Format, error) {
	switch adapterMode {
	case "adapt_anthropic_sdk":
		return sdktranslator.FormatClaude, nil
	case "adapt_openai_compatible_sdk":
		return sdktranslator.FormatOpenAI, nil
	default:
		return "", fmt.Errorf("unsupported adapter mode: %s", adapterMode)
	}
}

func normalizeResponsesBridgeTranslatedRequestPayload(raw []byte, upstreamModel string, streamRequested bool, toFormat sdktranslator.Format) ([]byte, error) {
	payload := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
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
	payload["stream"] = streamRequested

	switch toFormat {
	case sdktranslator.FormatOpenAI:
		if _, ok := payload["messages"]; !ok {
			if input, hasInput := payload["input"]; hasInput {
				if msgs := responsesInputToOpenAIMessages(input); len(msgs) > 0 {
					payload["messages"] = msgs
				}
			}
		}
		payload["messages"] = prependSystemInstructionToOpenAIMessages(payload["messages"], payload["instructions"])
		if _, ok := payload["messages"]; !ok {
			payload["messages"] = []any{}
		}
	case sdktranslator.FormatClaude:
		if _, ok := payload["messages"]; !ok {
			payload["messages"] = []any{}
		}
		if _, ok := payload["max_tokens"]; !ok {
			if v, has := payload["max_output_tokens"]; has {
				payload["max_tokens"] = v
			} else {
				payload["max_tokens"] = 4096
			}
		}
	}

	return json.Marshal(payload)
}

func prependSystemInstructionToOpenAIMessages(messages any, instructions any) any {
	instructionText := strings.TrimSpace(anyToString(instructions))
	if instructionText == "" {
		return messages
	}

	arr, _ := messages.([]any)
	if len(arr) > 0 {
		if first, ok := arr[0].(map[string]any); ok {
			if strings.EqualFold(strings.TrimSpace(anyToString(first["role"])), "system") {
				return arr
			}
		}
	}

	systemMessage := map[string]any{
		"role":    "system",
		"content": instructionText,
	}
	if len(arr) == 0 {
		return []any{systemMessage}
	}
	return append([]any{systemMessage}, arr...)
}

func responsesInputToOpenAIMessages(input any) []any {
	switch v := input.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []any{map[string]any{"role": "user", "content": strings.TrimSpace(v)}}
	case []any:
		out := make([]any, 0, len(v))
		for _, rawItem := range v {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.ToLower(strings.TrimSpace(anyToString(item["type"])))
			switch itemType {
			case "message", "":
				role := strings.TrimSpace(anyToString(item["role"]))
				if role == "" {
					role = "user"
				}
				content := responsesContentToOpenAIMessageContent(item["content"])
				if content == nil {
					continue
				}
				out = append(out, map[string]any{
					"role":    role,
					"content": content,
				})
			case "function_call":
				callID := strings.TrimSpace(anyToString(item["call_id"]))
				if callID == "" {
					callID = strings.TrimSpace(anyToString(item["id"]))
				}
				name := strings.TrimSpace(anyToString(item["name"]))
				if name == "" {
					continue
				}
				arguments := anyToJSONString(item["arguments"])
				if arguments == "" {
					arguments = "{}"
				}
				toolCall := map[string]any{
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": arguments,
					},
				}
				if callID != "" {
					toolCall["id"] = callID
				}
				out = append(out, map[string]any{
					"role":       "assistant",
					"content":    "",
					"tool_calls": []any{toolCall},
				})
			case "function_call_output":
				callID := strings.TrimSpace(anyToString(item["call_id"]))
				output := anyToJSONString(item["output"])
				if output == "" {
					output = strings.TrimSpace(anyToString(item["output"]))
				}
				out = append(out, map[string]any{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      output,
				})
			}
		}
		return out
	default:
		return nil
	}
}

func responsesContentToOpenAIMessageContent(content any) any {
	switch v := content.(type) {
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		return text
	case []any:
		parts := make([]any, 0, len(v))
		textOnly := true
		for _, rawPart := range v {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			partType := strings.ToLower(strings.TrimSpace(anyToString(part["type"])))
			switch partType {
			case "input_text", "output_text", "text", "":
				text := strings.TrimSpace(anyToString(part["text"]))
				if text == "" {
					continue
				}
				parts = append(parts, map[string]any{
					"type": "text",
					"text": text,
				})
			case "input_image", "image_url":
				textOnly = false
				url := strings.TrimSpace(anyToString(part["image_url"]))
				if url == "" {
					if m, ok := part["image_url"].(map[string]any); ok {
						url = strings.TrimSpace(anyToString(m["url"]))
					}
				}
				if url == "" {
					continue
				}
				parts = append(parts, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": url,
					},
				})
			}
		}
		if len(parts) == 0 {
			return nil
		}
		if textOnly && len(parts) == 1 {
			if one, ok := parts[0].(map[string]any); ok {
				if text := strings.TrimSpace(anyToString(one["text"])); text != "" {
					return text
				}
			}
		}
		return parts
	default:
		return nil
	}
}

func anyToJSONString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func anyToString(v any) string {
	s, _ := v.(string)
	return s
}
