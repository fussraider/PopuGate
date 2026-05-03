package service

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestAuditService(t *testing.T) *AuditService {
	t.Helper()
	db := testutil.OpenTestDB(t)
	return NewAuditService(store.NewAuditStore(db))
}

func TestAuditService_LogAndList(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()

	if err := svc.Log(ctx, "admin", "secret.create", "created label=u1"); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := svc.Log(ctx, "admin", "secret.rotate", "rotated label=u1"); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Action != "secret.rotate" {
		t.Errorf("first entry action = %q, want secret.rotate", entries[0].Action)
	}
}

func TestAuditService_ListEmpty(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()

	entries, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty list, got %d", len(entries))
	}
}

func TestAuditService_CleanOld(t *testing.T) {
	svc := newTestAuditService(t)
	ctx := context.Background()

	if err := svc.Log(ctx, "admin", "fresh.action", "detail"); err != nil {
		t.Fatalf("Log: %v", err)
	}

	count, err := svc.CleanOld(ctx)
	if err != nil {
		t.Fatalf("CleanOld: %v", err)
	}
	// Fresh entry should NOT be cleaned (< 30 days old)
	if count != 0 {
		t.Errorf("expected 0 cleaned, got %d", count)
	}
}
