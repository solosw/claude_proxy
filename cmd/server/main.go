package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/handler"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"
)

func main() {
	// 1. 加载配置文件
	cfg, err := appconfig.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 4. 设置 Gin 模式（后续可以从配置中读取）
	gin.SetMode(gin.DebugMode)

	// 5. 创建 Gin 路由器
	router := gin.Default()

	// 404 时打印未匹配的 Method + Path，便于排查（协议转换不会返回 404，404 表示路由未命中）
	router.NoRoute(func(c *gin.Context) {
		log.Printf("[ClaudeRouter] 404 no route: %s %s", c.Request.Method, c.Request.URL.Path)
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "path": c.Request.URL.Path, "method": c.Request.Method})
	})

	// 初始化数据库
	db, err := storage.Init(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}

	// 自动迁移模型相关表结构（包括 ComboItem 子表）
	if err := db.AutoMigrate(&model.Model{}, &model.Combo{}, &model.ComboItem{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 所有对外 API 统一挂载在 /back 前缀下，方便前端通过 Vite 代理。
	apiRoot := router.Group("/back")

	// 使用 API Key 认证中间件
	apiRoot.Use(middleware.APIKeyAuth(cfg.Auth.APIKey))

	// 注册 OpenAI 兼容的聊天端点（现在直接从数据库读取模型配置）
	chatHandler := handler.NewChatHandler()
	chatHandler.RegisterRoutes(apiRoot)

	// 注册 Anthropic 兼容的 messages 端点（Claude Code 会调用 /v1/messages）
	messagesHandler := handler.NewMessagesHandler(cfg)
	messagesHandler.RegisterRoutes(apiRoot) // POST /back/v1/messages、/back/v1/messages/count_tokens

	// 无 /back 前缀的 v1 路由，便于代理把 /back 剥掉后转发到后端时仍能命中（POST /v1/messages 等）
	v1 := router.Group("/v1")
	v1.Use(middleware.APIKeyAuth(cfg.Auth.APIKey))
	messagesHandler.RegisterRoutesV1(v1) // POST /v1/messages、/v1/messages/count_tokens

	// 前端模型聊天测试端点（支持 SSE）
	chatTestHandler := handler.NewChatTestHandler()
	chatTestHandler.RegisterRoutes(apiRoot)

	// 注册模型、组合模型、运营商列表接口
	handler.RegisterModelRoutes(apiRoot, cfg)

	// 健康检查端点，方便后续做探针、反向代理检查
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// 启动服务器
	if err := router.Run(cfg.Server.Addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
