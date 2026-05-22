package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMemory(t *testing.T) {
	Reset()

	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify settings table was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM settings").Scan(&count)
	if err != nil {
		t.Fatalf("query settings: %v", err)
	}
	if count == 0 {
		t.Error("expected default settings to be seeded")
	}

	// Verify other tables exist by inserting/selecting
	tables := []string{"secrets", "upstreams", "instances", "slaves",
		"traffic_global", "traffic_user", "geoblock_cache",
		"quota_alerts", "expiry_alerts", "token_blocklist"}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s not accessible: %v", table, err)
		}
	}

	Reset()
}

func TestOpen_RetriesOnTransientFailure(t *testing.T) {
	Reset()

	// First call with impossible path should fail
	_, err := Open(Config{Path: "/nonexistent/deeply/nested/that/cannot/be/created/test.db"})
	if err == nil {
		t.Fatal("expected error for invalid path")
	}

	// After failure, retry should succeed with a valid path
	dir := t.TempDir()
	db, err := Open(Config{Path: filepath.Join(dir, "retry.db")})
	if err != nil {
		t.Fatalf("retry Open should succeed, got: %v", err)
	}
	defer func() { _ = db.Close() }()

	var val int
	if err := db.QueryRow("SELECT 1").Scan(&val); err != nil {
		t.Fatalf("db query failed: %v", err)
	}

	Reset()
}

func TestOpen_Singleton(t *testing.T) {
	Reset()

	dir := t.TempDir()
	path := filepath.Join(dir, "singleton.db")

	db1, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}

	db2, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}

	if db1 != db2 {
		t.Error("expected same instance on second Open")
	}

	Reset()
	_ = db1.Close()
}

func TestOpen_CreatesDirectory(t *testing.T) {
	Reset()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "dir", "test.db")
	db, err := Open(Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Open with nested dir: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected database file to be created")
	}

	Reset()
}

func TestClose_Nil(t *testing.T) {
	Reset()
	if err := Close(); err != nil {
		t.Errorf("Close on nil instance should return nil, got: %v", err)
	}
}
