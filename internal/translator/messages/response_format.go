package messages

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// ConvertAnthropicStreamToOpenAIResponses 将 Anthropic SSE 流式响应转换为 OpenAI Responses API SSE 格式。
// Anthropic 格式: message_start -> content_block_start -> content_block_delta -> content_block_stop -> message_delta -> message_stop
// OpenAI Responses 格式: response_start -> output_start -> content_delta -> output_stop -> response_stop
func ConvertAnthropicStreamToOpenAIResponses(ctx context.Context, r io.Reader, w io.Writer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 512*1024)

	var (
		responseStarted bool
		outputStarted   bool
		_               bool
		fullContent     bytes.Buffer
		stopReason      string
		totalTokens     int
	)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}

		line := scanner.Bytes()
		if len(line) < 6 || string(line[:6]) != "event:" {
			continue
		}

		eventType := string(bytes.TrimSpace(line[6:]))

		// 读取对应的 data 行
		if !scanner.Scan() {
			break
		}
		dataLine := scanner.Bytes()
		if len(dataLine) < 6 || string(dataLine[:5]) != "data:" {
			continue
		}
		dataStr := string(bytes.TrimSpace(dataLine[5:]))

		switch eventType {
		case "message_start":
			if !responseStarted {
				responseStarted = true
				writeOpenAIResponsesSSE(w, "response_start", map[string]any{
					"type":       "response_start",
					"id":         "resp-" + generateID(),
					"object":     "response",
					"created_at": int(time.Now().Unix()),
					"status":     "in_progress",
					"model":      "unknown", // 上游已经设置，这里只作占位
				})
			}

		case "content_block_start":
			var event map[string]any
			if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
				continue
			}
			// Anthropic content_block_start 仅表示一个块开始，在 OpenAI Responses 中对应 output_start
			if !outputStarted {
				outputStarted = true
				_ = true
				writeOpenAIResponsesSSE(w, "output_start", map[string]any{
					"type":   "output_start",
					"id":     "output-" + generateID(),
					"status": "in_progress",
					"role":   "assistant",
				})
			}

		case "content_block_delta":
			var event map[string]any
			if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
				continue
			}
			// 提取 delta 内容
			if delta, ok := event["delta"].(map[string]any); ok {
				if text, ok := delta["text"].(string); ok && text != "" {
					fullContent.WriteString(text)
					// 流式发送文本增量
					writeOpenAIResponsesSSE(w, "content_delta", map[string]any{
						"type":  "content_delta",
						"index": 0,
						"delta": map[string]any{
							"type": "text_delta",
							"text": text,
						},
					})
				}
			}

		case "message_delta":
			var event map[string]any
			if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
				continue
			}
			// 提取 stop_reason
			if delta, ok := event["delta"].(map[string]any); ok {
				if reason, ok := delta["stop_reason"].(string); ok {
					stopReason = reason
				}
			}

		case "message_stop":
			// 消息结束，发送最终的内容块和响应结束
			if outputStarted {
				writeOpenAIResponsesSSE(w, "output_stop", map[string]any{
					"type":   "output_stop",
					"status": "completed",
				})
			}

			// 发送最终的响应停止事件，包含完整的输出和使用统计
			writeOpenAIResponsesSSE(w, "response_stop", map[string]any{
				"type":        "response_stop",
				"status":      "completed",
				"stop_reason": mapStopReason(stopReason),
				"usage": map[string]any{
					"prompt_tokens":     0, // 上游应提供，这里作占位
					"completion_tokens": 0,
					"total_tokens":      totalTokens,
				},
			})
		}

		// 读完事件和数据后，跳过空行
		if scanner.Scan() {
			// 空行
		}
	}
}

// writeOpenAIResponsesSSE 按 OpenAI Responses SSE 格式写入事件。
func writeOpenAIResponsesSSE(w io.Writer, event string, data map[string]any) {
	io.WriteString(w, "event: "+event+"\n")
	if b, err := json.Marshal(data); err == nil {
		io.WriteString(w, "data: "+string(b)+"\n")
	}
	io.WriteString(w, "\n")
}

