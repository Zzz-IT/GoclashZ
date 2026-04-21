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
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 🎯 核心修复：Windows 单例锁 (Single Instance Lock)
	// 防止用户多次双击打开多个后台，导致内核端口被抢占和托盘堆积
	mutexName, _ := syswin.UTF16PtrFromString("Global\\GoclashZ_Single_Instance_Mutex")
	mutexHandle, err := syswin.CreateMutex(nil, false, mutexName)
	if err != nil {
		fmt.Println("创建互斥锁失败:", err)
	}
	// 如果错误码是 ERROR_ALREADY_EXISTS，说明程序已经在后台运行
	if syswin.GetLastError() == syswin.ERROR_ALREADY_EXISTS {
		fmt.Println("GoclashZ 已经在运行中，退出重复进程！")
		os.Exit(0) 
	}
	// 确保程序正常退出时释放锁
	if mutexHandle != 0 {
		defer syswin.CloseHandle(mutexHandle)
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
