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

	"github.com/fussraider/PopuGate/pkg/logger"
	_ "modernc.org/sqlite"
)

var dbLog = logger.WithScope("database")

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
		_ = db.Close()
		return nil, err
	}

	if err := migrateSecretTagsToJSON(db); err != nil {
		_ = db.Close()
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
	dbLog.Infof("Loaded %d migrations", len(migrations))

	if len(migrations) == 0 {
		return nil
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		name       TEXT    NOT NULL,
		applied_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	applied, err := loadAppliedVersions(db)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		dbLog.Infof("Applying migration %d (%s)", m.version, m.name)
		if err := applyMigration(db, m); err != nil {
			return err
		}
	}
	return nil
}

func loadAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_version")
	if err != nil {
		return nil, fmt.Errorf("read schema_version: %w", err)
	}
	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return nil, err
		}
		applied[v] = true
	}
	_ = rows.Close()
	return applied, nil
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.version, err)
	}

	sqlToRun := extractUpSQL(m.content)
	if sqlToRun != "" {
		if err := execMigrationStatements(tx, sqlToRun, m); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if _, err := tx.Exec("INSERT INTO schema_version (version, name) VALUES (?, ?)", m.version, m.name); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}
	return tx.Commit()
}

func extractUpSQL(content string) string {
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
		if downIdx != -1 {
			sqlToRun = content[:downIdx]
		} else {
			sqlToRun = content
		}
	}
	return strings.TrimSpace(sqlToRun)
}

func execMigrationStatements(tx *sql.Tx, sqlToRun string, m migration) error {
	for _, stmt := range strings.Split(sqlToRun, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			if strings.Contains(strings.ToUpper(stmt), "ADD COLUMN") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("run migration %d (%s) statement [%s]: %w", m.version, m.name, stmt, err)
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
	defer func() { _ = rows.Close() }()

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
		jsonTags, jsonErr := json.Marshal(clean)
		if jsonErr != nil {
			dbLog.Warnf("marshal tags for row %d: %v", r.rowid, jsonErr)
		}
		if _, err := db.Exec("UPDATE secrets SET tags = ? WHERE rowid = ?", string(jsonTags), r.rowid); err != nil {
			return fmt.Errorf("migrate secret tags row %d: %w", r.rowid, err)
		}
	}
	return nil
}
