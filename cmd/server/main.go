package main

import (
	"log"
	"strings"
	"time"

	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/handler"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/jchv/go-webview2"
)

func main() {
	cfg, err := appconfig.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	gin.SetMode(gin.DebugMode)
	router := gin.Default()

	webGroup := router.Group("/")
	webGroup.Use(func(c *gin.Context) {
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

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/back") {
			c.JSON(404, gin.H{"error": "API not found"})
			return
		}
		if strings.Contains(path, ".") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.File("./public/web" + path)
			return
		}
		c.File("./public/web/index.html")
	})

	db, err := storage.Init(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	if err := db.AutoMigrate(&model.Model{}, &model.Combo{}, &model.ComboItem{}, &model.User{}, &model.UsageLog{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	apiRoot := router.Group("/back")

	// 登录接口不需要认证，直接注册
	apiRoot.POST("/api/login", func(c *gin.Context) {
		handler.LoginWithoutAuth(c, cfg)
	})

	// 需要认证的 API 组
	authenticated := apiRoot.Group("")
	authenticated.Use(middleware.APIKeyAuth(cfg.Auth.APIKey))
	authenticated.Use(middleware.ErrorHandler())

	chatHandler := handler.NewChatHandler()
	chatHandler.RegisterRoutes(authenticated)

	messagesHandler := handler.NewMessagesHandler(cfg)
	messagesHandler.RegisterRoutes(authenticated)

	// 新增：Codex 直通 /v1/responses
	codexProxyHandler := handler.NewCodexProxyHandler(cfg)
	codexProxyHandler.RegisterRoutes(authenticated)

	chatTestHandler := handler.NewChatTestHandler()
	chatTestHandler.RegisterRoutes(authenticated)

	handler.RegisterModelRoutes(authenticated, cfg)

	// 不需要认证的路由
	apiRoot.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	go func() {
		log.Printf("server starting on %s", cfg.Server.Addr)
		if err := router.Run(cfg.Server.Addr); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

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