// mapStopReason 将 Anthropic stop_reason 映射为 OpenAI Responses 的停止原因。
func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "completed"
	case "max_tokens":
		return "max_output_tokens"
	case "tool_use":
		return "tool_calls"
	default:
		return "completed"
	}
}

// generateID 生成随机 ID（简化实现）。
func generateID() string {
	return time.Now().Format("20060102150405") // 可根据需要改为 UUID
}

// writeAnthropicSSE 按 Anthropic SSE 格式写入事件。
func writeAnthropicSSE(w io.Writer, event string, data map[string]any) {
	io.WriteString(w, "event: "+event+"\n")
	if b, err := json.Marshal(data); err == nil {
		io.WriteString(w, "data: "+string(b)+"\n")
	}
	io.WriteString(w, "\n")
}

// WriteAnthropicSSE 是 writeAnthropicSSE 的导出版本，供 handler 使用
func WriteAnthropicSSE(w io.Writer, event string, data map[string]any) {
	writeAnthropicSSE(w, event, data)
}

// mapStopReasonReverse 将 OpenAI Responses 的 stop_reason 映射为 Anthropic。
func mapStopReasonReverse(reason string) string {
	switch reason {
	case "completed":
		return "end_turn"
	case "max_output_tokens":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return "end_turn"
	}
}

