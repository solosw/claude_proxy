package main

import (
	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/handler"
	"awesomeProject/internal/middleware"
	"awesomeProject/internal/model"
	"awesomeProject/internal/oldhandler"
	"awesomeProject/internal/storage"
	"awesomeProject/internal/task"
	"awesomeProject/pkg/utils"
	"github.com/gin-gonic/gin"
	"log"
	"strings"
)

func main() {
	cfg, err := appconfig.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	// 初始化日志级别
	utils.InitLogger(cfg.Log.Level)
	if cfg.Server.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

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

	db, err := storage.Init(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	if err := db.AutoMigrate(&model.Model{}, &model.Combo{}, &model.ComboItem{}, &model.User{}, &model.UsageLog{}, &model.ErrorLog{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	// 启动定时清理任务
	task.StartCleanupTask()

	apiRoot := router.Group("/back")

	// 登录接口不需要认证，直接注册
	apiRoot.POST("/api/login", func(c *gin.Context) {
		handler.LoginWithoutAuth(c, cfg)
	})

	// 需要认证的 API 组
	authenticated := apiRoot.Group("")
	authenticated.Use(middleware.APIKeyAuth(cfg.Auth.APIKey))
	authenticated.Use(middleware.ErrorHandler())

	chatHandler := handler.NewChatHandler(cfg)
	chatHandler.RegisterRoutes(authenticated)

	messagesHandler := handler.NewMessagesHandler(cfg)
	messagesHandler.RegisterRoutes(authenticated)

	// 新增：Codex 直通 /v1/responses
	codexProxyHandler := handler.NewCodexProxyHandler(cfg)
	codexProxyHandler.RegisterRoutes(authenticated)

	chatTestHandler := handler.NewChatTestHandler()
	chatTestHandler.RegisterRoutes(authenticated)
	oldMessagesHandler := oldhandler.NewMessagesHandler(cfg)
	oldMessagesHandler.RegisterRoutes(authenticated)

	handler.RegisterModelRoutes(authenticated, cfg)
	oldhandler.Start()
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
	select {}

}
