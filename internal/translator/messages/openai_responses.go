package messages

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
)

// userAgentTransport 自定义 Transport，用于在 HTTP 请求中添加 User-Agent
type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	return t.base.RoundTrip(req)
}

// OpenAIResponsesAdapter 协议适配器：将入口的 Anthropic /v1/messages 请求转为 OpenAI Responses API（/v1/responses）请求并转发，
// 使用与 chat_test_handler 相同的 openai-go SDK 构建请求体，保证与官方序列化一致，避免网关兼容性问题。
// 返回的 body/stream 为 OpenAI Responses 格式，由 handler 按 response_format 再转为 Anthropic（如需）。
type OpenAIResponsesAdapter struct{}

func init() {
	Registry.Register("openai_responses", &CodexStrategy{})
}

// anthropicPayloadToResponsesParams 从 Anthropic payload 构建与 chat_test 一致的 ResponseNewParams，用于 Marshal 后发往上游。
func anthropicPayloadToResponsesParams(payload map[string]any, opts ExecuteOptions) (responses.ResponseNewParams, error) {
	inputList := make(responses.ResponseInputParam, 0)
	if sys, ok := payload["system"].(string); ok && strings.TrimSpace(sys) != "" {
		inputList = append(inputList, responses.ResponseInputItemParamOfMessage(strings.TrimSpace(sys), responses.EasyInputMessageRoleSystem))
	}
	msgs, ok := payload["messages"].([]any)
	if !ok {
		return responses.ResponseNewParams{}, errEmptyMessages
	}
	for _, m := range msgs {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		if role == "" {
			role = "user"
		}
		role = strings.ToLower(strings.TrimSpace(role))
		var msgRole responses.EasyInputMessageRole
		switch role {
		case "assistant":
			msgRole = responses.EasyInputMessageRoleAssistant
		case "system":
			msgRole = responses.EasyInputMessageRoleSystem
		case "developer":
			msgRole = responses.EasyInputMessageRoleDeveloper
		default:
			msgRole = responses.EasyInputMessageRoleUser
		}
		text := extractAnthropicMessageText(mm)
		inputList = append(inputList, responses.ResponseInputItemParamOfMessage(text, msgRole))
	}
	if len(inputList) == 0 {
		return responses.ResponseNewParams{}, errEmptyMessages
	}
	inputUnion := responses.ResponseNewParamsInputUnion{}
	inputUnion.OfInputItemList = inputList

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(opts.UpstreamModel),
		Input: inputUnion,
	}
	if v, ok := payload["max_tokens"].(float64); ok && v > 0 {
		params.MaxOutputTokens = openai.Int(int64(v))
	} else {
		params.MaxOutputTokens = openai.Int(1024)
	}
	if v, ok := payload["temperature"].(float64); ok {
		params.Temperature = openai.Float(v)
	}
	// 工具转发：在 SDK 的 params 上设置 Tools、ToolChoice，由 MarshalJSON 序列化进请求体
	if toolsIn, ok := payload["tools"].([]any); ok && len(toolsIn) > 0 {
		params.Tools = anthropicToolsToResponsesTools(toolsIn)
		logStep("openai_responses adapter: tools_count=%d", len(params.Tools))
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
		}
		if tc, ok := payload["tool_choice"].(map[string]any); ok {
			if v, _ := tc["type"].(string); v == "none" {
				params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
					OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsNone),
				}
				logStep("openai_responses adapter: tool_choice=none")
			} else if name, _ := tc["name"].(string); v == "tool" && name != "" {
				params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
					OfFunctionTool: &responses.ToolChoiceFunctionParam{
						Name: name,
						Type: constant.Function("function"),
					},
				}
				logStep("openai_responses adapter: tool_choice=tool name=%s", name)
			} else {
				logStep("openai_responses adapter: tool_choice=auto")
			}
		} else {
			logStep("openai_responses adapter: tool_choice=auto (default)")
		}
	}
	return params, nil
}

// anthropicToolsToResponsesTools 将 Anthropic tools[] 转为 SDK 的 []ToolUnionParam，由 SDK 序列化。
// 与 Codex/Responses API 代理示例一致：strict 默认 false，部分网关对 strict: true 会返回 502。
func anthropicToolsToResponsesTools(tools []any) []responses.ToolUnionParam {
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name == "" {
			continue
		}
		var parameters map[string]any
		if schema := m["input_schema"]; schema != nil {
			parameters, _ = schema.(map[string]any)
		}
		if parameters == nil {
			parameters = make(map[string]any)
		}
		strict := false
		if b, ok := m["strict"].(bool); ok {
			strict = b
		}
		out = append(out, responses.ToolParamOfFunction(name, parameters, strict))
	}
	return out
}

