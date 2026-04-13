package database

import (
	"testing"
)

func TestOpenMemory(t *testing.T) {
	db, err := Open(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

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
}
