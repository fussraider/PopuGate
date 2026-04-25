package database

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	mu       sync.Mutex
	instance *sql.DB
)

// migration represents a numbered SQL migration file.
type migration struct {
	version int
	name    string
	content string
}

// Config holds database configuration.
type Config struct {
	Path string // Path to the SQLite database file
}

// Open creates a new SQLite connection and runs migrations.
// Uses a mutex instead of sync.Once so that transient failures can be retried.
func Open(cfg Config) (*sql.DB, error) {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance, nil
	}
	var err error
	instance, err = openDB(cfg)
	if err != nil {
		instance = nil // ensure no half-initialized state
		return nil, err
	}
	return instance, nil
}

// openDB opens the database and runs migrations (extracted for testability).
func openDB(cfg Config) (*sql.DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(cfg.Path)
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		return nil, fmt.Errorf("create db directory: %w", mkErr)
	}

	db, err := sql.Open("sqlite", cfg.Path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}

	// SQLite performance optimizations
	db.SetMaxOpenConns(1)

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// runMigrations discovers and applies pending migrations in order.
func runMigrations(db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	if len(migrations) == 0 {
		return nil
	}

	// Ensure schema_version table exists
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		name       TEXT    NOT NULL,
		applied_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	// Get applied versions
	applied := make(map[int]bool)
	rows, err := db.Query("SELECT version FROM schema_version")
	if err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	// Apply pending migrations
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.content); err != nil {
			tx.Rollback()
			return fmt.Errorf("run migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_version (version, name) VALUES (?, ?)", m.version, m.name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}

	return nil
}

// loadMigrations reads migration files from the embedded FS, sorted by version number.
func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(parts[1], ".sql")
		content, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    name,
			content: string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

// Close closes the database connection.
func Close() error {
	if instance != nil {
		return instance.Close()
	}
	return nil
}

// Reset resets the singleton for testing purposes.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	instance = nil
}
