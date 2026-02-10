package messages

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIAdapter 使用 go-openai 请求上游，将 Anthropic 请求/响应与 OpenAI 互转。
type OpenAIAdapter struct{}

// openaiErrorBody 从 go-openai 错误中取出 statusCode 和可返回的 body，供上层按 Claude 格式返回。
// 支持 APIError（OpenAI 标准）和 RequestError（含 Body，如 ModelScope 429 等）。
func openaiErrorBody(err error) (statusCode int, contentType string, body []byte) {
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		statusCode = reqErr.HTTPStatusCode
		contentType = "application/json"
		// 若上游带有原始 body（如 WAF/网关 JSON 或 HTML），直接透传，方便上层解析或展示；
		// 若 body 为空（如仅有“upstream error”之类的信息），构造一个标准的 error JSON，避免上层拿不到 message。
		if len(reqErr.Body) > 0 {
			body = reqErr.Body
		} else {
			out := map[string]any{"error": map[string]any{"message": err.Error()}}
			body, _ = json.Marshal(out)
		}
		logStep("openai adapter: request error status=%d bodyLen=%d", statusCode, len(body))
		return statusCode, contentType, body
	}
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) {
		return 0, "", nil
	}
	statusCode = apiErr.HTTPStatusCode
	if statusCode == 0 {
		statusCode = http.StatusBadGateway
	}
	contentType = "application/json"
	// 按 OpenAI / NewAPI 文档还原标准错误结构：
	// {"error":{"message":"...","type":"...","param":...,"code":...}}
	errObj := map[string]any{
		"message": apiErr.Message,
	}
	if apiErr.Type != "" {
		errObj["type"] = apiErr.Type
	}
	if apiErr.Param != nil {
		errObj["param"] = *apiErr.Param
	}
	if apiErr.Code != nil {
		errObj["code"] = apiErr.Code
	}
	out := map[string]any{"error": errObj}
	body, _ = json.Marshal(out)
	logStep("openai adapter: api error status=%d bodyLen=%d", statusCode, len(body))
	return statusCode, contentType, body
}

// Execute 使用 go-openai SDK 请求上游，再转回 Anthropic 格式。
func (a *OpenAIAdapter) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("openai adapter: start, stream=%v, baseURL=%s, model=%s", opts.Stream, opts.BaseURL, opts.UpstreamModel)

	messages := anthropicToOpenAIMessages(payload)
	if len(messages) == 0 {
		logStep("openai adapter: no messages, err=empty")
		return 0, "", nil, nil, errEmptyMessages
	}

	maxTokens := 4096
	if v, ok := payload["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}

	oaiReq := openai.ChatCompletionRequest{
		Model:     opts.UpstreamModel,
		MaxTokens: maxTokens,
		Stream:    opts.Stream,
		Messages:  openAIMessagesToSDK(messages),
	}
	if v, ok := payload["temperature"].(float64); ok {
		f := float32(v)
		oaiReq.Temperature = f
	}
	if v, ok := payload["top_p"].(float64); ok {
		f := float32(v)
		oaiReq.TopP = f
	}
	if !opts.MinimalOpenAI {
		if toolsIn, ok := payload["tools"].([]any); ok && len(toolsIn) > 0 {
			oaiReq.Tools = openAIToolsToSDK(anthropicToolsToOpenAI(toolsIn))
			oaiReq.ToolChoice = "auto"
			if tc, ok := payload["tool_choice"].(map[string]any); ok {
				if v, _ := tc["type"].(string); v == "none" {
					oaiReq.ToolChoice = "none"
				} else if name, _ := tc["name"].(string); v == "tool" && name != "" {
					oaiReq.ToolChoice = map[string]any{"type": "function", "function": map[string]any{"name": name}}
				}
			}
		}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	// go-openai 的 BaseURL 通常为 https://api.openai.com/v1
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}
	logStep("openai adapter: creating client baseURL=%s", baseURL)

	cfg := openai.DefaultConfig(opts.APIKey)
	cfg.BaseURL = baseURL
	cfg.HTTPClient = &http.Client{Timeout: 600 * time.Second}
	client := openai.NewClientWithConfig(cfg)

	if opts.Stream {
		stream, errStream := client.CreateChatCompletionStream(ctx, oaiReq)
		logStep("openai adapter: CreateChatCompletionStream done, err=%v", errStream)
		if errStream != nil {
			code, ct, errBody := openaiErrorBody(errStream)
			return code, ct, errBody, nil, errStream
		}
		// 转成 Anthropic SSE 格式的 ReadCloser；客户端断开时 ctx 取消，goroutine 会提前退出并关闭 stream。
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer stream.Close()
			convertOpenAIStreamToAnthropicWriter(ctx, stream, pw)
		}()
		return http.StatusOK, "text/event-stream", nil, pr, nil
	}

	resp, err := client.CreateChatCompletion(ctx, oaiReq)
	logStep("openai adapter: CreateChatCompletion done, err=%v", err)
	if err != nil {
		code, ct, errBody := openaiErrorBody(err)
		return code, ct, errBody, nil, err
	}

	// 将 go-openai 响应转为 Anthropic 格式
	out, err := openAIRespToAnthropic(resp)
	if err != nil {
		logStep("openai adapter: convert response err=%v", err)
		return 0, "", nil, nil, err
	}
	body, _ = json.Marshal(out)
	logStep("openai adapter: success status=200 bodyLen=%d", len(body))
	return http.StatusOK, "application/json", body, nil, nil
}

