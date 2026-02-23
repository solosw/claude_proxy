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
)

// CodexStrategy Codex 运营商：将 Anthropic Messages API 转换为 Codex Responses API
// 参考：https://github.com/icebear0828/codex-proxy
const codexDefaultBaseURL = "https://chatgpt.com/backend-api"

// CodexStrategy Codex 运营商策略
type CodexStrategy struct{}

func init() {
	OperatorRegistry.Register("codex", &CodexStrategy{})
}

// CodexRequest Codex Responses API 请求格式
type CodexRequest struct {
	Model              string           `json:"model"`
	Instructions       string           `json:"instructions"`
	Input              []CodexInputItem `json:"input"`
	Stream             bool             `json:"stream"`
	Store              bool             `json:"store"`
	Reasoning          *CodexReasoning  `json:"reasoning,omitempty"`
	Tools              []interface{}    `json:"tools,omitempty"`
	PreviousResponseID string           `json:"previous_response_id,omitempty"`
	ToolChoice         string           `json:"tool_choice,omitempty"`
}

// CodexInputItem Codex 输入项
type CodexInputItem struct {
	Role    string `json:"role"` // "user" | "assistant" | "system"
	Content string `json:"content"`
}

// CodexReasoning Codex 推理配置
type CodexReasoning struct {
	Effort string `json:"effort"` // "low" | "medium" | "high"
}

// CodexSSEEvent Codex SSE 事件
type CodexSSEEvent struct {
	Event string          `json:"-"`
	Data  json.RawMessage `json:"-"`
}

// CodexResponseCreated response.created 事件数据
type CodexResponseCreated struct {
	Response struct {
		ID string `json:"id"`
	} `json:"response"`
}

// CodexTextDelta response.output_text.delta 事件数据
type CodexTextDelta struct {
	Delta string `json:"delta"`
}

