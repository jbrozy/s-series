// Command server runs the S-Check API and background workers.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"scheck/internal/cache"
	"scheck/internal/config"
	"scheck/internal/db"
	"scheck/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[s-check] ")
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("Datenverzeichnis: %v", err)
	}
	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	if err := db.Migrate(ctx, database, cfg.MigrationsDir); err != nil {
		log.Fatalf("Migration fehlgeschlagen: %v", err)
	}
	rdb := cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err := rdb.WaitReady(ctx); err != nil {
		log.Fatalf("Redis nicht erreichbar: %v", err)
	}
	srv := server.New(cfg, database, rdb)
	if err := srv.EnsureAdmin(ctx); err != nil {
		log.Printf("Admin-Anlage fehlgeschlagen: %v", err)
	}
	go srv.RunWorkers(ctx, cfg.Workers)

	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Handler(), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(sctx)
	}()
	log.Printf("API lauscht auf %s", cfg.ListenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
