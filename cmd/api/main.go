package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/bitop-dev/agent-platform-api/internal/api"
	"github.com/bitop-dev/agent-platform-api/internal/auth"
	"github.com/bitop-dev/agent-platform-api/internal/config"
	"github.com/bitop-dev/agent-platform-api/internal/db"
	"github.com/bitop-dev/agent-platform-api/internal/registry"
	"github.com/bitop-dev/agent-platform-api/internal/runner"
	"github.com/bitop-dev/agent-platform-api/internal/scheduler"
	"github.com/bitop-dev/agent-platform-api/internal/ws"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

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
	slog.Info("database ready", "driver", store.Driver())

	// Auth
	a := auth.New(cfg.JWTSecret, cfg.JWTExpiryMinutes)

	// Encryption for API keys at rest
	enc, err := auth.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryption: %w", err)
	}
	if cfg.EncryptionKey == "" {
		slog.Warn("ENCRYPTION_KEY not set — API keys stored in plaintext (dev mode)")
	}

	// WebSocket hub
	hub := ws.NewHub()

	// Skill registry syncer
	syncer := registry.NewSyncer(store.Queries)

	// Sync skills from all sources on startup (non-blocking)
	go func() {
		if err := syncer.SyncAll(context.Background()); err != nil {
			slog.Error("skill registry sync failed", "error", err)
		}
	}()

	// Runner
	r := runner.New(store, hub, 4)
	r.Start()
	defer r.Stop()

	// Scheduler — polls every 30s for due schedules
	sched := scheduler.New(store, r, enc, 30*time.Second)
	sched.Start()
	defer sched.Stop()

	// Router
	app := api.NewRouter(store, a, enc, r, hub, syncer, sched, cfg)

	// Graceful shutdown with drain period
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("received signal, draining...", "signal", sig.String())
		sched.Stop()
		slog.Info("scheduler stopped")
		r.Stop()
		slog.Info("runner drained")
		_ = app.ShutdownWithTimeout(10 * time.Second)
		slog.Info("server stopped")
	}()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	slog.Info("listening", "addr", addr)
	return app.Listen(addr)
}
