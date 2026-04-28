package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestTrafficStore_GetGlobal_DefaultRow(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)

	// Migration inserts a default row with id=1 and zero values
	result, err := s.GetGlobal(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from default row")
	}
	if result.BytesIn != 0 || result.BytesOut != 0 {
		t.Errorf("expected zero traffic, got in=%d out=%d", result.BytesIn, result.BytesOut)
	}
}
