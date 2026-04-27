package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"goclashz/core/sys"
	"goclashz/core/utils"
	syswin "golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 1. 判断是否为 Wails 开发模式 (模仿 Stelliberty 放行 Debug)
	// 在 Wails Dev 模式下，通常可执行文件路径包含临时目录或 wails-dev
	exePath, _ := os.Executable()
	isDebugMode := false
	if filepath.Base(exePath) == "GoclashZ-dev.exe" || len(os.Getenv("WAILS_DEV_SERVER")) > 0 {
		isDebugMode = true
		fmt.Println("👉 Wails 开发模式，跳过单实例检查")
	}

	// 2. 单实例锁逻辑
	if !isDebugMode {
		mutexName, _ := syswin.UTF16PtrFromString("Global\\GoclashZ_Single_Instance_Mutex")
		mutexHandle, err := syswin.CreateMutex(nil, false, mutexName)
		
		// ✅ 核心修复：直接通过系统调用返回的 err 判断，切勿使用 GetLastError()
		if err != nil {
			if err == syswin.ERROR_ALREADY_EXISTS {
				fmt.Println("⚠️ GoclashZ 已经在后台运行，正在唤醒已有窗口...")
				
				// 🚀 核心重构：调用统一的唤醒与闪烁函数 (由 core/sys 提供)
				sys.FocusMainWindowAndFlashTwiceWin32Only()
				
				// 显式释放内核互斥锁句柄
				if mutexHandle != 0 {
					syswin.CloseHandle(mutexHandle)
				}
				os.Exit(0)
			} else {
				fmt.Printf("创建互斥锁发生异常: %v\n", err)
			}
		}

		// 确保当前程序真的退出时（而不是假死）再释放锁
		if mutexHandle != 0 {
			defer syswin.CloseHandle(mutexHandle)
		}
	}

	app := NewApp()

	// 👇 修复：将默认兜底颜色改为夜间模式，对齐 app.go 的默认行为
	var r, g, b uint8 = 17, 17, 17 // 默认夜间底色 (#111111)
	
	// ✅ 使用统一的智能数据目录读取主题
	themeFile := filepath.Join(utils.GetDataDir(), "theme_setting.txt")
	// 🎯 修复：使用匿名函数建立独立的局部作用域，让 defer 立即执行
	func() {
		if f, err := os.Open(themeFile); err == nil {
			defer f.Close() // 现在它会在大括号结束时立刻执行，而不是等待 main() 结束

			buf := make([]byte, 16)
			n, _ := f.Read(buf)

			if n > 0 && strings.TrimSpace(string(buf[:n])) == "light" {
				r, g, b = 242, 242, 242
			}
		}
	}()

	err := wails.Run(&options.App{
		Title:  "GoclashZ",
		Width:  1024,
		Height: 768,
		MinWidth:  900, // 👈 核心修复：限制最小宽度，防止 UI 布局挤压
		MinHeight: 600, // 👈 核心修复：限制最小高度
		Frameless: true, // 保持无边框，自己渲染 UI
		
		HideWindowOnClose: true, // 👈 1. 新增：点击关闭按钮时，隐藏窗口而不是退出进程
		StartHidden:       true, // 👈 核心：启动时默认不弹窗，为“静默启动”铺垫
		
		// 2. 👈 使用动态读取的颜色
		BackgroundColour: &options.RGBA{R: r, G: g, B: b, A: 255}, 
		
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:             windows.SystemDefault, 
			DisableWindowIcon: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