func extractAnthropicMessageText(mm map[string]any) string {
	content := mm["content"]
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var sb strings.Builder
		for _, blk := range v {
			bm, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "" && t != "text" {
				continue
			}
			if txt, ok := bm["text"].(string); ok {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(strings.TrimSpace(txt))
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

func (a *OpenAIResponsesAdapter) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("openai_responses adapter: start, stream=%v, baseURL=%s, model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

	params, err := anthropicPayloadToResponsesParams(payload, opts)
	if err != nil {
		logStep("openai_responses adapter: build params err=%v", err)
		return 0, "", nil, nil, err
	}

	// 调试输出：打印构建的请求参数
	if reqBody, err := params.MarshalJSON(); err == nil {
		logStep("openai_responses adapter: payload_converted=%s", string(reqBody))
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	// 使用 OpenAI SDK 客户端,与 chat_test_handler 完全一致
	httpClient := &http.Client{Timeout: 30 * time.Minute}
	// 如果提供了 User-Agent，使用自定义 Transport 添加
	if strings.TrimSpace(opts.UserAgent) != "" {
		httpClient.Transport = &userAgentTransport{
			base:      httpClient.Transport,
			userAgent: opts.UserAgent,
		}
	}
	client := openai.NewClient(
		option.WithAPIKey(opts.APIKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(httpClient),
	)

	logStep("openai_responses adapter: calling SDK with stream=%v", opts.Stream)

	if opts.Stream {
		// 流式调用
		return a.executeStream(ctx, &client, params)
	}

	// 非流式调用
	resp, err := client.Responses.New(ctx, params)
	if err != nil {
		logStep("openai_responses adapter: SDK error=%v", err)
		return a.handleSDKError(err)
	}

	// 空指针防护：确保响应对象不为 nil
	if resp == nil {
		logStep("openai_responses adapter: received nil response from SDK")
		return 502, "application/json", []byte(`{"error":"upstream returned nil response"}`), nil, nil
	}

	// 检查响应中的工具调用
	if resp.Output != nil && len(resp.Output) > 0 {
		logStep("openai_responses adapter: response output_count=%d", len(resp.Output))
		for i, output := range resp.Output {
			logStep("openai_responses adapter: output[%d] type=%s", i, output.Type)
			// 检查是否有工具调用相关字段
			if output.Name != "" {
				logStep("openai_responses adapter: output[%d] has name=%s", i, output.Name)
			}
			if output.CallID != "" {
				logStep("openai_responses adapter: output[%d] has call_id=%s", i, output.CallID)
			}
			if output.Arguments != "" {
				logStep("openai_responses adapter: output[%d] has arguments (len=%d)", i, len(output.Arguments))
			}
		}
	} else {
		logStep("openai_responses adapter: response output is empty")
	}

	// 构建响应 JSON（安全地处理可能为 nil 的字段）
	respJSON := map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  resp.Model,
		"status": string(resp.Status),
	}

	// 安全地添加 output 字段
	if resp.Output != nil {
		respJSON["output"] = resp.Output
	} else {
		respJSON["output"] = []any{}
	}

	// 直接添加 usage 字段（结构体类型，不需要 nil 检查）
	respJSON["usage"] = resp.Usage

	body, err = json.Marshal(respJSON)
	if err != nil {
		logStep("openai_responses adapter: marshal response err=%v", err)
		return 0, "", nil, nil, err
	}

	// 调试输出：OpenAI Responses API 原始响应格式
	logStep("openai_responses adapter: response_original=%s", string(body))

	// 如果是 JSON 格式,尝试解析并美化输出（不转义 Unicode）
	var prettyJSON map[string]any
	if json.Unmarshal(body, &prettyJSON) == nil {
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(prettyJSON); err == nil {
			logStep("openai_responses adapter: response_pretty=\n%s", buffer.String())
		}
	}

	logStep("openai_responses adapter: success bodyLen=%d", len(body))
	return 200, "application/json", body, nil, nil
}

// executeStream 执行流式调用并返回 SSE 流
func (a *OpenAIResponsesAdapter) executeStream(ctx context.Context, client *openai.Client, params responses.ResponseNewParams) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	stream := client.Responses.NewStreaming(ctx, params)

	writeSSEEvent := func(writer *bufio.Writer, eventName string, eventData map[string]any) bool {
		writer.WriteString("event: " + eventName + "\n")
		if dataBytes, marshalErr := json.Marshal(eventData); marshalErr == nil {
			writer.WriteString("data: " + string(dataBytes) + "\n")
		} else {
			logStep("openai_responses adapter: marshal stream event err=%v, event=%s", marshalErr, eventName)
			fallbackBytes, _ := json.Marshal(map[string]any{
				"type":    "error",
				"message": "failed to encode stream event",
			})
			writer.WriteString("data: " + string(fallbackBytes) + "\n")
		}
		writer.WriteString("\n")
		if flushErr := writer.Flush(); flushErr != nil {
			logStep("openai_responses adapter: flush stream event err=%v, event=%s", flushErr, eventName)
			return false
		}
		return true
	}

	writeStreamError := func(writer *bufio.Writer, message string) bool {
		return writeSSEEvent(writer, "error", map[string]any{
			"type":    "error",
			"message": message,
		})
	}

	// 创建一个管道,将 SDK 的流式事件转换为 SSE 格式
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		writer := bufio.NewWriter(pw)

		for stream.Next() {
			event := stream.Current()

			// 调试输出：记录每个事件
			logStep("openai_responses adapter: stream event type=%s, delta=%s", event.Type, event.Delta)

			if strings.EqualFold(event.Type, "error") {
				message := strings.TrimSpace(event.Delta)
				if message == "" {
					message = "upstream returned stream error event"
				}
				logStep("openai_responses adapter: upstream emitted error event=%s", message)
				writeStreamError(writer, message)
				return
			}

			// 将 SDK 事件转换为标准 SSE 格式
			eventData := map[string]any{
				"type": event.Type,
			}

			// 根据事件类型添加相应字段
			switch event.Type {
			case "response.created":
				if event.Response.Error.Code != "" {
					logStep("openai_responses adapter: missing response payload for event=%s", event.Type)
					writeStreamError(writer, "upstream returned response.created without response payload")
					return
				}
				eventData["id"] = event.Response.ID
				eventData["model"] = event.Response.Model
				eventData["status"] = event.Response.Status
			case "response.output_text.delta":
				eventData["delta"] = event.Delta
				eventData["output_index"] = event.OutputIndex
				eventData["index"] = event.OutputIndex // backward compatibility for old parser
			case "response.output_text.done":
				eventData["output_index"] = event.OutputIndex
				eventData["index"] = event.OutputIndex // backward compatibility for old parser
			case "response.function_call_arguments.delta":
				eventData["delta"] = event.Delta
				eventData["output_index"] = event.OutputIndex
				eventData["item_id"] = event.ItemID
			case "response.function_call_arguments.done":
				eventData["arguments"] = event.Arguments
				eventData["name"] = event.Name
				eventData["output_index"] = event.OutputIndex
				eventData["item_id"] = event.ItemID
			case "response.output_item.added", "response.output_item.done":
				eventData["output_index"] = event.OutputIndex
				eventData["item"] = event.Item
			case "response.done":
				if event.Response.Error.Code != "" {
					logStep("openai_responses adapter: missing response payload for event=%s", event.Type)
					writeStreamError(writer, "upstream returned response.done without response payload")
					return
				}
				eventData["id"] = event.Response.ID
				eventData["status"] = event.Response.Status
				eventData["usage"] = event.Response.Usage
			}

			if ok := writeSSEEvent(writer, event.Type, eventData); !ok {
				return
			}
		}

		if err := stream.Err(); err != nil {
			logStep("openai_responses adapter: stream error=%v", err)
			writeStreamError(writer, err.Error())
		}
	}()

	logStep("openai_responses adapter: stream started, returning pipe")
	return 200, "text/event-stream", nil, pr, nil
}

// handleSDKError 处理 SDK 错误并返回适当的 HTTP 响应
func (a *OpenAIResponsesAdapter) handleSDKError(err error) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, retErr error) {
	// 空指针防护：确保 err 不为 nil
	if err == nil {
		logStep("openai_responses adapter: handleSDKError called with nil error")
		return 500, "application/json", []byte(`{"error":"internal error: nil error passed to handler"}`), nil, nil
	}

	var apiErr *openai.Error
	if errors, ok := err.(*openai.Error); ok && errors != nil {
		apiErr = errors

		// 安全地获取状态码，防止空指针
		statusCode = 502 // 默认值
		if apiErr.StatusCode > 0 {
			statusCode = apiErr.StatusCode
		}
		if statusCode < 400 {
			statusCode = 502
		}

		// 安全地获取响应体
		body = []byte(`{"error":"upstream service error"}`) // 默认错误消息
		if apiErr.DumpResponse != nil {
			if dumpBody := apiErr.DumpResponse(false); len(dumpBody) > 0 {
				body = dumpBody
			}
		}

		logStep("openai_responses adapter: API error status=%d, body=%s", statusCode, string(body))
		return statusCode, "application/json", body, nil, nil
	}

	// 非 API 错误
	logStep("openai_responses adapter: non-API error=%v", err)
	return 0, "", nil, nil, err
}
