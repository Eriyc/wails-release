package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:assets
var assets embed.FS

func init() {
	application.RegisterEvent[logEvent]("update:log")
	application.RegisterEvent[progressEvent]("update:progress")
	application.RegisterEvent[updateView]("update:available")
}

func main() {
	service := NewUpdateService(loadConfig())

	app := application.New(application.Options{
		Name:        "github-releases-app",
		Description: "Wails updater example using GitHub Releases directly",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "wailsrel: GitHub Releases Example",
		URL:              "/",
		Width:            1320,
		Height:           920,
		DevToolsEnabled:  true,
		BackgroundColour: application.NewRGB(14, 20, 26),
		StartState:       application.WindowStateNormal,
		MinWidth:         900,
		MinHeight:        720,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
