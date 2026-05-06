package service

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// BulkToggle
// ---------------------------------------------------------------------------

func TestSecretService_BulkToggle_Enable(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "u1", "")
	svc.Add(ctx, "u2", "")
	svc.Toggle(ctx, "u1", false) // disable u1
	svc.Toggle(ctx, "u2", false) // disable u2

	n, err := svc.BulkToggle(ctx, []string{"u1", "u2"}, true)
	if err != nil {
		t.Fatalf("BulkToggle enable: %v", err)
	}
	if n != 2 {
		t.Errorf("BulkToggle enabled %d, want 2", n)
	}

	s, _ := svc.Get(ctx, "u1")
	if !s.Enabled {
		t.Error("u1 should be enabled")
	}
}

func TestSecretService_BulkToggle_DisableAll_Rejected(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "only", "")
	// "only" is the sole enabled secret — disabling it must be rejected.
	_, err := svc.BulkToggle(ctx, []string{"only"}, false)
	if err == nil {
		t.Error("expected error when disabling all enabled secrets, got nil")
	}
}

func TestSecretService_BulkToggle_DisablePartial(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "a", "")
	svc.Add(ctx, "b", "")
	svc.Add(ctx, "c", "")

	// Disable a & b while c remains enabled — should succeed.
	n, err := svc.BulkToggle(ctx, []string{"a", "b"}, false)
	if err != nil {
		t.Fatalf("BulkToggle partial disable: %v", err)
	}
	if n != 2 {
		t.Errorf("BulkToggle disabled %d, want 2", n)
	}

	sa, _ := svc.Get(ctx, "a")
	sb, _ := svc.Get(ctx, "b")
	sc, _ := svc.Get(ctx, "c")
	if sa.Enabled || sb.Enabled {
		t.Error("a and b should be disabled")
	}
	if !sc.Enabled {
		t.Error("c should still be enabled")
	}
}

// ---------------------------------------------------------------------------
// BulkSetLimits
// ---------------------------------------------------------------------------

func TestSecretService_BulkSetLimits(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "m1", "")
	svc.Add(ctx, "m2", "")

	n, err := svc.BulkSetLimits(ctx, []string{"m1", "m2"}, 5, 3, 1<<20, "")
	if err != nil {
		t.Fatalf("BulkSetLimits: %v", err)
	}
	if n != 2 {
		t.Errorf("BulkSetLimits updated %d, want 2", n)
	}

	s1, _ := svc.Get(ctx, "m1")
	if s1.MaxConns != 5 || s1.MaxIPs != 3 || s1.QuotaBytes != 1<<20 {
		t.Errorf("m1 limits unexpected: %+v", s1)
	}
	s2, _ := svc.Get(ctx, "m2")
	if s2.MaxConns != 5 {
		t.Errorf("m2.MaxConns = %d, want 5", s2.MaxConns)
	}
}

func TestSecretService_BulkSetLimits_Empty(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	n, err := svc.BulkSetLimits(ctx, []string{}, 5, 3, 0, "")
	if err != nil {
		t.Fatalf("BulkSetLimits empty: %v", err)
	}
	if n != 0 {
		t.Errorf("BulkSetLimits empty returned %d, want 0", n)
	}
}
