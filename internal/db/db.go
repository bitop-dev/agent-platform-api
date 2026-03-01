// Package db provides database access for the platform using sqlc + goose.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/bitop-dev/agent-platform-api/internal/db/sqlc"
)

// embeddedMigrations is defined in migrations.go via go:embed

// Store wraps the sqlc queries with the underlying database connection.
type Store struct {
	*sqlc.Queries
	db     *sql.DB
	driver string
}

// Open creates a database connection from a URL.
// Supported: sqlite://path/to/file.db, postgres://...
func Open(databaseURL string) (*Store, error) {
	if strings.HasPrefix(databaseURL, "sqlite://") {
		return openSQLite(strings.TrimPrefix(databaseURL, "sqlite://"))
	}
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		return openPostgres(databaseURL)
	}
	return nil, fmt.Errorf("unsupported database URL: %s", databaseURL)
}

func openSQLite(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_fk=true")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}

	return &Store{Queries: sqlc.New(db), db: db, driver: "sqlite"}, nil
}

func openPostgres(url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	return &Store{Queries: sqlc.New(db), db: db, driver: "postgres"}, nil
}

// Migrate runs all pending goose migrations.
func (s *Store) Migrate(ctx context.Context) error {
	goose.SetBaseFS(embeddedMigrations)

	dialect := "sqlite3"
	if s.driver == "postgres" {
		dialect = "postgres"
	}
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, s.db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// Ping verifies the database connection.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Driver returns "sqlite" or "postgres".
func (s *Store) Driver() string {
	return s.driver
}

// DB returns the underlying sql.DB for transactions etc.
func (s *Store) DB() *sql.DB {
	return s.db
}
