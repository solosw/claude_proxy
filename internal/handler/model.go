package handler

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
)

const apiKeyChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// generateAPIKey 生成32位随机API Key（字母+数字）
func generateAPIKey() (string, error) {
	result := make([]byte, 32)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(apiKeyChars))))
		if err != nil {
			return "", err
		}
		result[i] = apiKeyChars[n.Int64()]
	}
	return string(result), nil
}

func generateUniqueAPIKey() (string, error) {
	for i := 0; i < 10; i++ {
		apiKey, err := generateAPIKey()
		if err != nil {
			return "", err
		}
		_, err = model.GetUserByAPIKey(apiKey)
		if err == model.ErrNotFound {
			return apiKey, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("failed to generate unique api key")
}

func RegisterModelRoutes(r gin.IRouter, cfg *appconfig.Config) {
	api := r.Group("/api")
	// 注意：登录接口在 main.go 中单独注册，不需要认证
	api.GET("/me/usage", getMyUsage)
	api.GET("/me/usage/logs", getMyUsageLogs)

	admin := api.Group("")
	admin.Use(middleware.RequireAdmin())

	admin.GET("/models", listModels)
	admin.POST("/models", createModel)
	admin.GET("/models/:id", getModel)
	admin.PUT("/models/:id", updateModel)
	admin.DELETE("/models/:id", deleteModel)

	admin.GET("/operators", listOperators(cfg))
	admin.GET("/operators/:id", getOperator(cfg))

	admin.GET("/combos", listCombos)
	admin.POST("/combos", createCombo)
	admin.GET("/combos/:id", getCombo)
	admin.PUT("/combos/:id", updateCombo)
	admin.DELETE("/combos/:id", deleteCombo)

	admin.GET("/users", listUsers)
	admin.POST("/users", createUser)
	admin.PUT("/users/:username", updateUser)
	admin.GET("/users/:username/usage", getUserUsage)
	admin.DELETE("/users/:username", deleteUser)
}

type loginRequest struct {
	APIKey string `json:"api_key"`
}

type loginResponse struct {
	Success  bool   `json:"success"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	Message  string `json:"message,omitempty"`
}

// login 返回一个登录处理器，需要 cfg 来检查管理员 API Key
func login(cfg *appconfig.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		loginHandler(c, cfg)
	}
}

// LoginWithoutAuth 不需要认证的登录处理器（由 main.go 直接调用）
func LoginWithoutAuth(c *gin.Context, cfg *appconfig.Config) {
	loginHandler(c, cfg)
}

func loginHandler(c *gin.Context, cfg *appconfig.Config) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_key required"})
		return
	}

	// 先检查是否是管理员
	adminAPIKey := strings.TrimSpace(cfg.Auth.APIKey)
	if adminAPIKey != "" && apiKey == adminAPIKey {
		c.JSON(http.StatusOK, loginResponse{
			Success:  true,
			Username: "admin",
			IsAdmin:  true,
		})
		return
	}

	// 检查用户表
	user, err := model.GetUserByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return
	}

	// 检查过期
	now := time.Now()
	if user.ExpireAt != nil && now.After(*user.ExpireAt) {
		c.JSON(http.StatusForbidden, gin.H{"error": "api key expired"})
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		Success:  true,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
	})
}

func getMyUsage(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, usageRespFromUser(u))
}

func getMyUsageLogs(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// 获取分页参数
	page := 1
	pageSize := 10
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	// 查询使用日志
	logs, total, err := model.GetUsageLogsByUsername(u.Username, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": total,
	})
}

func listUsers(c *gin.Context) {
	users, err := model.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 应用筛选条件
	username := strings.TrimSpace(c.Query("username"))
	isAdminStr := strings.TrimSpace(c.Query("is_admin"))

	filtered := make([]*model.User, 0)
	for _, u := range users {
		// 按用户名筛选
		if username != "" && !strings.Contains(strings.ToLower(u.Username), strings.ToLower(username)) {
			continue
		}

		// 按管理员状态筛选
		if isAdminStr != "" {
			isAdmin := isAdminStr == "true" || isAdminStr == "1"
			if u.IsAdmin != isAdmin {
				continue
			}
		}

		filtered = append(filtered, u)
	}

	c.JSON(http.StatusOK, filtered)
}

type userCreateRequest struct {
	Username string     `json:"username"`
	Quota    int64      `json:"quota"`
	ExpireAt *time.Time `json:"expire_at"`
	IsAdmin  bool       `json:"is_admin"`
}

func createUser(c *gin.Context) {
	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}

	for i := 0; i < 10; i++ {
		apiKey, err := generateUniqueAPIKey()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate API key"})
			return
		}
		u := &model.User{
			Username: username,
			APIKey:   apiKey,
			Quota:    req.Quota,
			ExpireAt: req.ExpireAt,
			IsAdmin:  req.IsAdmin,
		}
		if err := model.CreateUser(u); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, u)
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user with unique api key"})
}

type userUpdateRequest struct {
	APIKey   *string    `json:"api_key"`
	Quota    *int64     `json:"quota"`
	ExpireAt *time.Time `json:"expire_at"`
	IsAdmin  *bool      `json:"is_admin"`
}

func updateUser(c *gin.Context) {
	username := c.Param("username")
	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	update := map[string]any{}
	if req.APIKey != nil {
		update["api_key"] = strings.TrimSpace(*req.APIKey)
	}
	if req.Quota != nil {
		update["quota"] = *req.Quota
	}
	if req.IsAdmin != nil {
		update["is_admin"] = *req.IsAdmin
	}
	if req.ExpireAt != nil {
		update["expire_at"] = req.ExpireAt
	}
	if len(update) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty update"})
		return
	}
	if err := model.UpdateUserByUsername(username, update); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	u, _ := model.GetUser(username)
	c.JSON(http.StatusOK, u)
}

func getUserUsage(c *gin.Context) {
	u, err := model.GetUser(c.Param("username"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usageRespFromUser(u))
}

func deleteUser(c *gin.Context) {
	username := c.Param("username")
	if strings.TrimSpace(username) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}

	// 检查用户是否存在
	_, err := model.GetUser(username)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	if err := model.DeleteUser(username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.Status(http.StatusNoContent)
}

type usageResponse struct {
	Username     string     `json:"username"`
	APIKey       string     `json:"api_key"`
	Quota        int64      `json:"quota"`
	Remaining    int64      `json:"remaining"`
	Unlimited    bool       `json:"unlimited"`
	ExpireAt     *time.Time `json:"expire_at"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	TotalTokens  int64      `json:"total_tokens"`
	IsAdmin      bool       `json:"is_admin"`
}

func usageRespFromUser(u *model.User) *usageResponse {
	if u == nil {
		return nil
	}
	resp := &usageResponse{
		Username:     u.Username,
		APIKey:       u.APIKey,
		Quota:        u.Quota,
		ExpireAt:     u.ExpireAt,
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
		IsAdmin:      u.IsAdmin,
	}
	if u.Quota < 0 {
		resp.Unlimited = true
		resp.Remaining = -1
	} else {
		resp.Remaining = u.Quota - u.TotalTokens
		if resp.Remaining < 0 {
			resp.Remaining = 0
		}
	}
	return resp
}

func listModels(c *gin.Context) {
	ms := model.ListModels()
	c.JSON(http.StatusOK, ms)
}

func getModel(c *gin.Context) {
	id := c.Param("id")
	m, err := model.GetModel(id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func createModel(c *gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.CreateModel(&m); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, m)
}

func updateModel(c *gin.Context) {
	id := c.Param("id")
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.UpdateModel(id, &m); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func deleteModel(c *gin.Context) {
	id := c.Param("id")
	if err := model.DeleteModel(id); err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func listOperators(cfg *appconfig.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil || cfg.Operators == nil {
			c.JSON(http.StatusOK, []any{})
			return
		}
		list := make([]*model.Operator, 0, len(cfg.Operators))
		for id, ep := range cfg.Operators {
			list = append(list, &model.Operator{ID: id, Name: strings.TrimSpace(ep.Name), Description: strings.TrimSpace(ep.Description), Enabled: ep.Enabled})
		}
		c.JSON(http.StatusOK, list)
	}
}

func getOperator(cfg *appconfig.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if cfg == nil || cfg.Operators == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		ep, ok := cfg.Operators[id]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "operator not found: " + id})
			return
		}
		c.JSON(http.StatusOK, &model.Operator{ID: id, Name: strings.TrimSpace(ep.Name), Description: strings.TrimSpace(ep.Description), Enabled: ep.Enabled})
	}
}

func listCombos(c *gin.Context) {
	cs := model.ListCombos()
	c.JSON(http.StatusOK, cs)
}

func getCombo(c *gin.Context) {
	id := c.Param("id")
	cb, err := model.GetCombo(id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cb)
}

func createCombo(c *gin.Context) {
	var cb model.Combo
	if err := c.ShouldBindJSON(&cb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.CreateCombo(&cb); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cb)
}

func updateCombo(c *gin.Context) {
	id := c.Param("id")
	var cb model.Combo
	if err := c.ShouldBindJSON(&cb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.UpdateCombo(id, &cb); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cb)
}

func deleteCombo(c *gin.Context) {
	id := c.Param("id")
	if err := model.DeleteCombo(id); err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
