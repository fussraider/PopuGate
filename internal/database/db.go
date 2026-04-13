package database

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	once     sync.Once
	instance *sql.DB
)

// Config holds database configuration.
type Config struct {
	Path string // Path to the SQLite database file
}

// Open creates a new SQLite connection and runs migrations.
func Open(cfg Config) (*sql.DB, error) {
	var err error
	once.Do(func() {
		// Ensure parent directory exists
		dir := filepath.Dir(cfg.Path)
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			err = fmt.Errorf("create db directory: %w", mkErr)
			return
		}

		instance, err = sql.Open("sqlite", cfg.Path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
		if err != nil {
			return
		}

		// SQLite performance optimizations
		instance.SetMaxOpenConns(1)

		err = runMigrations(instance)
	})
	return instance, err
}

// runMigrations reads and executes SQL migration files.
func runMigrations(db *sql.DB) error {
	content, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	if _, err := db.Exec(string(content)); err != nil {
		return fmt.Errorf("run migration: %w", err)
	}

	return nil
}

// Close closes the database connection.
func Close() error {
	if instance != nil {
		return instance.Close()
	}
	return nil
}
