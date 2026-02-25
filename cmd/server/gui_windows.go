//go:build windows
// +build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
)

// Windows API 常量
const (
	SW_HIDE      = 0
	SW_SHOW      = 5
	SW_MINIMIZE  = 6
	SW_RESTORE   = 9
	WM_CLOSE     = 0x0010
	WM_NCDESTROY = 0x0082
	GWL_WNDPROC  = -4
)

var (
	user32DLL            = syscall.NewLazyDLL("user32.dll")
	procShowWindow       = user32DLL.NewProc("ShowWindow")
	procSetForeground    = user32DLL.NewProc("SetForegroundWindow")
	procSetWindowLongPtr = user32DLL.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtr = user32DLL.NewProc("GetWindowLongPtrW")
	procCallWindowProc   = user32DLL.NewProc("CallWindowProcW")
	procDefWindowProc    = user32DLL.NewProc("DefWindowProcW")

	webview webview2.WebView
	mu      sync.Mutex
	visible = false
	hwnd    uintptr
	guiMode = true // true: GUI 模式，false: 浏览器模式

	// 保存原始的窗口过程
	originalWndProc uintptr
)

func startWindowsGUI(addr string) {
	url := "http://" + addr
	guiMode = true

	// 使用 systray.Register 而不是 systray.Run，以便与 webview 共存
	systray.Register(func() {
		onSystrayReady(url)
	}, func() {
		onSystrayExit()
	})

	// 创建并运行 webview
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "ClaudeRouter",
			Width:  1200,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		log.Fatalln("Failed to load webview2, please ensure WebView2 runtime is installed.")
	}
	defer w.Destroy()

	mu.Lock()
	webview = w
	mu.Unlock()

	w.SetSize(1200, 800, webview2.HintNone)
	log.Printf("opening webview: %s", url)
	w.Navigate(url)

	// 等待窗口创建完成后获取句柄
	time.Sleep(500 * time.Millisecond)
	hwnd = getWebViewHandle()
	if hwnd != 0 {
		log.Printf("WebView window handle: 0x%x", hwnd)
		// 初始隐藏窗口
		showWindow(hwnd, SW_HIDE)
		visible = false

		// 设置窗口过程来拦截关闭事件
		hookWindowClose(hwnd)
	}

	w.Run()
}

// hookWindowClose 通过 SetWindowLongPtr 拦截窗口关闭事件
func hookWindowClose(hwnd uintptr) {
	// 获取原始的窗口过程
	//ret, _, _ := procGetWindowLongPtr.Call(hwnd, GWL_WNDPROC)
	//originalWndProc = ret
	//
	//// 设置新的窗口过程（这里使用 Go 的回调函数）
	//// 注意：这需要使用 cgo 来实现，因为 Go 函数不能直接作为 Windows 回调
	//// 作为替代方案，我们使用 JavaScript 来拦截关闭事件
	//injectCloseButtonHandler(webview)
}

// injectCloseButtonHandler 注入 JavaScript 来拦截关闭按钮
func injectCloseButtonHandler(w webview2.WebView) {
	js := `
	(function() {
		// 监听窗口关闭事件
		window.addEventListener('beforeunload', function(e) {
			// 阻止默认关闭行为
			e.preventDefault();
			e.returnValue = '';

			// 通知后端隐藏窗口
			fetch('/back/api/hide-window', {
				method: 'POST',
				headers: {'Content-Type': 'application/json'}
			}).catch(err => console.log('Hide window request failed:', err));

			return false;
		});

		// 监听页面卸载
		window.addEventListener('unload', function(e) {
			e.preventDefault();
			return false;
		});

		// 拦截 Alt+F4 和其他关闭快捷键
		document.addEventListener('keydown', function(e) {
			if ((e.altKey && e.key === 'F4') || (e.ctrlKey && e.key === 'w')) {
				e.preventDefault();
				fetch('/back/api/hide-window', {
					method: 'POST',
					headers: {'Content-Type': 'application/json'}
				}).catch(err => console.log('Hide window request failed:', err));
				return false;
			}
		});
	})();
	`
	w.Eval(js)
}

// getWebViewHandle 通过窗口类名获取 webview 窗口句柄
func getWebViewHandle() uintptr {
	user32 := syscall.NewLazyDLL("user32.dll")
	procFindWindow := user32.NewProc("FindWindowW")

	// WebView2 使用的窗口类名
	className, _ := syscall.UTF16PtrFromString("Chrome_WidgetWin_1")
	ret, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
	return ret
}

func showWindow(hwnd uintptr, show int32) {
	procShowWindow.Call(hwnd, uintptr(show))
}

func setForegroundWindow(hwnd uintptr) {
	procSetForeground.Call(hwnd)
}

func onSystrayReady(url string) {
	// 加载托盘图标
	iconData := loadTrayIcon()
	if iconData != nil {
		systray.SetIcon(iconData)
	}

	systray.SetTitle("ClaudeRouter")
	systray.SetTooltip("ClaudeRouter - API Router")

	// 添加菜单项
	mShow := systray.AddMenuItem("Show", "Show main window")
	mHide := systray.AddMenuItem("Hide", "Hide main window")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit application")

	// 初始状态：隐藏窗口，所以显示 "Show" 菜单
	mHide.Hide()

	// 处理菜单点击
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				toggleWindowVisibility(true)
				mShow.Hide()
				mHide.Show()
			case <-mHide.ClickedCh:
				toggleWindowVisibility(false)
				mHide.Hide()
				mShow.Show()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
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

// openInBrowser 在默认浏览器中打开 URL
func openInBrowser(url string) {
	log.Printf("Opening URL in default browser: %s", url)
	cmd := exec.Command("cmd", "/c", "start", url)
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

// loadTrayIcon 从文件加载托盘图标（ICO 格式）
func loadTrayIcon() []byte {
	iconPath := "./public/web/logo.ico"
	data, err := os.ReadFile(iconPath)
	if err != nil {
		log.Printf("Failed to load tray icon from %s: %v, using no icon", iconPath, err)
		return nil
	}
	return data
}

func onSystrayExit() {
	log.Println("Systray exiting")
}

func toggleWindowVisibility(show bool) {
	mu.Lock()
	defer mu.Unlock()

	if hwnd == 0 {
		log.Println("Window handle not available")
		return
	}

	if show {
		log.Println("Showing window")
		showWindow(hwnd, SW_RESTORE)
		setForegroundWindow(hwnd)
		visible = true
	} else {
		log.Println("Hiding window")
		showWindow(hwnd, SW_HIDE)
		visible = false
	}
}

func hideWindowHandler() {
	if guiMode {
		toggleWindowVisibility(false)
	}
}
