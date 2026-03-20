package main

import (
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"

	appsvc "excelabel/internal/app"
	presentation "excelabel/internal/presentation/wails"
)

func main() {
	service := appsvc.NewSyncService(appsvc.Config{
		RootDir:         filepath.Clean("."),
		WorkbookPath:    filepath.Join(".", "output", "excelabel.xlsx"),
		AutoSyncEnabled: false,
	})
	app := presentation.NewDesktopApp(service)

	err := wails.Run(&options.App{
		Title:  "ExceLabel",
		Width:  1440,
		Height: 920,
		AssetServer: nil,
		OnStartup: app.Startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
