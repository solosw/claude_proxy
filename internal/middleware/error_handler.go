package middleware

import (
	"awesomeProject/internal/model"
	"awesomeProject/internal/modelstate"
	"awesomeProject/pkg/utils"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

// responseWriter 包装 gin.ResponseWriter 来捕获状态码
type responseWriter struct {
	gin.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ErrorHandler 中间件：捕获错误状态码并禁用模型
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 包装 ResponseWriter
		rw := &responseWriter{ResponseWriter: c.Writer, statusCode: http.StatusOK}
		c.Writer = rw

		// 继续处理请求
		c.Next()

		// 检查响应状态码
		statusCode := rw.statusCode

		shouldDisable := statusCode >= 400

		if shouldDisable {
			// 从请求中提取模型 ID
			modelID := extractModelIDFromRequest(c)
			if modelID != "" {
				// 禁用模型 15 分钟
				modelstate.DisableModelTemporarily(modelID, modelstate.TemporaryModelDisableTTL)
				utils.Logger.Printf("[ClaudeRouter] error_handler: model_disabled model=%s status=%d", modelID, statusCode)
			}
			conversionID := extractConversationIDFromRequest(c)
			if conversionID != "" {
				modelstate.ClearConversationModel(conversionID)

			}
			var username string
			if u := CurrentUser(c); u != nil {
				username = u.Username
			}
			_ = model.RecordErrorLog(modelID, username, statusCode, fmt.Sprintf("UpStream Error:%v", rw.statusCode))

		}
	}
}

// extractModelIDFromRequest 从请求中提取模型 ID
func extractModelIDFromRequest(c *gin.Context) string {
	// 从 Gin 上下文中获取（如果在处理器中设置过）
	if modelID, exists := c.Get("real_model_id"); exists {
		if id, ok := modelID.(string); ok {
			return strings.TrimSpace(id)
		}
	}

	return ""
}
func extractConversationIDFromRequest(c *gin.Context) string {
	// 从 Gin 上下文中获取（如果在处理器中设置过）
	if modelID, exists := c.Get("real_conversation_id"); exists {
		if id, ok := modelID.(string); ok {
			return strings.TrimSpace(id)
		}
	}

	return ""
}
