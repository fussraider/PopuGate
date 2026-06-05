package store

import (
	"context"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestAuditStore_InsertAndList(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewAuditStore(db)
	ctx := context.Background()

	if err := s.Insert(ctx, "admin", "secret.create", "created label=user1"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := s.Insert(ctx, "admin", "secret.rotate", "rotated label=user1"); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	entries, err := s.List(ctx, 10, 0, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Most recent first
	if entries[0].Action != "secret.rotate" {
		t.Errorf("first entry action = %q, want secret.rotate", entries[0].Action)
	}
	if entries[0].User != "admin" {
		t.Errorf("first entry user = %q, want admin", entries[0].User)
	}
}

func TestAuditStore_ListPagination(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewAuditStore(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = s.Insert(ctx, "admin", "test.action", "detail")
	}

	entries, err := s.List(ctx, 2, 0, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	entries2, err := s.List(ctx, 2, 2, nil)
	if err != nil {
		t.Fatalf("List offset: %v", err)
	}
	if len(entries2) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries2))
	}

	// Different IDs
	if entries2[0].ID == entries[0].ID {
		t.Error("expected different entries for different offset")
	}
}

func TestAuditStore_ListEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewAuditStore(db)

	entries, err := s.List(context.Background(), 10, 0, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty list, got %d", len(entries))
	}
}

func TestAuditStore_CleanOld(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewAuditStore(db)
	ctx := context.Background()

	// Insert a stale entry directly (old timestamp)
	past := time.Now().Add(-48 * time.Hour).Unix()
	_, _ = db.ExecContext(ctx, "INSERT INTO audit_log (timestamp, user, action, detail) VALUES (?, ?, ?, ?)",
		past, "admin", "test.action", "detail")

	// Insert a fresh entry
	_ = s.Insert(ctx, "admin", "fresh.action", "detail")

	// Clean entries older than 24 hours
	count, err := s.CleanOld(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOld: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 cleaned, got %d", count)
	}

	entries, _ := s.List(ctx, 10, 0, nil)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after clean, got %d", len(entries))
	}
	if entries[0].Action != "fresh.action" {
		t.Errorf("expected fresh.action, got %s", entries[0].Action)
	}
}

func TestAuditStore_ListFiltered(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewAuditStore(db)
	ctx := context.Background()

	_ = s.Insert(ctx, "admin1", "secret.create", "d1")
	_ = s.Insert(ctx, "admin2", "secret.rotate", "d2")
	_ = s.Insert(ctx, "system", "proxy.start", "d3")

	// Filter by user
	entries, err := s.List(ctx, 10, 0, &model.AuditFilter{Users: []string{"admin1", "system"}})
	if err != nil {
		t.Fatalf("List filtered user: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Filter by action
	entries, err = s.List(ctx, 10, 0, &model.AuditFilter{Actions: []string{"secret.rotate"}})
	if err != nil {
		t.Fatalf("List filtered action: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].User != "admin2" {
		t.Errorf("expected user admin2, got %s", entries[0].User)
	}
}