// openAIRespToAnthropic 将 go-openai ChatCompletionResponse 转为 Anthropic 响应结构。
func openAIRespToAnthropic(resp openai.ChatCompletionResponse) (*anthropicResp, error) {
	contentBlocks := []map[string]any{}
	stopReason := "end_turn"
	if len(resp.Choices) > 0 {
		msg := resp.Choices[0].Message
		if msg.Content != "" {
			contentBlocks = append(contentBlocks, map[string]any{"type": "text", "text": msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			inputVal := any(tc.Function.Arguments)
			var parsed map[string]any
			if json.Unmarshal([]byte(tc.Function.Arguments), &parsed) == nil {
				inputVal = parsed
			}
			contentBlocks = append(contentBlocks, map[string]any{
				"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": inputVal,
			})
		}
		stopReason = mapFinishReason(string(resp.Choices[0].FinishReason))
	}
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, map[string]any{"type": "text", "text": ""})
	}
	return &anthropicResp{
		ID: resp.ID, Type: "message", Role: "assistant",
		Content: contentBlocks, StopReason: stopReason, Model: resp.Model,
		Usage: anthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func openAIMessagesToSDK(msgs []openAIMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		msg := openai.ChatCompletionMessage{Role: m.role()}
		if m.Content != nil {
			if s, ok := m.Content.(string); ok {
				msg.Content = s
			}
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}
		out = append(out, msg)
	}
	return out
}

func (m openAIMessage) role() string {
	r := m.Role
	if r == "" {
		return "user"
	}
	return r
}

func openAIToolsToSDK(tools []openAITool) []openai.Tool {
	out := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}
	return out
}

