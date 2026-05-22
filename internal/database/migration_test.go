package database

import (
	"testing"
)

func TestOpenMemory_CreatesSchemaVersionTable(t *testing.T) {
	Reset()
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify schema_version table exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("schema_version table not accessible: %v", err)
	}

	// Verify at least one migration was recorded
	if count == 0 {
		t.Error("expected at least one migration to be recorded in schema_version")
	}
}

func TestOpenMemory_AllTablesExist(t *testing.T) {
	Reset()
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	tables := []string{
		"settings", "secrets", "upstreams", "instances", "slaves",
		"traffic_global", "traffic_user", "geoblock_cache",
		"quota_alerts", "expiry_alerts", "token_blocklist", "schema_version",
		"scheduler_task_overrides", "scheduler_history",
	}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s not accessible: %v", table, err)
		}
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	Reset()
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	_ = db.Close()

	// Re-opening should not re-apply migrations
	Reset()
	db, err = Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer func() { _ = db.Close() }()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if count == 0 {
		t.Error("expected migrations to be recorded")
	}
}

func TestReset_AllowsReopening(t *testing.T) {
	Reset()
	db1, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	_ = db1.Close()

	Reset()
	db2, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open after Reset: %v", err)
	}
	_ = db2.Close()
}
