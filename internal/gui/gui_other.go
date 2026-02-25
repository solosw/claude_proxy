package gui

import (
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"
)

func StartWindowsGUI(addr string) {
	// 非 Windows 平台：不启动 GUI，改用浏览器模式
	url := "http://" + addr
	log.Printf("GUI not available on this platform, opening in default browser: %s", url)
	StartBrowserMode(addr)
}

func StartBrowserMode(addr string) {
	url := "http://" + addr

	// 在非 Windows 平台使用 systray.Run() 而不是 Register()
	systray.Run(func() {
		onSystrayReadyBrowserMode(url)
	}, func() {
		onSystrayExit()
	})
}

func onSystrayReadyBrowserMode(url string) {
	// 加载托盘图标
	iconData := loadTrayIcon()
	if iconData != nil {
		systray.SetIcon(iconData)
	}

	systray.SetTitle("ClaudeRouter")
	systray.SetTooltip("ClaudeRouter - API Router")

	// 添加菜单项
	mOpen := systray.AddMenuItem("Open", "Open in default browser")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit application")

	// 处理菜单点击
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openInBrowser(url)
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onSystrayExit() {
	log.Println("Systray exiting")
	os.Exit(0)
}

// openInBrowser 在默认浏览器中打开 URL
func openInBrowser(url string) {
	log.Printf("Opening URL in default browser: %s", url)

	var cmd *exec.Cmd

	// 根据操作系统选择打开浏览器的命令
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		log.Printf("Unsupported platform for opening browser: %s", runtime.GOOS)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

// loadTrayIcon 从文件加载托盘图标（ICO 格式）
func loadTrayIcon() []byte {
	iconPath := "./public/web/favicon.ico"
	data, err := os.ReadFile(iconPath)
	if err != nil {
		log.Printf("Failed to load tray icon from %s: %v, using no icon", iconPath, err)
		return nil
	}
	return data
}

func hideWindowHandler() {
	// 非 Windows 平台：无操作
}
