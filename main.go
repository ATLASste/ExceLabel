package main

import (
	"embed"
	"log"
	"path/filepath"

	appsvc "excelabel/internal/app"
	presentation "excelabel/internal/presentation/wails"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service := appsvc.NewSyncService(appsvc.Config{
		RootDir:         filepath.Clean("."),
		WorkbookPath:    filepath.Join(".", "output", "excelabel.xlsx"),
		AutoSyncEnabled: false,
	})
	app := presentation.NewDesktopApp(service)

	err := wails.Run(&options.App{
		Title:  "ExceLabel",
		Width:  1640,
		Height: 1000,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.Startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