// ConvertOpenAIResponsesStreamToAnthropic 将 OpenAI Responses API SSE 流转换为 Anthropic SSE 格式。
// 当上游为 openai_responses 且客户端需要 Anthropic 格式时使用。
func ConvertOpenAIResponsesStreamToAnthropic(ctx context.Context, r io.Reader, w io.Writer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, 512*1024)

	var (
		messageStarted    bool
		currentBlockIndex int
		blockIndexMap     map[int]int // OpenAI output index -> Anthropic block index
		stopReason        string
		currentToolUseID  string
		currentToolName   string
		toolInputBuffer   strings.Builder
	)
	blockIndexMap = make(map[int]int)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Bytes()
		if len(line) < 6 || string(line[:6]) != "event:" {
			continue
		}
		eventType := string(bytes.TrimSpace(line[6:]))
		if !scanner.Scan() {
			break
		}
		dataLine := scanner.Bytes()
		if len(dataLine) < 5 || string(dataLine[:5]) != "data:" {
			continue
		}
		dataStr := string(bytes.TrimSpace(dataLine[5:]))

		// 调试输出：记录 OpenAI Responses API 的原始事件
		logStep("OpenAI Responses stream event: type=%s, data=%s", eventType, dataStr)

		switch eventType {
		case "response.created", "response_start":
			// OpenAI Responses API 使用 "response.created" 或旧版 "response_start"
			if !messageStarted {
				messageStarted = true
				var obj map[string]any
				if json.Unmarshal([]byte(dataStr), &obj) == nil {
					writeAnthropicSSE(w, "message_start", map[string]any{
						"type": "message_start",
						"message": map[string]any{
							"id":    getStr(obj, "id", "msg-"+generateID()),
							"type":  "message",
							"role":  "assistant",
							"model": getStr(obj, "model", "unknown"),
							"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
						},
					})
				}
			}

		case "response.in_progress":
			// 进行中状态，跳过
			continue

		case "response.output_item.added":
			// 新的输出项添加，准备开始新块
			// 注意：实际的块开始在收到具体内容时才发送
			continue

		case "response.output_text.delta":
			// OpenAI Responses API 的文本增量事件
			if !messageStarted {
				messageStarted = true
				writeAnthropicSSE(w, "message_start", map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"id":    "msg-" + generateID(),
						"type":  "message",
						"role":  "assistant",
						"model": "unknown",
						"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
					},
				})
			}

			var obj map[string]any
			if json.Unmarshal([]byte(dataStr), &obj) == nil {
				outputIdx := int(getFloat(obj, "output_index", 0))
				delta, _ := obj["delta"].(string)

				// 检查是否需要发送 content_block_start
				if _, exists := blockIndexMap[outputIdx]; !exists {
					blockIndexMap[outputIdx] = currentBlockIndex
					writeAnthropicSSE(w, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": currentBlockIndex,
						"content_block": map[string]any{
							"type": "text",
							"text": "",
						},
					})
					currentBlockIndex++
				}

				if delta != "" {
					writeAnthropicSSE(w, "content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": blockIndexMap[outputIdx],
						"delta": map[string]any{
							"type": "text_delta",
							"text": delta,
						},
					})
				}
			}

		case "response.function_call_arguments.delta":
			// 工具调用参数增量
			if !messageStarted {
				messageStarted = true
				writeAnthropicSSE(w, "message_start", map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"id":    "msg-" + generateID(),
						"type":  "message",
						"role":  "assistant",
						"model": "unknown",
						"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
					},
				})
			}

			var obj map[string]any
			if json.Unmarshal([]byte(dataStr), &obj) == nil {
				outputIdx := int(getFloat(obj, "output_index", 0))
				delta, _ := obj["delta"].(map[string]any)

				// 首次遇到此输出项，发送 content_block_start
				if _, exists := blockIndexMap[outputIdx]; !exists {
					blockIndexMap[outputIdx] = currentBlockIndex

					// 从 delta 中提取工具信息
					if delta != nil {
						if name, ok := delta["name"].(string); ok && name != "" {
							currentToolName = name
						}
						currentToolUseID = "toolu_" + generateID()
					}

					writeAnthropicSSE(w, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": currentBlockIndex,
						"content_block": map[string]any{
							"type": "tool_use",
							"id":   currentToolUseID,
							"name": currentToolName,
						},
					})
					currentBlockIndex++
					toolInputBuffer.Reset()
				}

				// 累积工具参数
				if delta != nil {
					// 提取命令和描述等参数
					for key, val := range delta {
						if key == "name" {
							// name 已在 start 中处理
							continue
						}
						if strVal, ok := val.(string); ok && strVal != "" {
							if toolInputBuffer.Len() > 0 {
								toolInputBuffer.WriteString(",")
							}
							toolInputBuffer.WriteString(`"` + key + `":"` + escapeJSON(strVal) + `"`)
						}
					}
				}
			}

		case "response.function_call_arguments.done":
			// 工具调用参数完成
			var obj map[string]any
			if json.Unmarshal([]byte(dataStr), &obj) == nil {
				outputIdx := int(getFloat(obj, "output_index", 0))
				if anthropicIdx, exists := blockIndexMap[outputIdx]; exists {
					// 发送完整的 input_json delta
					fullInput := "{" + toolInputBuffer.String() + "}"
					writeAnthropicSSE(w, "content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": anthropicIdx,
						"delta": map[string]any{
							"type":       "input_json_delta",
							"partial_json": fullInput,
						},
					})
				}
			}

		case "response.output_item.done":
			// 输出项完成
			var obj map[string]any
			if json.Unmarshal([]byte(dataStr), &obj) == nil {
				outputIdx := int(getFloat(obj, "output_index", 0))
				if anthropicIdx, exists := blockIndexMap[outputIdx]; exists {
					writeAnthropicSSE(w, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": anthropicIdx,
					})
				}
			}

		case "response.output_text.done", "output_stop":
			// 旧版输出完成事件
			var obj map[string]any
			if json.Unmarshal([]byte(dataStr), &obj) == nil {
				outputIdx := int(getFloat(obj, "output_index", 0))
				if anthropicIdx, exists := blockIndexMap[outputIdx]; exists {
					writeAnthropicSSE(w, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": anthropicIdx,
					})
				}
			}

		case "response.completed", "response.done", "response_stop":
			// OpenAI Responses API 的响应完成事件
			var obj map[string]any
			if json.Unmarshal([]byte(dataStr), &obj) == nil {
				if reason, _ := obj["stop_reason"].(string); reason != "" {
					stopReason = reason
				}
				if stopReason == "" {
					if status, _ := obj["status"].(string); status == "completed" {
						stopReason = "completed"
					}
				}
			}
			// 如果是 response.completed，推断为 tool_use
			if eventType == "response.completed" && stopReason == "" {
				stopReason = "tool_calls"
			}

			writeAnthropicSSE(w, "message_delta", map[string]any{
				"type":  "message_delta",
				"delta": map[string]any{"stop_reason": mapStopReasonReverse(stopReason)},
			})
			writeAnthropicSSE(w, "message_stop", map[string]any{"type": "message_stop"})
		}
		if scanner.Scan() {
			// 空行
		}
	}
}

