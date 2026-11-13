module github.com/shuaigehuang/codexlink/apps/desktop

go 1.27.0

require (
	github.com/coder/websocket v1.8.15
	github.com/shuaigehuang/codexlink v0.0.0
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/wailsapp/wails/v3 v3.1.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
)

replace github.com/shuaigehuang/codexlink => ../../..
