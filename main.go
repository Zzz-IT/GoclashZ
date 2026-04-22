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
	"unsafe"
)

//go:embed all:frontend/dist
var assets embed.FS

// 引入 User32.dll 用于唤醒窗口
var (
	user32                  = syswin.NewLazySystemDLL("user32.dll")
	procFindWindow          = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow") // 👈 新增：用于获取当前系统的焦点窗口
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
	FLASHW_ALL       = 0x00000003 // 同时闪烁标题栏和任务栏按钮
	FLASHW_TIMERNOFG = 0x0000000C // 持续闪烁直到窗口被移到前台
)

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
				fmt.Println("⚠️ GoclashZ 已经在后台运行！")
				
				// 🎯 模仿 Stelliberty 的 UX：试图唤醒已经隐藏的窗口
				// 这里的 "GoclashZ" 必须与 wails.Run 里的 Title 完全一致
				windowName, _ := syswin.UTF16PtrFromString("GoclashZ")
				hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(windowName)))
				
				if hwnd != 0 {
					// 1. 检查已有窗口的状态
					isVisible, _, _ := procIsWindowVisible.Call(hwnd)
					isMinimized, _, _ := procIsIconic.Call(hwnd)
					fgHwnd, _, _ := procGetForegroundWindow.Call()

					// 2. 如果窗口不可见（在托盘）或已最小化
					if isVisible == 0 || isMinimized != 0 {
						fmt.Println("👉 已有窗口在后台/最小化，正在恢复显示并执行固定闪烁...")
						procShowWindow.Call(hwnd, 9) // SW_RESTORE (9): 恢复并显示
						
						// 🚀 核心修复：针对恢复场景，使用极速单次闪烁 (1次)
						// 配合短超时，实现瞬间“晃一下”的效果，不拖泥带水
						finfo := FLASHWINFO{
							CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
							Hwnd:      syswin.Handle(hwnd),
							DwFlags:   FLASHW_ALL, 
							UCount:    1,
							DwTimeout: 75,
						}
						procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))
					} else {
						// 3. 如果窗口已经在桌面上显示，但不是焦点，则执行持续闪烁提醒
						if hwnd != fgHwnd {
							fmt.Println("👉 窗口已在桌面，执行持续闪烁提醒...")
							finfo := FLASHWINFO{
								CbSize:    uint32(unsafe.Sizeof(FLASHWINFO{})),
								Hwnd:      syswin.Handle(hwnd),
								DwFlags:   FLASHW_ALL | FLASHW_TIMERNOFG, // 持续闪烁直到点击
								UCount:    0,
								DwTimeout: 0,
							}
							procFlashWindowEx.Call(uintptr(unsafe.Pointer(&finfo)))
						}
					}
				}
				
				// 唤醒后立即退出当前这个多余的进程
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

	// 1. 👈 动态读取上一次保存的主题
	var r, g, b uint8 = 242, 242, 242 // 默认日间底色 (#F2F2F2)
	
	// ✅ 使用统一的智能数据目录读取主题
	themeFile := filepath.Join(utils.GetDataDir(), "theme_setting.txt")
	content, err := os.ReadFile(themeFile)
	if err == nil && string(content) == "dark" {
		r, g, b = 17, 17, 17 // 匹配夜间模式底色 (#111111)
	}

	err = wails.Run(&options.App{
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
