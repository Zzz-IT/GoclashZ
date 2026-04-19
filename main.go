package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "GoclashZ",
		Width:  1024,
		Height: 768,
		Frameless: true, // 保持无边框，自己渲染 UI
		
		// 实色底色，与 CSS --glass-bg (#F2F2F2) 保持一致
		BackgroundColour: &options.RGBA{R: 242, G: 242, B: 242, A: 255}, 
		
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown, // 👈 关键修改：注册退出时的回调函数
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			// ⚠️ 修改：彻底移除透明、半透明和 Mica 材质请求，回归纯粹实色渲染
			Theme:             windows.SystemDefault, 
			DisableWindowIcon: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
