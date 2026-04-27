package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"goclashz/core/utils"
	syswin "golang.org/x/sys/windows" // 👈 引入 windows 底层包并起别名，避免与 Wails 的 windows 冲突
	"strings"
	"time"
	"unsafe"
)

//go:embed all:frontend/dist
var assets embed.FS

// 引入 User32.dll 用于唤醒窗口
var (
	user32                  = syswin.NewLazySystemDLL("user32.dll")
	procFindWindow          = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")    // 🚀 新增
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow") // 🚀 新增
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procFlashWindowEx       = user32.NewProc("FlashWindowEx")
)

// FLASHWINFO 结构体，用于配置闪烁行为
type FLASHWINFO struct {
	CbSize    uint32
	Hwnd      syswin.Handle
	DwFlags   uint32
	UCount    uint32
	DwTimeout uint32
}

const (
	FLASHW_STOP      = 0x00000000 // 🚀 新增：用于强制终止闪烁状态
	FLASHW_CAPTION   = 0x00000001 // 闪烁标题栏
	FLASHW_TRAY      = 0x00000002 // 闪烁任务栏按钮
	FLASHW_ALL       = 0x00000003 // 同时闪烁
	FLASHW_TIMERNOFG = 0x0000000C // 虽保留定义，但不再使用它组合以避免“红而不退”

	SW_RESTORE = 9 // 🚀 新增：用于恢复最小化的窗口
)

// 🛡️ 辅助函数：根据窗口标题查找主窗口句柄
func findMainWindow() uintptr {
	windowName, _ := syswin.UTF16PtrFromString("GoclashZ")
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(windowName)))
	return hwnd
}

// 🛡️ 辅助函数：强行重置任务栏闪烁状态，彻底消除红点顽疾
func stopTaskbarFlash(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	finfo := FLASHWINFO{
		CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
		Hwnd:      syswin.Handle(hwnd),
		DwFlags:   FLASHW_STOP,
		UCount:    0,
		DwTimeout: 0,
	}
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))
}

// 🛡️ 辅助函数：执行标准化的“快速”闪烁并自动清理
func flashWindowTwice(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	// 1. 先清一次旧状态，避免上一次闪烁残留干扰
	stopTaskbarFlash(hwnd)

	// 2. 节奏加快 (150ms)，次数回归 2 次，且仅针对 TRAY 闪烁以实现“深红到没有”的清爽感
	finfo := FLASHWINFO{
		CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
		Hwnd:      syswin.Handle(hwnd),
		DwFlags:   FLASHW_TRAY, // 改为仅任务栏，视觉上更干净
		UCount:    2,
		DwTimeout: 150, // 节奏加快
	}
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))

	// 3. 异步延时等待闪烁动作完成，然后强制清空状态位
	go func() {
		time.Sleep(800 * time.Millisecond)
		stopTaskbarFlash(hwnd)
	}()
}

// 🛡️ 辅助函数：将窗口暴力拉到最前台并获取焦点
func focusWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	// 组合拳：恢复窗口 -> 提到顶层 -> 设为前台焦点
	procShowWindow.Call(hwnd, SW_RESTORE)
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
}

// 🛡️ 终极入口：唤醒主窗口并执行标准的双次闪烁 (供 main.go 和 app.go 共享)
func focusMainWindowAndFlashTwice() {
	hwnd := findMainWindow()
	if hwnd != 0 {
		focusWindow(hwnd)
		flashWindowTwice(hwnd)
	}
}

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
				fmt.Println("⚠️ GoclashZ 已经在后台运行，正在唤醒窗口并闪烁两次...")
				
				// 🚀 核心重构：调用统一的唤醒与闪烁函数
				focusMainWindowAndFlashTwice()
				
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
