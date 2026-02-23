package main

import (
	"log"
	"time"

	"awesomeProject/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/zserge/lorca"
)

func main() {
	// 1. 加载配置文件
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 2. 启动嵌入式 HTTP 服务器用于提供前端静态文件
	// 端口从配置中读取，避免冲突
	webPort := cfg.Server.Addr

	// 使用公共目录作为静态文件服务
	go func() {
		gin.SetMode(gin.ReleaseMode)
		router := gin.Default()

		// 静态文件路由
		router.Static("/assets", "./public/web/assets")
		router.Static("/css", "./public/web/css")
		router.Static("/js", "./public/web/js")

		// Vue History 路由支持
		router.NoRoute(func(c *gin.Context) {
			c.File("./public/web/index.html")
		})

		log.Printf("GUI static server starting on %s", webPort)
		if err := router.Run(webPort); err != nil {
			log.Fatalf("failed to start static server: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(500 * time.Millisecond)

	// 3. 创建 webview 窗口
	url := "http://localhost:9090"
	log.Printf("Opening webview with URL: %s", url)

	// 创建 lorca webview 窗口
	// 签名: New(url, dir string, width, height int, customArgs ...string)
	// dir 是 Chrome 用户数据目录，空字符串使用默认目录
	// 注意：Windows 下需要 WebView2 运行时 (Edge Chromium)
	ui, err := lorca.New(url, "", 1200, 800)
	if err != nil {
		log.Fatal("Failed to create webview:", err)
	}
	defer ui.Close()

	// 等待窗口关闭
	<-ui.Done()
}
