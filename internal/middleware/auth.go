package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"
)

const currentUserKey = "current_user"

// APIKeyAuth 使用配置中的管理员 API Key + 用户 API Key 做认证。
// 支持以下几种方式携带：
// - Authorization: Bearer <API_KEY>
// - X-API-Key: <API_KEY>
// - token: <API_KEY>（兼容现有前端拦截器）
func APIKeyAuth(apiKey string) gin.HandlerFunc {
	adminAPIKey := strings.TrimSpace(apiKey)

	// 如果未配置管理员 apiKey，仍可通过用户表鉴权；若用户表也为空，相当于关闭认证（仅用于开发环境）。
	return func(c *gin.Context) {
		provided := extractAPIKey(c)
		if provided == "" {
			if adminAPIKey == "" {
				c.Next()
				return
			}
			unauthorized(c)
			return
		}

		// 兼容现有配置：config.auth.api_key 始终视为管理员。
		if adminAPIKey != "" && provided == adminAPIKey {
			c.Set(currentUserKey, &model.User{
				Username: "admin",
				APIKey:   adminAPIKey,
				Quota:    -1,
				IsAdmin:  true,
			})
			c.Next()
			return
		}

		if storage.DB == nil {
			unauthorized(c)
			return
		}

		user, err := model.GetUserByAPIKey(provided)
		if err != nil {
			unauthorized(c)

			return
		}

		now := time.Now()
		if user.ExpireAt != nil && now.After(*user.ExpireAt) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"success": false,
				"message": "api key expired",
			})
			return
		}

		if user.Quota <= 0.1 && user.Quota >= 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"success": false,
				"message": "quota exceeded",
			})
			return
		}

		c.Set(currentUserKey, user)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil || !u.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"success": false,
				"message": "admin required",
			})
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) *model.User {
	if c == nil {
		return nil
	}
	v, ok := c.Get(currentUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*model.User)
	return u
}

func extractAPIKey(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		if v := strings.TrimSpace(authHeader[7:]); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(c.GetHeader("X-API-Key")); v != "" {
		return v
	}
	if v := strings.TrimSpace(c.GetHeader("token")); v != "" {
		return v
	}
	return ""
}

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":    http.StatusUnauthorized,
		"success": false,
		"message": "unauthorized",
	})
}
