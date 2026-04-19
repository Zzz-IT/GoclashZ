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
		
		// ⚠️ 修改：填入亮色模式的十六进制底色 (#F4F4F5 -> R:244, G:244, B:245)
		BackgroundColour: &options.RGBA{R: 244, G: 244, B: 245, A: 255}, 
		
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
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