func getStr(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func getFloat(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// ConvertOpenAIResponsesMessageToAnthropic 将 OpenAI Responses API 非流式 JSON 转为 Anthropic message JSON。
// 输出结构与 openai.go 的 openAIRespToAnthropic 一致，供 Claude Code 等客户端使用：id, type, role, content, stop_reason, model, usage。
// 支持多种 output 类型：message(文本)、function_call(工具调用) 等
func ConvertOpenAIResponsesMessageToAnthropic(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	var content []map[string]any

	// 遍历 output 数组，提取所有内容块（文本、工具调用等）
	outputItems := resp["output"]
	switch out := outputItems.(type) {
	case []any:
		for _, it := range out {
			item, _ := it.(map[string]any)
			if item == nil {
				continue
			}

			itemType, _ := item["type"].(string)

			switch itemType {
			case "message":
				// 提取文本内容
				textBlocks := extractOutputTextContent(item)
				content = append(content, textBlocks...)

			case "function_call":
				// 转换为 Anthropic 的 tool_use 格式
				toolUse := map[string]any{
					"type": "tool_use",
					"id":   getStr(item, "call_id", "call_"+generateID()),
					"name": getStr(item, "name", "unknown"),
				}

				// 解析 arguments (可能是 JSON 字符串)
				args := item["arguments"]
				if argsStr, ok := args.(string); ok && argsStr != "" {
					var argsObj map[string]any
					if err := json.Unmarshal([]byte(argsStr), &argsObj); err == nil {
						toolUse["input"] = argsObj
					} else {
						// 如果解析失败，使用空对象
						toolUse["input"] = map[string]any{}
					}
				} else if argsMap, ok := args.(map[string]any); ok {
					toolUse["input"] = argsMap
				} else {
					toolUse["input"] = map[string]any{}
				}

				content = append(content, toolUse)
			}
		}

	case map[string]any:
		// 单个 output 对象
		itemType, _ := out["type"].(string)
		if itemType == "message" {
			content = extractOutputTextContent(out)
		} else if itemType == "function_call" {
			toolUse := map[string]any{
				"type": "tool_use",
				"id":   getStr(out, "call_id", "call_"+generateID()),
				"name": getStr(out, "name", "unknown"),
			}

			args := out["arguments"]
			if argsStr, ok := args.(string); ok && argsStr != "" {
				var argsObj map[string]any
				if err := json.Unmarshal([]byte(argsStr), &argsObj); err == nil {
					toolUse["input"] = argsObj
				} else {
					toolUse["input"] = map[string]any{}
				}
			} else if argsMap, ok := args.(map[string]any); ok {
				toolUse["input"] = argsMap
			} else {
				toolUse["input"] = map[string]any{}
			}

			content = append(content, toolUse)
		}
	}

	// 兼容部分网关返回 message.content 为字符串
	if len(content) == 0 {
		if msg, _ := resp["message"].(map[string]any); msg != nil {
			if s, ok := msg["content"].(string); ok {
				content = []map[string]any{{"type": "text", "text": s}}
			}
		}
	}

	// 与 openai.openAIRespToAnthropic 一致：至少一个 content 块，无内容时用空字符串
	if len(content) == 0 {
		content = []map[string]any{{"type": "text", "text": ""}}
	}

	inputTok, outputTok := parseUsageTokens(resp["usage"])
	stopReason := parseStopReason(resp)
	id := getStr(resp, "id", "msg-"+generateID())
	model := getStr(resp, "model", "unknown")

	out := map[string]any{
		"id":          id,
		"type":        "message",
		"role":        "assistant",
		"content":     content,
		"stop_reason": stopReason,
		"model":       model,
		"usage":       map[string]any{"input_tokens": inputTok, "output_tokens": outputTok},
	}
	return json.Marshal(out)
}

// extractOutputTextContent 从 output 项（type=message）中取出 content 数组，将 output_text 转为 Anthropic 的 text 块。
func extractOutputTextContent(item map[string]any) []map[string]any {
	var content []map[string]any
	raw, _ := item["content"].([]any)
	for _, c := range raw {
		blk, _ := c.(map[string]any)
		if blk == nil {
			continue
		}
		if typ, _ := blk["type"].(string); typ != "output_text" {
			continue
		}
		text, _ := blk["text"].(string)
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	return content
}

// parseUsageTokens 从 usage 中解析 token 数，兼容 input_tokens/output_tokens 与 prompt_tokens/completion_tokens。
func parseUsageTokens(u any) (inputTok, outputTok int) {
	m, ok := u.(map[string]any)
	if !ok {
		return 0, 0
	}
	if n, ok := m["input_tokens"].(float64); ok {
		inputTok = int(n)
	} else if n, ok := m["prompt_tokens"].(float64); ok {
		inputTok = int(n)
	}
	if n, ok := m["output_tokens"].(float64); ok {
		outputTok = int(n)
	} else if n, ok := m["completion_tokens"].(float64); ok {
		outputTok = int(n)
	}
	return inputTok, outputTok
}

// parseStopReason 从响应中解析 stop_reason，与 openai mapFinishReason 语义对齐。
func parseStopReason(resp map[string]any) string {
	if r, ok := resp["stop_reason"].(string); ok && r != "" {
		return mapStopReasonReverse(r)
	}
	// 从 output 首项或 status 推断
	if out, ok := resp["output"].([]any); ok && len(out) > 0 {
		if item, _ := out[0].(map[string]any); item != nil {
			if r, ok := item["stop_reason"].(string); ok && r != "" {
				return mapStopReasonReverse(r)
			}
		}
	}
	return "end_turn"
}

// ConvertAnthropicMessageToOpenAIResponses 将 Anthropic 非流式 message JSON 转为 OpenAI Responses API 的 JSON。
// 参考 any-api 等网关的协议：同一入口可按模型配置返回不同响应格式。
// Anthropic: id, type, role, content:[{type:"text",text}], stop_reason, usage
// OpenAI Responses: id, object, created, model, output:[{type:"message",role,content:[{type:"output_text",text}]}], status, usage
func ConvertAnthropicMessageToOpenAIResponses(body []byte) ([]byte, error) {
	var anth map[string]any
	if err := json.Unmarshal(body, &anth); err != nil {
		return nil, err
	}

	// 从 Anthropic content 抽取文本，转为 output_text 块
	var outputContent []map[string]any
	if raw, ok := anth["content"].([]any); ok {
		for _, it := range raw {
			blk, _ := it.(map[string]any)
			if blk == nil {
				continue
			}
			t, _ := blk["type"].(string)
			if t != "text" {
				continue
			}
			text, _ := blk["text"].(string)
			outputContent = append(outputContent, map[string]any{
				"type":        "output_text",
				"text":       text,
				"annotations": []any{},
			})
		}
	}
	if outputContent == nil {
		outputContent = []map[string]any{}
	}

	role, _ := anth["role"].(string)
	if role == "" {
		role = "assistant"
	}
	model, _ := anth["model"].(string)
	if model == "" {
		model = "unknown"
	}

	// usage
	inputTok := 0
	outputTok := 0
	if u, ok := anth["usage"].(map[string]any); ok {
		if n, ok := u["input_tokens"].(float64); ok {
			inputTok = int(n)
		}
		if n, ok := u["output_tokens"].(float64); ok {
			outputTok = int(n)
		}
	}
	totalTok := inputTok + outputTok

	out := map[string]any{
		"id":      anth["id"],
		"object":  "response",
		"created": int(time.Now().Unix()),
		"model":   model,
		"output": []map[string]any{
			{
				"type":    "message",
				"role":   role,
				"content": outputContent,
			},
		},
		"status": "completed",
		"usage": map[string]any{
			"input_tokens":      inputTok,
			"output_tokens":     outputTok,
			"completion_tokens": outputTok,
			"total_tokens":      totalTok,
		},
	}
	return json.Marshal(out)
}

// ConvertOpenAIResponsesJSONToStream 将同步的 OpenAI Responses JSON 响应转换为 SSE 流格式。
func ConvertOpenAIResponsesJSONToStream(ctx context.Context, body []byte, w io.Writer) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	// 发送 response.created 事件
	writeOpenAIResponsesSSE(w, "response.created", map[string]any{
		"type":   "response.created",
		"id":     resp["id"],
		"model":  resp["model"],
		"status": "in_progress",
	})

	// 提取输出内容
	output, ok := resp["output"].([]any)
	if !ok || len(output) == 0 {
		return
	}

	for idx, item := range output {
		if ctx.Err() != nil {
			return
		}

		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// 发送 response.output_text.delta 事件
		if text, ok := itemMap["content"].(string); ok && text != "" {
			// 模拟流式输出：将文本分块发送
			chunkSize := 10
			for i := 0; i < len(text); i += chunkSize {
				end := i + chunkSize
				if end > len(text) {
					end = len(text)
				}
				chunk := text[i:end]

				writeOpenAIResponsesSSE(w, "response.output_text.delta", map[string]any{
					"type":  "response.output_text.delta",
					"delta": chunk,
					"index": idx,
				})
			}
		}

		// 发送 response.output_text.done 事件
		writeOpenAIResponsesSSE(w, "response.output_text.done", map[string]any{
			"type":  "response.output_text.done",
			"index": idx,
		})
	}

	// 发送 response.done 事件
	writeOpenAIResponsesSSE(w, "response.done", map[string]any{
		"type":   "response.done",
		"id":     resp["id"],
		"status": resp["status"],
		"usage":  resp["usage"],
	})
}

// ConvertAnthropicJSONToStream 将同步的 Anthropic JSON 响应转换为 SSE 流格式。
func ConvertAnthropicJSONToStream(ctx context.Context, body []byte, w io.Writer) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	// 发送 message_start 事件
	writeAnthropicSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":           resp["id"],
			"type":         "message",
			"role":         resp["role"],
			"content":      []any{},
			"model":        resp["model"],
			"stop_reason":  nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})

	// 提取 content 数组
	content, ok := resp["content"].([]any)
	if !ok {
		content = []any{}
	}

	for idx, block := range content {
		if ctx.Err() != nil {
			return
		}

		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}

		blockType, _ := blockMap["type"].(string)

		// 发送 content_block_start 事件
		writeAnthropicSSE(w, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": idx,
			"content_block": map[string]any{
				"type": blockType,
			},
		})

		// 发送 content_block_delta 事件
		if blockType == "text" {
			if text, ok := blockMap["text"].(string); ok && text != "" {
				// 模拟流式输出：将文本分块发送
				chunkSize := 10
				for i := 0; i < len(text); i += chunkSize {
					end := i + chunkSize
					if end > len(text) {
						end = len(text)
					}
					chunk := text[i:end]

					writeAnthropicSSE(w, "content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": idx,
						"delta": map[string]any{
							"type": "text_delta",
							"text": chunk,
						},
					})
				}
			}
		} else if blockType == "tool_use" {
			// 工具调用块
			writeAnthropicSSE(w, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{
					"type":  "tool_use_delta",
					"input": blockMap["input"],
				},
			})
		}

		// 发送 content_block_stop 事件
		writeAnthropicSSE(w, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": idx,
		})
	}

	// 发送 message_delta 事件
	usage := resp["usage"]
	writeAnthropicSSE(w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   resp["stop_reason"],
			"stop_sequence": resp["stop_sequence"],
		},
		"usage": usage,
	})

	// 发送 message_stop 事件
	writeAnthropicSSE(w, "message_stop", map[string]any{
		"type": "message_stop",
	})
}
