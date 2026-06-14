// OpsEngine 桌面应用入口

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "OpsEngine",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        640,
		DisableResize:    false,
		Fullscreen:       false,
		Frameless:        true,
		WindowStartState: options.Normal,
		BackgroundColour: &options.RGBA{R: 26, G: 26, B: 25, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Windows: &windows.Options{
			DisableFramelessWindowDecorations: false,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
		// 启用拖拽文件功能：image_push_tar 等节点的 file_path 字段需要拿到本机绝对路径
		// 前端把可接收拖拽的元素打上 CSS 属性 --wails-drop-target: drop；
		// 用户拖到该元素上松手时，runtime.OnFileDrop 回调收到绝对路径
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		panic("启动失败: " + err.Error())
	}
}
