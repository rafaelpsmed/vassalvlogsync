//go:build !headless

package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"sync"

	"github.com/rafael/vassal-vlog-sync/internal/clientapp"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	core, err := clientapp.New()
	if err != nil {
		log.Fatal(err)
	}

	ui := NewWailsApp(core)

	var syncOnce sync.Once
	startSync := func() {
		syncOnce.Do(func() {
			go func() {
				if err := core.RunSync(); err != nil {
					log.Printf("sync error: %v", err)
				}
			}()
		})
	}
	startSync()

	webAssets, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	appMenu.Append(menu.EditMenu())
	appMenu.Append(menu.WindowMenu())

	err = wails.Run(&options.App{
		Title:             "Vassal vLog Sync",
		Width:             960,
		Height:            720,
		HideWindowOnClose: true,
		Menu:              appMenu,
		AssetServer: &assetserver.Options{
			Assets: webAssets,
		},
		OnStartup: func(ctx context.Context) {
			ui.startup(ctx)
			startSync()
		},
		Bind: []interface{}{ui},
	})
	if err != nil {
		log.Fatal(err)
	}
}
