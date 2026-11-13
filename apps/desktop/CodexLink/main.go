package main

import (
	"embed"
	"log"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	var window *application.WebviewWindow
	var forceQuit atomic.Bool
	service := &CodexLinkService{}
	app := application.New(application.Options{
		Name:        "CodexLink",
		Description: "远程控制电脑上的 Codex 与 magpie",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.codexlink.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Restore()
					window.Focus()
				}
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "CodexLink",
		Width:     1280,
		Height:    820,
		MinWidth:  1040,
		MinHeight: 680,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	systemTray := app.SystemTray.New()
	systemTray.SetIcon(trayIcon)
	systemTray.SetTooltip("CodexLink · 点击打开控制台")
	systemTray.OnClick(func() {
		window.Show()
		window.Restore()
		window.Focus()
	})

	trayMenu := app.NewMenu()
	trayMenu.Add("打开 CodexLink 控制台").OnClick(func(ctx *application.Context) {
		window.Show()
		window.Restore()
		window.Focus()
	})
	trayMenu.Add("隐藏到系统托盘").OnClick(func(ctx *application.Context) {
		window.Hide()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("退出并断开").OnClick(func(ctx *application.Context) {
		forceQuit.Store(true)
		app.Quit()
	})
	systemTray.SetMenu(trayMenu)

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !forceQuit.Load() && service.keepRunningOnClose() {
			window.Hide()
			e.Cancel()
		}
	})

	err := app.Run()

	if err != nil {
		log.Fatal(err)
	}
}
