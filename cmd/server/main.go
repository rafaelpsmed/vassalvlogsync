package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rafael/vassal-vlog-sync/internal/server"
	"github.com/rafael/vassal-vlog-sync/internal/ws"
)

func main() {
	addr := envOr("ADDR", ":8080")
	driver := envOr("DATABASE_DRIVER", "sqlite")
	dsn := envOr("DATABASE_DSN", "file:./data/vassalvlogsync.db?_pragma=foreign_keys(1)")
	dataDir := envOr("DATA_DIR", "./data/vlogs")
	baseURL := envOr("BASE_URL", "http://localhost:8080")

	store, err := server.Open(driver, dsn, dataDir, baseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	hub := ws.NewHub()
	defer hub.Close()
	mailer := server.NewMailerFromEnv()
	srv := server.NewServer(store, hub, mailer)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("servidor em %s (driver=%s)", addr, driver)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("desligando...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("servidor parado")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
