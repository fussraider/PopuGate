package testutil

import (
	"database/sql"
	"embed"
	"testing"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// OpenTestDB opens an in-memory SQLite database with migrations applied.
// It bypasses the database.Open singleton, creating a fresh instance per test.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	db.SetMaxOpenConns(1)

	content, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	if _, err := db.Exec(string(content)); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	return db
}