// convertOpenAIStreamToAnthropicWriter 从 go-openai stream 读 chunk 并写入 Claude Code（Anthropic）流式 SSE 格式；ctx 取消时立即退出。
// 支持 text、tool_calls 和（若上游提供）thinking，参考 maxnowack/anthropic-proxy。
func convertOpenAIStreamToAnthropicWriter(ctx context.Context, stream *openai.ChatCompletionStream, w io.Writer) {
	first := true
	textBlockStarted := false
	encounteredToolCall := false
	toolArgs := make(map[int]string) // index -> 累积的 arguments

	for {
		if ctx.Err() != nil {
			return
		}
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// OpenAI SDK 在正常结束时不会给 [DONE]，这里仅在完全无内容时补齐空响应
				if first {
					writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","content":[],"model":""}}`)
					writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
					writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
					writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`)
					writeSSE(w, "message_stop", `{"type":"message_stop"}`)
				}
			}
			return
		}

		var (
			deltaText string
			finish    string
		)
		if len(chunk.Choices) > 0 {
			d := chunk.Choices[0].Delta
			deltaText = d.Content
			finish = string(chunk.Choices[0].FinishReason)
		}

		// 首个有效 chunk：发送 message_start / content_block_start
		if first {
			first = false
			writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","content":[],"model":""}}`)
			// 文本 block 仅在有 text 或 reasoning 时真正开始
		}

		// 处理 tool_calls（若 go-openai 暴露 ToolCalls 字段）
		if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			encounteredToolCall = true
			for _, tc := range chunk.Choices[0].Delta.ToolCalls {
				idx := tc.Index
				// 第一次见到该 index：发送 content_block_start
				if _, ok := toolArgs[*idx]; !ok {
					toolArgs[*idx] = ""
					startJSON, _ := json.Marshal(map[string]any{
						"type":  "content_block_start",
						"index": idx,
						"content_block": map[string]any{
							"type":  "tool_use",
							"id":    tc.ID,
							"name":  tc.Function.Name,
							"input": map[string]any{
								// 具体 JSON 由后续 input_json_delta 补齐
							},
						},
					})
					writeSSE(w, "content_block_start", string(startJSON))
				}
				newArgs := tc.Function.Arguments
				oldArgs := toolArgs[*idx]
				if len(newArgs) > len(oldArgs) {
					deltaJSON := newArgs[len(oldArgs):]
					toolArgs[*idx] = newArgs
					deltaPayload, _ := json.Marshal(map[string]any{
						"type":  "content_block_delta",
						"index": idx,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": deltaJSON,
						},
					})
					writeSSE(w, "content_block_delta", string(deltaPayload))
				}
			}
		} else if deltaText != "" {
			// 纯文本 delta
			if !textBlockStarted {
				textBlockStarted = true
				writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			}
			escaped, _ := json.Marshal(deltaText)
			writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(escaped)+`}}`)
		}

		if finish != "" {
			// 根据是否有 tool 调用决定 stop_reason
			stopReason := mapFinishReason(finish)
			if encounteredToolCall {
				stopReason = "tool_use"
			}
			stopJSON, _ := json.Marshal(stopReason)

			// 结束各类 content_block
			if encounteredToolCall {
				for idx := range toolArgs {
					stopPayload, _ := json.Marshal(map[string]any{
						"type":  "content_block_stop",
						"index": idx,
					})
					writeSSE(w, "content_block_stop", string(stopPayload))
				}
			} else if textBlockStarted {
				writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
			}

			writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":`+string(stopJSON)+`}}`)
			writeSSE(w, "message_stop", `{"type":"message_stop"}`)
			return
		}
	}
}

var _ = time.Second

// openAIReq 用于构建上游请求体（仅需字段）
type openAIReq struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"` // "none"|"auto"|"required" 或 {"type":"function","function":{"name":"..."}}
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	TopP        *float32        `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type openAITool struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description,omitempty"`
		Parameters  interface{} `json:"parameters,omitempty"` // JSON Schema，对应 Anthropic 的 input_schema
	} `json:"function"`
}

