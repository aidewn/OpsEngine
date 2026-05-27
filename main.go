// OpsEngine 桌面应用入口

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "OpsEngine",
		Width:            1280,
		Height:           800,
		MinWidth:         900,
		MinHeight:        600,
		DisableResize:    false,
		Fullscreen:       false,
		WindowStartState: options.Normal,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// 启用拖拽文件功能：image_push_tar 等节点的 file_path 字段需要拿到本机绝对路径
		// 前端把可接收拖拽的元素打上 CSS 属性 --wails-drop-target: drop；
		// 用户拖到该元素上松手时，runtime.OnFileDrop 回调收到绝对路径
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		panic("启动失败: " + err.Error())
	}
}
