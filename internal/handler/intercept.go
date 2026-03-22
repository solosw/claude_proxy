package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"awesomeProject/internal/model"
	"awesomeProject/pkg/utils"
)

var (
	// 中文问"你是/叫什么模型"类问题（不用\b，中文无空格边界）
	modelQueryPatternChinese = regexp.MustCompile(`(?i)(?:你|您)(?::|：)?(?:是|叫|属于|基于|使用|用的|叫啥|是啥)?(?:(?:什么|啥|哪种|哪个)?(?:模型|大模型|AI|人工智能|机器人)?|人|谁|什么(?:人|家伙))`)
	// 英文：what/which model / who are you / your name
	modelQueryPatternEnglish = regexp.MustCompile(`(?i)\b(?:what\s*(?:model|AI|model\s+are\s+you|model\s+is\s+this|model\s+name|are\s+you\s+using)|which\s+model|who\s+are\s+you|what(?:\s+is)?\s+your\s+name|gpt[-\s]?\d+|llama\s*\d*|qwen\s*\d*|mistral\s*\d*)\b`)
	greetPattern = regexp.MustCompile(`(?i)\b(?:你好|您好|嗨|哈喽|hello|hi|hey|yo|在吗|在不在|早上好|早啊|中午好|下午好|晚上好)\b`)
)

func matchesInterceptPattern(text string) (matched bool, isGreet bool) {
	if modelQueryPatternChinese.MatchString(text) || modelQueryPatternEnglish.MatchString(text) {
		return true, false
	}
	if greetPattern.MatchString(text) {
		return true, true
	}
	return false, false
}

func buildInterceptReplyText(comboID string, isGreet bool) string {
	cb, err := model.GetCombo(comboID)
	if err != nil || cb == nil {
		return ""
	}
	desc := cb.Description
	if desc == "" {
		return ""
	}
	if isGreet {
		return desc
	}
	return desc
}

func interceptAnthropicReply(c *gin.Context, comboID string, stream bool, isGreet bool) bool {
	reply := buildInterceptReplyText(comboID, isGreet)
	if reply == "" {
		return false
	}
	utils.Logger.Debugf("[ClaudeRouter] intercept: combo=%s stream=%v isGreet=%v", comboID, stream, isGreet)
	if stream {
		msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		w := c.Writer
		writeSSE := func(eventType string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
			w.Flush()
		}
		writeSSE("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":           msgID,
				"type":         "message",
				"role":         "assistant",
				"content":      []any{},
				"model":        comboID,
				"stop_reason":  nil,
				"stop_sequence": nil,
				"usage":        map[string]any{"input_tokens": 1, "output_tokens": 1},
			},
		})
		writeSSE("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": 0,
			"content_block": map[string]any{
				"type": "text",
				"text": "",
			},
		})
		writeSSE("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": reply,
			},
		})
		writeSSE("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": 0,
		})
		writeSSE("message_delta", map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": map[string]any{"output_tokens": 1},
		})
		writeSSE("message_stop", map[string]any{"type": "message_stop"})
		return true
	}
	c.JSON(http.StatusOK, map[string]any{
		"id":      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		"type":    "message",
		"role":    "assistant",
		"model":   comboID,
		"content": []any{map[string]any{"type": "text", "text": reply}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
	return true
}

func interceptOpenAIChatReply(c *gin.Context, comboID string, stream bool, isGreet bool) bool {
	reply := buildInterceptReplyText(comboID, isGreet)
	if reply == "" {
		return false
	}
	utils.Logger.Debugf("[ClaudeRouter] intercept: combo=%s stream=%v isGreet=%v", comboID, stream, isGreet)
	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		w := c.Writer
		chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
		writeSSE := func(data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", string(b))
			w.Flush()
		}
		writeSSE(map[string]any{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   comboID,
			"choices": []any{
				map[string]any{
					"index": 0,
					"delta": map[string]any{"role": "assistant", "content": reply},
					"finish_reason": nil,
				},
			},
		})
		writeSSE(map[string]any{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   comboID,
			"choices": []any{
				map[string]any{
					"index":        0,
					"delta":        map[string]any{},
					"finish_reason": "stop",
				},
			},
		})
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.Flush()
		return true
	}
	c.JSON(http.StatusOK, map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   comboID,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": reply,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
	return true
}

func interceptOpenAIResponsesReply(c *gin.Context, comboID string, stream bool, isGreet bool) bool {
	reply := buildInterceptReplyText(comboID, isGreet)
	if reply == "" {
		return false
	}
	utils.Logger.Debugf("[ClaudeRouter] intercept: combo=%s stream=%v isGreet=%v", comboID, stream, isGreet)
	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		w := c.Writer
		respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
		writeSSE := func(eventType string, data any) {
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
			w.Flush()
		}
		writeSSE("response.created", map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":     respID,
				"object": "realtime.response",
				"status": "in_progress",
			},
		})
		writeSSE("response.output_text.delta", map[string]any{
			"type":         "response.output_text.delta",
			"output_index": 0,
			"content_index": 0,
			"delta":        reply,
		})
		writeSSE("response.completed", map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     respID,
				"object": "realtime.response",
				"status": "completed",
				"output": []any{
					map[string]any{
						"type": "message",
						"role": "assistant",
						"content": []any{
							map[string]any{"type": "output_text", "text": reply},
						},
					},
				},
			},
		})
		return true
	}
	c.JSON(http.StatusOK, map[string]any{
		"id":     fmt.Sprintf("resp_%d", time.Now().UnixNano()),
		"object": "response",
		"status": "completed",
		"model":  comboID,
		"output": []any{
			map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": reply},
				},
			},
		},
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
	})
	return true
}
