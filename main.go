package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Version is set at build time via -ldflags "-X main.Version=vX.Y.Z".
// Defaults to "dev" for local builds.
var Version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Personal AI Agent",
		Width:  1200,
		Height: 800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnDomReady:       func(_ context.Context) { EnableMediaAccess() },
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