// CodexCompleted response.completed 事件数据
type CodexCompleted struct {
	Response struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"response"`
}

// Execute 执行 Codex 运营商转发
func (s *CodexStrategy) Execute(ctx context.Context, payload map[string]any, opts ExecuteOptions) (statusCode int, contentType string, body []byte, streamBody io.ReadCloser, err error) {
	logStep("operator codex: start, stream=%v, baseURL=%s", opts.Stream, opts.BaseURL)

	// 转换 Anthropic 请求为 Codex 请求
	codexReq, err := anthropicToCodexRequest(payload, opts.UpstreamModel)
	if err != nil {
		logStep("operator codex: request conversion failed, err=%v", err)
		return 0, "", nil, nil, fmt.Errorf("failed to convert request: %w", err)
	}

	// 设置 stream 标志
	codexReq.Stream = opts.Stream

	// 序列化请求
	reqBody, err := json.Marshal(codexReq)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("failed to marshal codex request: %w", err)
	}

	logStep("operator codex: payload_to_send=%s", string(reqBody))

	// 构建 URL
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = codexDefaultBaseURL
	}
	url := baseURL + "/v1/responses"

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}

	// 发送请求
	client := &http.Client{
		Timeout: 600 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("failed to send request: %w", err)
	}

	statusCode = resp.StatusCode
	contentType = resp.Header.Get("Content-Type")

	logStep("operator codex: upstream response status=%d contentType=%s", statusCode, contentType)

	// 处理错误响应
	if statusCode < 200 || statusCode >= 300 {
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		logStep("operator codex: upstream error status=%d bodyLen=%d", statusCode, len(body))
		return statusCode, contentType, body, nil, fmt.Errorf("upstream error: status=%d", statusCode)
	}

	// 流式响应：转换 Codex SSE 为 Anthropic SSE
	if opts.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			defer resp.Body.Close()
			if err := convertCodexStreamToAnthropic(ctx, resp.Body, pw); err != nil {
				logStep("operator codex: stream conversion error: %v", err)
			}
		}()
		return statusCode, "text/event-stream", nil, pr, nil
	}

	// 非流式响应：收集完整响应并转换
	defer resp.Body.Close()
	anthropicResp, err := collectCodexResponseToAnthropic(resp.Body)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("failed to collect response: %w", err)
	}

	body, err = json.Marshal(anthropicResp)
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("failed to marshal anthropic response: %w", err)
	}

	logStep("operator codex: non-stream response converted, bodyLen=%d", len(body))
	return statusCode, "application/json", body, nil, nil
}

// anthropicToCodexRequest 将 Anthropic Messages API 请求转换为 Codex Responses API 请求
func anthropicToCodexRequest(payload map[string]any, upstreamModel string) (*CodexRequest, error) {
	// 提取 system 消息作为 instructions
	var instructions string
	if systemVal, ok := payload["system"]; ok {
		if systemStr, ok := systemVal.(string); ok {
			instructions = systemStr
		} else if systemArr, ok := systemVal.([]interface{}); ok {
			// system 可能是数组格式 [{type: "text", text: "..."}]
			var systemParts []string
			for _, item := range systemArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if text, ok := itemMap["text"].(string); ok {
						systemParts = append(systemParts, text)
					}
				}
			}
			instructions = strings.Join(systemParts, "\n\n")
		}
	}
	if instructions == "" {
		instructions = "You are a helpful assistant."
	}

	// 转换 messages
	var input []CodexInputItem
	if messagesVal, ok := payload["messages"].([]interface{}); ok {
		for _, msg := range messagesVal {
			msgMap, ok := msg.(map[string]interface{})
			if !ok {
				continue
			}

			role, _ := msgMap["role"].(string)
			if role == "" {
				continue
			}

			// 提取 content
			content := extractContent(msgMap["content"])
			if content == "" && role == "assistant" {
				// assistant 消息可能只有 tool_calls，需要特殊处理
				continue
			}

			// 映射角色
			codexRole := role
			if role == "system" {
				// system 消息已经放到 instructions 中，跳过
				continue
			}

			input = append(input, CodexInputItem{
				Role:    codexRole,
				Content: content,
			})
		}
	}

	// 确保至少有一条输入
	if len(input) == 0 {
		input = append(input, CodexInputItem{
			Role:    "user",
			Content: "",
		})
	}

	// 构建请求
	req := &CodexRequest{
		Model:        upstreamModel,
		Instructions: instructions,
		Input:        input,
		Stream:       true,
		Store:        false,
		Tools:        []interface{}{}, // 默认空数组
	}

	// 处理 tools 定义
	if toolsVal, ok := payload["tools"]; ok {
		if toolsArr, ok := toolsVal.([]interface{}); ok && len(toolsArr) > 0 {
			// 转换 Anthropic tools 格式为 OpenAI/Codex 格式
			// Anthropic: {name, description, input_schema}
			// OpenAI/Codex: {type: "function", function: {name, description, parameters}}
			convertedTools := make([]interface{}, 0, len(toolsArr))
			for _, tool := range toolsArr {
				toolMap, ok := tool.(map[string]interface{})
				if !ok {
					continue
				}

				// 提取 Anthropic 格式的字段
				name, _ := toolMap["name"].(string)
				description, _ := toolMap["description"].(string)
				inputSchema := toolMap["input_schema"]

				// 构建 OpenAI/Codex 格式
				codexTool := map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name":        name,
						"description": description,
						"parameters":  inputSchema, // input_schema → parameters
					},
				}
				convertedTools = append(convertedTools, codexTool)
			}
			req.Tools = convertedTools
			logStep("anthropicToCodexRequest: converted %d Anthropic tools to Codex format", len(convertedTools))
		}
	}

	// 处理 thinking/reasoning
	if thinkingVal, ok := payload["thinking"]; ok {
		if thinkingMap, ok := thinkingVal.(map[string]interface{}); ok {
			if budget, ok := thinkingMap["budget_tokens"].(float64); ok && budget > 0 {
				// 根据 budget 设置 reasoning effort
				effort := "medium"
				if budget > 10000 {
					effort = "high"
				} else if budget < 5000 {
					effort = "low"
				}
				req.Reasoning = &CodexReasoning{Effort: effort}
			}
		}
	}
	req.ToolChoice = "auto"
	logStep("anthropicToCodexRequest_Tools=%v", req.Tools)
	return req, nil
}

// extractContent 从 Anthropic content 中提取文本，并将 tool_use/tool_result 展平为可读文本
func extractContent(contentVal interface{}) string {
	if contentStr, ok := contentVal.(string); ok {
		return contentStr
	}
	if contentArr, ok := contentVal.([]interface{}); ok {
		var parts []string
		for _, item := range contentArr {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, _ := itemMap["type"].(string)
			switch blockType {
			case "text":
				// 文本块
				if text, ok := itemMap["text"].(string); ok {
					parts = append(parts, text)
				}

			case "tool_use":
				// 工具调用块：展平为 [Tool Call: name(args)]
				name, _ := itemMap["name"].(string)
				if name == "" {
					name = "unknown"
				}
				inputStr := "{}"
				if input := itemMap["input"]; input != nil {
					if inputBytes, err := json.Marshal(input); err == nil {
						inputStr = string(inputBytes)
					}
				}
				parts = append(parts, fmt.Sprintf("[Tool Call: %s(%s)]", name, inputStr))

			case "tool_result":
				// 工具结果块：展平为 [Tool Result (id)]: content
				toolUseID, _ := itemMap["tool_use_id"].(string)
				if toolUseID == "" {
					toolUseID = "unknown"
				}

				prefix := "Tool Result"
				if isError, ok := itemMap["is_error"].(bool); ok && isError {
					prefix = "Tool Error"
				}

				// 提取 content
				resultContent := ""
				if contentVal := itemMap["content"]; contentVal != nil {
					if contentStr, ok := contentVal.(string); ok {
						resultContent = contentStr
					} else if contentArr, ok := contentVal.([]interface{}); ok {
						// content 也可能是数组
						var textParts []string
						for _, c := range contentArr {
							if cMap, ok := c.(map[string]interface{}); ok {
								if cMap["type"] == "text" {
									if text, ok := cMap["text"].(string); ok {
										textParts = append(textParts, text)
									}
								}
							}
						}
						resultContent = strings.Join(textParts, "\n")
					}
				}
				parts = append(parts, fmt.Sprintf("[%s (%s)]: %s", prefix, toolUseID, resultContent))
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// convertCodexStreamToAnthropic 将 Codex SSE 流转换为 Anthropic SSE 流
// 参考 codex-proxy: https://github.com/icebear0828/codex-proxy/blob/main/src/translation/codex-to-anthropic.ts
func convertCodexStreamToAnthropic(ctx context.Context, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024) // 10MB max

	// 生成消息 ID
	msgID := fmt.Sprintf("msg_%s", generateRandomID(24))

	// 1. 立即发送 message_start（不等待 response.created）
	messageStart := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         "claude-3-5-sonnet-20241022",
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	if err := writeCodexAnthropicSSE(writer, messageStart); err != nil {
		return err
	}

	// 2. 发送 content_block_start
	contentBlockStart := map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	}
	if err := writeCodexAnthropicSSE(writer, contentBlockStart); err != nil {
		return err
	}

	// 3. 处理 Codex 流事件
	var currentEvent string
	var dataLines []string
	var outputTokens int

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// 空行表示事件结束
		if line == "" {
			if currentEvent != "" && len(dataLines) > 0 {
				dataStr := strings.Join(dataLines, "\n")
				if dataStr != "[DONE]" {
					if err := processCodexStreamEvent(currentEvent, dataStr, writer, &outputTokens); err != nil {
						logStep("operator codex: failed to process event: %v", err)
					}
				}
			}
			currentEvent = ""
			dataLines = nil
			continue
		}

		// 解析 SSE 字段
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	// 4. 发送 content_block_stop
	contentBlockStop := map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	}
	if err := writeCodexAnthropicSSE(writer, contentBlockStop); err != nil {
		return err
	}

	// 5. 发送 message_delta
	messageDelta := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": outputTokens,
		},
	}
	if err := writeCodexAnthropicSSE(writer, messageDelta); err != nil {
		return err
	}

	// 6. 发送 message_stop
	messageStop := map[string]interface{}{
		"type": "message_stop",
	}
	if err := writeCodexAnthropicSSE(writer, messageStop); err != nil {
		return err
	}

	return nil
}

// processCodexStreamEvent 处理单个 Codex 流事件
func processCodexStreamEvent(event string, dataStr string, writer io.Writer, outputTokens *int) error {
	switch event {
	case "response.output_text.delta":
		// 发送 content_block_delta 事件
		var delta CodexTextDelta
		if err := json.Unmarshal([]byte(dataStr), &delta); err != nil {
			return err
		}

		if delta.Delta != "" {
			contentBlockDelta := map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": delta.Delta,
				},
			}
			if err := writeCodexAnthropicSSE(writer, contentBlockDelta); err != nil {
				return err
			}
		}

	case "response.completed":
		// 提取 usage 信息，但不发送事件（在流结束时统一发送）
		var completed CodexCompleted
		if err := json.Unmarshal([]byte(dataStr), &completed); err != nil {
			return err
		}
		*outputTokens = completed.Response.Usage.OutputTokens
	}

	return nil
}

// generateRandomID 生成指定长度的随机 ID
func generateRandomID(length int) string {
	const charset = "0123456789abcdef"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

// writeCodexAnthropicSSE 写入 Anthropic SSE 事件（Codex 专用）
func writeCodexAnthropicSSE(writer io.Writer, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	eventType := "message_start"
	if dataMap, ok := data.(map[string]interface{}); ok {
		if t, ok := dataMap["type"].(string); ok {
			eventType = t
		}
	}

	_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", eventType, jsonData)
	return err
}

// collectCodexResponseToAnthropic 收集 Codex 非流式响应并转换为 Anthropic 格式
func collectCodexResponseToAnthropic(reader io.Reader) (map[string]interface{}, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	var fullText strings.Builder
	var responseID string
	var inputTokens, outputTokens int

	var currentEvent string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if currentEvent != "" && len(dataLines) > 0 {
				dataStr := strings.Join(dataLines, "\n")
				if dataStr != "[DONE]" {
					switch currentEvent {
					case "response.created":
						var created CodexResponseCreated
						if err := json.Unmarshal([]byte(dataStr), &created); err == nil {
							responseID = created.Response.ID
						}

					case "response.output_text.delta":
						var delta CodexTextDelta
						if err := json.Unmarshal([]byte(dataStr), &delta); err == nil {
							fullText.WriteString(delta.Delta)
						}

					case "response.completed":
						var completed CodexCompleted
						if err := json.Unmarshal([]byte(dataStr), &completed); err == nil {
							inputTokens = completed.Response.Usage.InputTokens
							outputTokens = completed.Response.Usage.OutputTokens
						}
					}
				}
			}
			currentEvent = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	// 构建 Anthropic 响应
	response := map[string]interface{}{
		"id":   responseID,
		"type": "message",
		"role": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fullText.String(),
			},
		},
		"model":         "claude-3-5-sonnet-20241022",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	return response, nil
}
