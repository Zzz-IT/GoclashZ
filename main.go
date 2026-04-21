package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"goclashz/core/utils"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
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
