package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyAuth 使用配置中的 API Key 做简单认证。
// 支持以下几种方式携带：
// - Authorization: Bearer <API_KEY>
// - X-API-Key: <API_KEY>
// - token: <API_KEY>（兼容现有前端拦截器）
func APIKeyAuth(apiKey string) gin.HandlerFunc {
	// 如果未配置 apiKey，则认为关闭认证（仅用于开发环境）。
	if strings.TrimSpace(apiKey) == "" {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		var provided string

		// 1. Authorization: Bearer xxx
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			provided = strings.TrimSpace(authHeader[7:])
		}

		// 2. X-API-Key
		if provided == "" {
			provided = c.GetHeader("X-API-Key")
		}

		// 3. token（兼容前端 axios 拦截器）
		if provided == "" {
			provided = c.GetHeader("token")
		}

		if provided == "" || provided != apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"success": false,
				"message": "unauthorized",
			})
			return
		}

		c.Next()
	}
}

