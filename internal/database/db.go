package database

import (
	"database/sql"
	"embed"
	"encoding/json"
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

	if err := migrateSecretTagsToJSON(db); err != nil {
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
	fmt.Printf("Loaded %d migrations\n", len(migrations))

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

		fmt.Printf("Applying migration %d (%s)\n", m.version, m.name)
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}

		content := m.content
		// Parse the migration content to get only the "Up" part.
		// We look for "-- +migrate Up:" and "-- +migrate Down:" markers.
		upIdx := strings.Index(content, "-- +migrate Up:")
		downIdx := strings.Index(content, "-- +migrate Down:")

		var sqlToRun string
		if upIdx != -1 {
			if downIdx != -1 && downIdx > upIdx {
				sqlToRun = content[upIdx+len("-- +migrate Up:") : downIdx]
			} else {
				sqlToRun = content[upIdx+len("-- +migrate Up:"):]
			}
		} else {
			// Fallback: if no markers, run everything (legacy)
			if downIdx != -1 {
				sqlToRun = content[:downIdx]
			} else {
				sqlToRun = content
			}
		}

		sqlToRun = strings.TrimSpace(sqlToRun)
		if sqlToRun != "" {
			// Split by semicolon and run each statement individually.
			// This is because Exec() might not support multiple ALTER TABLE statements in one call depending on driver.
			statements := strings.Split(sqlToRun, ";")
			for _, stmt := range statements {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" {
					continue
				}
				if _, err := tx.Exec(stmt); err != nil {
					// If column already exists, we can ignore this error for ADD COLUMN statements
					if strings.Contains(strings.ToUpper(stmt), "ADD COLUMN") && strings.Contains(err.Error(), "duplicate column name") {
						continue
					}
					tx.Rollback()
					return fmt.Errorf("run migration %d (%s) statement [%s]: %w", m.version, m.name, stmt, err)
				}
			}
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

// migrateSecretTagsToJSON converts existing comma-separated secret tags to JSON arrays.
// Idempotent: rows already containing JSON arrays are excluded by the WHERE clause.
func migrateSecretTagsToJSON(db *sql.DB) error {
	rows, err := db.Query("SELECT rowid, tags FROM secrets WHERE tags != '' AND tags != '[]' AND tags NOT LIKE '[%'")
	if err != nil {
		return fmt.Errorf("migrate secret tags: %w", err)
	}
	defer rows.Close()

	type row struct {
		rowid int64
		tags  string
	}
	var toUpdate []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.rowid, &r.tags); err != nil {
			continue
		}
		toUpdate = append(toUpdate, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate secret tags: %w", err)
	}

	for _, r := range toUpdate {
		parts := strings.Split(r.tags, ",")
		clean := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				clean = append(clean, p)
			}
		}
		jsonTags, _ := json.Marshal(clean)
		if _, err := db.Exec("UPDATE secrets SET tags = ? WHERE rowid = ?", string(jsonTags), r.rowid); err != nil {
			return fmt.Errorf("migrate secret tags row %d: %w", r.rowid, err)
		}
	}
	return nil
}