// openAIMessage 支持 text、assistant+tool_calls、tool 三种形态
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content,omitempty"` // string 或 null（当有 tool_calls 时）
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"` // role=tool 时
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// anthropicResp 用于输出 Anthropic 格式响应（非流式）
type anthropicResp struct {
	ID         string           `json:"id,omitempty"`
	Type       string           `json:"type,omitempty"`
	Role       string           `json:"role,omitempty"`
	Content    []map[string]any `json:"content,omitempty"` // [{type:"text",text:"..."} 或 {type:"tool_use",id,name,input}]
	StopReason string           `json:"stop_reason,omitempty"`
	Model      string           `json:"model,omitempty"`
	Usage      anthropicUsage   `json:"usage,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// anthropicToolsToOpenAI 将 Anthropic tools[] 转为 OpenAI tools[]。每项为 { name, description, input_schema } -> { type: "function", function: { name, description, parameters } }。
func anthropicToolsToOpenAI(tools []any) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := m["description"].(string)
		schema := m["input_schema"]
		ot := openAITool{Type: "function"}
		ot.Function.Name = name
		ot.Function.Description = desc
		if schema != nil {
			ot.Function.Parameters = schema
		}
		out = append(out, ot)
	}
	return out
}

func anthropicToOpenAIMessages(payload map[string]any) []openAIMessage {
	var out []openAIMessage

	if sys, ok := payload["system"].(string); ok && strings.TrimSpace(sys) != "" {
		out = append(out, openAIMessage{Role: "system", Content: sys})
	}

	msgs, ok := payload["messages"].([]any)
	if !ok {
		return out
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
		converted := anthropicMessageToOpenAI(role, mm)
		out = append(out, converted...)
	}
	return out
}

// anthropicMessageToOpenAI 将单条 Anthropic 消息转为一条或多条 OpenAI 消息（user 含 tool_result 时会拆成多条 tool + 可选 user）。
func anthropicMessageToOpenAI(role string, mm map[string]any) []openAIMessage {
	contentRaw := mm["content"]
	switch role {
	case "assistant":
		text, toolUses := extractAnthropicAssistantContent(contentRaw)
		if len(toolUses) > 0 {
			msg := openAIMessage{Role: "assistant"}
			if text != "" {
				msg.Content = text
			} else {
				msg.Content = nil
			}
			for _, tu := range toolUses {
				msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
					ID:   tu.id,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: tu.name, Arguments: tu.input},
				})
			}
			return []openAIMessage{msg}
		}
		return []openAIMessage{{Role: "assistant", Content: text}}
	case "user":
		text, toolResults := extractAnthropicUserContent(contentRaw)
		if len(toolResults) == 0 {
			return []openAIMessage{{Role: "user", Content: text}}
		}
		var list []openAIMessage
		for _, tr := range toolResults {
			list = append(list, openAIMessage{Role: "tool", ToolCallID: tr.toolUseID, Content: tr.content})
		}
		if strings.TrimSpace(text) != "" {
			list = append(list, openAIMessage{Role: "user", Content: text})
		}
		return list
	default:
		content := extractAnthropicMessageContent(contentRaw)
		return []openAIMessage{{Role: role, Content: content}}
	}
}

type anthropicToolUse struct {
	id    string
	name  string
	input string
}

type anthropicToolResult struct {
	toolUseID string
	content   string
}

func extractAnthropicAssistantContent(contentRaw any) (text string, toolUses []anthropicToolUse) {
	switch c := contentRaw.(type) {
	case string:
		return c, nil
	case []any:
		var sb strings.Builder
		for _, blk := range c {
			bm, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			t, _ := bm["type"].(string)
			switch t {
			case "text":
				if txt, ok := bm["text"].(string); ok {
					sb.WriteString(txt)
				}
			case "tool_use":
				id, _ := bm["id"].(string)
				name, _ := bm["name"].(string)
				inputStr := ""
				if inp, ok := bm["input"].(map[string]any); ok {
					b, _ := json.Marshal(inp)
					inputStr = string(b)
				} else if s, ok := bm["input"].(string); ok {
					inputStr = s
				}
				toolUses = append(toolUses, anthropicToolUse{id: id, name: name, input: inputStr})
			}
		}
		return sb.String(), toolUses
	}
	return "", nil
}

func extractAnthropicUserContent(contentRaw any) (text string, toolResults []anthropicToolResult) {
	switch c := contentRaw.(type) {
	case string:
		return c, nil
	case []any:
		var sb strings.Builder
		for _, blk := range c {
			bm, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			t, _ := bm["type"].(string)
			switch t {
			case "text":
				if txt, ok := bm["text"].(string); ok {
					sb.WriteString(txt)
				}
			case "tool_result":
				toolUseID, _ := bm["tool_use_id"].(string)
				content := ""
				if cnt, ok := bm["content"].(string); ok {
					content = cnt
				} else if cnt, ok := bm["content"].([]any); ok {
					// Anthropic 允许 content 为 content block 数组，这里简化为 JSON 字符串
					b, _ := json.Marshal(cnt)
					content = string(b)
				}
				toolResults = append(toolResults, anthropicToolResult{toolUseID: toolUseID, content: content})
			}
		}
		return sb.String(), toolResults
	}
	return "", nil
}

func extractAnthropicMessageContent(contentRaw any) string {
	var sb strings.Builder
	switch c := contentRaw.(type) {
	case string:
		sb.WriteString(c)
	case []any:
		for _, blk := range c {
			bm, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "" && t != "text" {
				continue
			}
			if txt, ok := bm["text"].(string); ok {
				sb.WriteString(txt)
			}
		}
	}
	return sb.String()
}

func mapFinishReason(r string) string {
	switch r {
	case "stop", "end_turn":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return r
	}
}

var errEmptyMessages = &protocolError{msg: "no messages to send"}

type protocolError struct{ msg string }

func (e *protocolError) Error() string { return e.msg }

func writeSSE(w io.Writer, event, data string) {
	io.WriteString(w, "event: "+event+"\n")
	io.WriteString(w, "data: "+data+"\n\n")
}

// openAIStreamChunk 用于解析 OpenAI 流式 SSE 的 data 行（text + tool_calls）。
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		Finish *string `json:"finish_reason"`
	} `json:"choices"`
}

// ConvertOpenAIStreamReaderToAnthropic 从原始 OpenAI SSE io.Reader 读 chunk，写入 Anthropic 格式到 w；供 NewAPI 等自建 HTTP 的适配器使用。
func ConvertOpenAIStreamReaderToAnthropic(ctx context.Context, r io.Reader, w io.Writer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 512*1024)
	first := true
	textBlockStarted := false
	encounteredToolCall := false
	toolArgs := make(map[int]string)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Bytes()
		if len(line) < 6 || string(line[:5]) != "data:" {
			continue
		}
		data := bytes.TrimSpace(line[5:])
		if len(data) == 0 {
			continue
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			// 结束：根据是否有 tool 调用决定 stop_reason
			if first {
				writeSSE(w, "message_start", `{"type":"message_start","message\":{\"id\":\"\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"\"}}`)
				writeSSE(w, "content_block_start", `{"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}`)
				writeSSE(w, "content_block_stop", `{"type\":\"content_block_stop\",\"index\":0}`)
				writeSSE(w, "message_delta", `{"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}`)
			} else {
				if encounteredToolCall {
					for idx := range toolArgs {
						stopPayload, _ := json.Marshal(map[string]any{
							"type":  "content_block_stop",
							"index": idx,
						})
						writeSSE(w, "content_block_stop", string(stopPayload))
					}
					writeSSE(w, "message_delta", `{"type":"message_delta","delta\":{\"stop_reason\":\"tool_use\"}}`)
				} else if textBlockStarted {
					writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
					writeSSE(w, "message_delta", `{"type":"message_delta","delta\":{\"stop_reason\":\"end_turn\"}}`)
				}
			}
			writeSSE(w, "message_stop", `{"type":"message_stop"}`)
			return
		}

		var chunk openAIStreamChunk
		if json.Unmarshal(data, &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		deltaText := d.Content
		finish := ""
		if chunk.Choices[0].Finish != nil {
			finish = *chunk.Choices[0].Finish
		}

		if first {
			first = false
			writeSSE(w, "message_start", `{"type":"message_start","message\":{\"id\":\"\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"\"}}`)
			// 文本 block 延后到有 text/reasoning 时再开始
		}

		// tool_calls
		if len(d.ToolCalls) > 0 {
			encounteredToolCall = true
			for _, tc := range d.ToolCalls {
				idx := tc.Index
				if _, ok := toolArgs[idx]; !ok {
					toolArgs[idx] = ""
					startJSON, _ := json.Marshal(map[string]any{
						"type":  "content_block_start",
						"index": idx,
						"content_block": map[string]any{
							"type":  "tool_use",
							"id":    tc.ID,
							"name":  tc.Function.Name,
							"input": map[string]any{
								// 具体 JSON 由后续 input_json_delta 补齐
							},
						},
					})
					writeSSE(w, "content_block_start", string(startJSON))
				}
				newArgs := tc.Function.Arguments
				oldArgs := toolArgs[idx]
				if len(newArgs) > len(oldArgs) {
					deltaJSON := newArgs[len(oldArgs):]
					toolArgs[idx] = newArgs
					deltaPayload, _ := json.Marshal(map[string]any{
						"type":  "content_block_delta",
						"index": idx,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": deltaJSON,
						},
					})
					writeSSE(w, "content_block_delta", string(deltaPayload))
				}
			}
		} else if deltaText != "" {
			if !textBlockStarted {
				textBlockStarted = true
				writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			}
			escaped, _ := json.Marshal(deltaText)
			writeSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(escaped)+`}}`)
		}

		if finish != "" {
			stopReason := mapFinishReason(finish)
			if encounteredToolCall {
				stopReason = "tool_use"
			}
			stopJSON, _ := json.Marshal(stopReason)

			if encounteredToolCall {
				for idx := range toolArgs {
					stopPayload, _ := json.Marshal(map[string]any{
						"type":  "content_block_stop",
						"index": idx,
					})
					writeSSE(w, "content_block_stop", string(stopPayload))
				}
			} else if textBlockStarted {
				writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
			}
			writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":`+string(stopJSON)+`}}`)
			writeSSE(w, "message_stop", `{"type":"message_stop"}`)
			return
		}
	}

	if first {
		writeSSE(w, "message_start", `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","content":[],"model":""}}`)
		writeSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`)
	}
	writeSSE(w, "message_stop", `{"type":"message_stop"}`)
}
