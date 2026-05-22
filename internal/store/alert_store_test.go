package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestAlertStore_WasAlertedFalseInitially(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewQuotaAlertStore(db)

	alerted, err := s.WasAlerted(context.Background(), "user1", 80)
	if err != nil {
		t.Fatalf("WasAlerted: %v", err)
	}
	if alerted {
		t.Fatal("expected false for unmarked alert")
	}
}

func TestAlertStore_MarkAlertedAndWasAlerted(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewQuotaAlertStore(db)
	ctx := context.Background()

	if err := s.MarkAlerted(ctx, "user1", 80); err != nil {
		t.Fatalf("MarkAlerted: %v", err)
	}

	alerted, err := s.WasAlerted(ctx, "user1", 80)
	if err != nil {
		t.Fatalf("WasAlerted: %v", err)
	}
	if !alerted {
		t.Fatal("expected true after marking")
	}

	// Different percent for same label should not be alerted
	alerted, err = s.WasAlerted(ctx, "user1", 90)
	if err != nil {
		t.Fatalf("WasAlerted 90%%: %v", err)
	}
	if alerted {
		t.Fatal("expected false for different percent")
	}

	// Different label for same percent should not be alerted
	alerted, err = s.WasAlerted(ctx, "user2", 80)
	if err != nil {
		t.Fatalf("WasAlerted user2: %v", err)
	}
	if alerted {
		t.Fatal("expected false for different label")
	}
}

func TestAlertStore_ClearForLabel(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewQuotaAlertStore(db)
	ctx := context.Background()

	_ = s.MarkAlerted(ctx, "user1", 80)
	_ = s.MarkAlerted(ctx, "user1", 90)
	_ = s.MarkAlerted(ctx, "user2", 80)

	if err := s.ClearForLabel(ctx, "user1"); err != nil {
		t.Fatalf("ClearForLabel: %v", err)
	}

	// user1 alerts should be gone
	alerted, err := s.WasAlerted(ctx, "user1", 80)
	if err != nil {
		t.Fatalf("WasAlerted user1 after clear: %v", err)
	}
	if alerted {
		t.Fatal("expected false for user1 after clear")
	}
	alerted, err = s.WasAlerted(ctx, "user1", 90)
	if err != nil {
		t.Fatalf("WasAlerted user1 90%% after clear: %v", err)
	}
	if alerted {
		t.Fatal("expected false for user1 90%% after clear")
	}

	// user2 should still be alerted
	alerted, err = s.WasAlerted(ctx, "user2", 80)
	if err != nil {
		t.Fatalf("WasAlerted user2: %v", err)
	}
	if !alerted {
		t.Fatal("expected true for user2 (not cleared)")
	}
}
