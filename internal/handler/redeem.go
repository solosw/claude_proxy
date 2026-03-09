package handler

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
)

const redeemCodeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateRedeemCode 生成随机兑换码（12位大写字母+数字）
func generateRedeemCode() (string, error) {
	result := make([]byte, 12)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(redeemCodeChars))))
		if err != nil {
			return "", err
		}
		result[i] = redeemCodeChars[n.Int64()]
	}
	// 格式化为 XXXX-XXXX-XXXX
	code := string(result)
	return code[:4] + "-" + code[4:8] + "-" + code[8:], nil
}

// RegisterRedeemRoutes 注册兑换码相关路由
func RegisterRedeemRoutes(r gin.IRouter) {
	// 用户接口（需要认证）
	r.POST("/api/redeem", redeemQuota)

	// 管理员接口（需要管理员权限）
	admin := r.Group("/api/redeem-codes")
	admin.Use(middleware.RequireAdmin())
	admin.GET("", listRedeemCodes)
	admin.POST("", createRedeemCode)
	admin.DELETE("/:id", deleteRedeemCode)
}

// createRedeemCode 创建兑换码（管理员）
func createRedeemCode(c *gin.Context) {
	var req struct {
		Code        string     `json:"code"`         // 可选，不填则自动生成
		Quota       float64    `json:"quota"`        // 必填
		MaxUses     int        `json:"max_uses"`     // 必填，最大使用次数
		ExpireAt    *time.Time `json:"expire_at"`    // 可选，过期时间
		Description string     `json:"description"`  // 可选，描述
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid request body",
		})
		return
	}

	// 验证必填字段
	if req.Quota <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "quota must be > 0",
		})
		return
	}

	if req.MaxUses < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "max_uses must be >= 1",
		})
		return
	}

	// 生成或验证兑换码
	code := strings.TrimSpace(req.Code)
	if code == "" {
		var err error
		code, err = generateRedeemCode()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to generate redeem code",
			})
			return
		}
	}

	// 获取当前管理员用户名
	user := middleware.CurrentUser(c)
	createdBy := "admin"
	if user != nil {
		createdBy = user.Username
	}

	// 创建兑换码
	redeemCode := &model.RedeemCode{
		Code:        code,
		Quota:       req.Quota,
		MaxUses:     req.MaxUses,
		ExpireAt:    req.ExpireAt,
		Description: req.Description,
		CreatedBy:   createdBy,
	}

	if err := model.CreateRedeemCode(redeemCode); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "redeem code already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to create redeem code: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "redeem code created successfully",
		"data":    redeemCode,
	})
}

// listRedeemCodes 查询兑换码列表（管理员）
func listRedeemCodes(c *gin.Context) {
	code := c.Query("code")
	createdBy := c.Query("created_by")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	codes, total, err := model.ListRedeemCodesWithPage(code, createdBy, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to list redeem codes: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"codes":     codes,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// deleteRedeemCode 删除兑换码（管理员）
func deleteRedeemCode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid id",
		})
		return
	}

	if err := model.DeleteRedeemCode(id); err != nil {
		if err == model.ErrRedeemCodeNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "redeem code not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to delete redeem code: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "redeem code deleted successfully",
	})
}

// redeemQuota 用户兑换额度
func redeemQuota(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid request body",
		})
		return
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "code required",
		})
		return
	}

	// 获取当前用户
	user := middleware.CurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "unauthorized",
		})
		return
	}

	// 执行兑换
	err := model.RedeemQuota(code, user.Username)
	if err != nil {
		switch err {
		case model.ErrRedeemCodeNotFound:
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "兑换码不存在",
			})
		case model.ErrRedeemCodeExpired:
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "兑换码已过期",
			})
		case model.ErrRedeemCodeExhausted:
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "兑换码已用完",
			})
		case model.ErrRedeemCodeAlreadyUsed:
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "您已经使用过该兑换码",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "兑换失败: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "兑换成功",
	})
}
