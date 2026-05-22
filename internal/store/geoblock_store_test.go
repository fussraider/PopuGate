package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestGeoblockStore_GetCacheNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewGeoblockCacheStore(db)

	filePath, downloadedAt, err := s.GetCache(context.Background(), "US")
	if err != nil {
		t.Fatalf("GetCache: %v", err)
	}
	if filePath != "" {
		t.Fatalf("expected empty file path, got '%s'", filePath)
	}
	if downloadedAt != 0 {
		t.Fatalf("expected zero downloaded_at, got %d", downloadedAt)
	}
}

func TestGeoblockStore_SetCacheGetCacheRoundtrip(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewGeoblockCacheStore(db)
	ctx := context.Background()

	if err := s.SetCache(ctx, "US", "/tmp/geoblock/us.cidr"); err != nil {
		t.Fatalf("SetCache: %v", err)
	}

	filePath, downloadedAt, err := s.GetCache(ctx, "US")
	if err != nil {
		t.Fatalf("GetCache: %v", err)
	}
	if filePath != "/tmp/geoblock/us.cidr" {
		t.Fatalf("expected '/tmp/geoblock/us.cidr', got '%s'", filePath)
	}
	if downloadedAt == 0 {
		t.Fatal("expected non-zero downloaded_at timestamp")
	}
}

func TestGeoblockStore_SetCacheUpdatesExisting(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewGeoblockCacheStore(db)
	ctx := context.Background()

	_ = s.SetCache(ctx, "DE", "/old/path")

	// Small delay to ensure different timestamp in seconds
	// Update with new path
	if err := s.SetCache(ctx, "DE", "/new/path"); err != nil {
		t.Fatalf("SetCache update: %v", err)
	}

	filePath, _, err := s.GetCache(ctx, "DE")
	if err != nil {
		t.Fatalf("GetCache: %v", err)
	}
	if filePath != "/new/path" {
		t.Fatalf("expected '/new/path' after update, got '%s'", filePath)
	}
}
