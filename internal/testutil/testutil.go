package testutil

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// OpenTestDB opens an in-memory SQLite database with migrations applied.
// It bypasses the database.Open singleton, creating a fresh instance per test.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	db.SetMaxOpenConns(1)

	if err := applyMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return db
}

type migration struct {
	version int
	name    string
	content string
}

func applyMigrations(db *sql.DB) error {
	entries, err := embeddedMigrations.ReadDir("migrations")
	if err != nil {
		return err
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
		content, err := embeddedMigrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		migrations = append(migrations, migration{version, name, string(content)})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// Create schema_version table
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		name       TEXT    NOT NULL,
		applied_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`); err != nil {
		return err
	}

	for _, m := range migrations {
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM schema_version WHERE version = ?", m.version).Scan(&count)
		if count > 0 {
			continue
		}

		cleanSQL := m.content
		if strings.Contains(m.content, "-- +migrate Up:") {
			parts := strings.Split(m.content, "-- +migrate Down:")
			cleanSQL = strings.ReplaceAll(parts[0], "-- +migrate Up:", "")
		} else {
			cleanSQL = strings.ReplaceAll(cleanSQL, "-- +migrate Up:", "")
			cleanSQL = strings.ReplaceAll(cleanSQL, "-- +migrate Down:", "")
		}

		if _, err := db.Exec(cleanSQL); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w\nSQL: %s", m.version, m.name, err, cleanSQL)
		}
		_, _ = db.Exec("INSERT INTO schema_version (version, name) VALUES (?, ?)", m.version, m.name)
	}

	return nil
}

// SeedTraffic inserts traffic data for testing via raw SQL.
func SeedTraffic(t *testing.T, db *sql.DB, label string, bytesIn, bytesOut int64) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO traffic_user (label, instance_id, bytes_in, bytes_out, snap_in, snap_out)
		VALUES (?, 1, ?, ?, ?, ?)
		ON CONFLICT(label, instance_id) DO UPDATE SET
			bytes_in = bytes_in + excluded.bytes_in,
			bytes_out = bytes_out + excluded.bytes_out,
			snap_in = excluded.snap_in,
			snap_out = excluded.snap_out
	`, label, bytesIn, bytesOut, bytesIn, bytesOut)
	if err != nil {
		t.Fatalf("SeedTraffic: %v", err)
	}
}
