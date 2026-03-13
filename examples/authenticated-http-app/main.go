package main

import (
	"embed"
	"log"

	"github.com/Eriyc/wailsrel/pkg/wailsupdate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	wailsupdate.RegisterEvents("update")
}

func main() {
	service := NewUpdateService(loadConfig())

	app := application.New(application.Options{
		Name:        "authenticated-http-app",
		Description: "Wails updater example using an authenticated HTTP proxy",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(service.AssetFS(assets)),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "wailsrel: Authenticated HTTP Example",
		URL:              "/",
		Width:            1320,
		Height:           920,
		DevToolsEnabled:  true,
		BackgroundColour: application.NewRGB(16, 15, 18),
		StartState:       application.WindowStateNormal,
		MinWidth:         900,
		MinHeight:        720,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
