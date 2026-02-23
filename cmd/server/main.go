package main

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/handler"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"

	"github.com/jchv/go-webview2"
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
	webGroup := router.Group("/")
	webGroup.Use(func(c *gin.Context) {
		// 只对web静态资源禁用缓存
		if strings.HasPrefix(c.Request.URL.Path, "/assets") ||
			strings.HasPrefix(c.Request.URL.Path, "/css") ||
			strings.HasPrefix(c.Request.URL.Path, "/js") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	})
	webGroup.Static("/assets", "./public/web/assets")
	webGroup.Static("/css", "./public/web/css")
	webGroup.Static("/js", "./public/web/js")

	// Vue History 路由支持 - 所有未匹配的路由都返回 index.html
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// 如果是 API 请求（/back 开头），返回 404
		if strings.HasPrefix(path, "/back") {
			c.JSON(404, gin.H{"error": "API not found"})
			return
		}

		// 检查是否是静态文件（包含文件扩展名）
		if strings.Contains(path, ".") {
			// 设置禁用缓存的响应头
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")

			// 尝试访问 web 目录下的文件
			c.File("./public/web" + path)
			return
		}

		// 其他路由返回 Vue 应用
		c.File("./public/web/index.html")
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

	// 在 goroutine 中启动服务器
	go func() {
		log.Printf("server starting on %s", cfg.Server.Addr)
		if err := router.Run(cfg.Server.Addr); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(500 * time.Millisecond)

	// 启动 WebView2 窗口
	url := "http://" + cfg.Server.Addr
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,

		WindowOptions: webview2.WindowOptions{
			Title:  "Claude",
			Width:  1200,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		log.Fatalln("Failed to load webview2, please ensure WebView2 runtime is installed.")
	}
	defer w.Destroy()
	w.SetSize(1200, 800, webview2.HintNone)
	log.Printf("opening webview: %s", url)
	w.Navigate(url)
	w.Run()
}
