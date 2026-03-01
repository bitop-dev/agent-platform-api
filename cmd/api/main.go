package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bitop-dev/agent-platform-api/internal/api"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/config"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open database
	store, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	// Run migrations
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Printf("database ready (%s)", store.Driver())

	// Auth
	a := auth.New(cfg.JWTSecret, cfg.JWTExpiryMinutes)

	// Encryption for API keys at rest
	enc, err := auth.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryption: %w", err)
	}
	if cfg.EncryptionKey == "" {
		log.Println("⚠ ENCRYPTION_KEY not set — API keys stored in plaintext (dev mode)")
	}

	// WebSocket hub
	hub := ws.NewHub()

	// Runner
	r := runner.New(store, hub, 4)
	r.Start()
	defer r.Stop()

	// Router
	app := api.NewRouter(store, a, enc, r, hub)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		_ = app.Shutdown()
	}()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("listening on %s", addr)
	return app.Listen(addr)
}
