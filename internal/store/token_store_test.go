package store

import (
	"context"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestTokenStore_IsBlockedFalseInitially(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTokenBlocklistStore(db)

	blocked, err := s.IsBlocked(context.Background(), "some-jti")
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if blocked {
		t.Fatal("expected false for unadded token")
	}
}

func TestTokenStore_AddAndIsBlocked(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTokenBlocklistStore(db)
	ctx := context.Background()

	futureExpiry := time.Now().Add(1 * time.Hour).Unix()
	if err := s.Add(ctx, "my-token-id", futureExpiry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	blocked, err := s.IsBlocked(ctx, "my-token-id")
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if !blocked {
		t.Fatal("expected true for added token")
	}

	// Different JTI should not be blocked
	blocked, err = s.IsBlocked(ctx, "other-token-id")
	if err != nil {
		t.Fatalf("IsBlocked other: %v", err)
	}
	if blocked {
		t.Fatal("expected false for non-blocklisted token")
	}
}

func TestTokenStore_CleanupRemovesExpiredKeepsValid(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTokenBlocklistStore(db)
	ctx := context.Background()

	pastExpiry := time.Now().Add(-1 * time.Hour).Unix()
	futureExpiry := time.Now().Add(1 * time.Hour).Unix()

	if err := s.Add(ctx, "expired-token", pastExpiry); err != nil {
		t.Fatalf("Add expired: %v", err)
	}
	if err := s.Add(ctx, "valid-token", futureExpiry); err != nil {
		t.Fatalf("Add valid: %v", err)
	}

	if err := s.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Expired token should be removed
	blocked, err := s.IsBlocked(ctx, "expired-token")
	if err != nil {
		t.Fatalf("IsBlocked expired: %v", err)
	}
	if blocked {
		t.Fatal("expected false for expired (cleaned up) token")
	}

	// Valid token should still be present
	blocked, err = s.IsBlocked(ctx, "valid-token")
	if err != nil {
		t.Fatalf("IsBlocked valid: %v", err)
	}
	if !blocked {
		t.Fatal("expected true for valid (not cleaned up) token")
	}
}
