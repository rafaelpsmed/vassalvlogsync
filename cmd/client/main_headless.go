//go:build headless

package main

import (
	"flag"
	"log"

	"github.com/rafael/vassal-vlog-sync/internal/clientapp"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	app, err := clientapp.New()
	if err != nil {
		log.Fatal(err)
	}

	watchDir := flag.String("watch", app.Config().WatchDir, "pasta a monitorar")
	serverURL := flag.String("server", app.Config().ServerURL, "URL do servidor")
	flag.Parse()

	if *watchDir != "" {
		_ = app.SetWatchDir(*watchDir)
	}
	if *serverURL != "" {
		_ = app.SetServerURL(*serverURL)
	}

	log.Printf("cliente headless — partida=%s pasta=%s", app.Config().GameName, app.Config().WatchDir)
	if err := app.RunSync(); err != nil {
		log.Fatal(err)
	}
}
