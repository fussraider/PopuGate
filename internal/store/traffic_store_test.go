package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestTrafficStore_GetGlobalReturnsZerosInitially(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)

	snap, err := s.GetGlobal(context.Background())
	if err != nil {
		t.Fatalf("GetGlobal: %v", err)
	}
	if snap.BytesIn != 0 || snap.BytesOut != 0 {
		t.Fatalf("expected zero bytes, got in=%d out=%d", snap.BytesIn, snap.BytesOut)
	}
	if snap.SnapIn != 0 || snap.SnapOut != 0 {
		t.Fatalf("expected zero snap, got in=%d out=%d", snap.SnapIn, snap.SnapOut)
	}
}

func TestTrafficStore_UpdateGlobalCumulative(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	if err := s.UpdateGlobal(ctx, 100, 200, 50, 100); err != nil {
		t.Fatalf("UpdateGlobal first: %v", err)
	}
	if err := s.UpdateGlobal(ctx, 300, 400, 150, 200); err != nil {
		t.Fatalf("UpdateGlobal second: %v", err)
	}

	snap, err := s.GetGlobal(ctx)
	if err != nil {
		t.Fatalf("GetGlobal: %v", err)
	}
	// bytes_in/out are cumulative (100+300, 200+400)
	if snap.BytesIn != 400 {
		t.Fatalf("expected bytes_in=400, got %d", snap.BytesIn)
	}
	if snap.BytesOut != 600 {
		t.Fatalf("expected bytes_out=600, got %d", snap.BytesOut)
	}
	// snap_in/out are overwritten with latest value
	if snap.SnapIn != 150 {
		t.Fatalf("expected snap_in=150, got %d", snap.SnapIn)
	}
	if snap.SnapOut != 200 {
		t.Fatalf("expected snap_out=200, got %d", snap.SnapOut)
	}
}

func TestTrafficStore_ResetGlobal(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	s.UpdateGlobal(ctx, 1000, 2000, 500, 1000)

	if err := s.ResetGlobal(ctx); err != nil {
		t.Fatalf("ResetGlobal: %v", err)
	}

	snap, err := s.GetGlobal(ctx)
	if err != nil {
		t.Fatalf("GetGlobal: %v", err)
	}
	if snap.BytesIn != 0 || snap.BytesOut != 0 {
		t.Fatalf("expected zero bytes after reset, got in=%d out=%d", snap.BytesIn, snap.BytesOut)
	}
	if snap.SnapIn != 0 || snap.SnapOut != 0 {
		t.Fatalf("expected zero snap after reset, got in=%d out=%d", snap.SnapIn, snap.SnapOut)
	}
}

func TestTrafficStore_UpdateUserTrafficUpsert(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	// Insert
	if err := s.UpdateUserTraffic(ctx, "user1", 100, 200, 50, 100); err != nil {
		t.Fatalf("UpdateUserTraffic insert: %v", err)
	}

	got, err := s.GetUserTraffic(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserTraffic: %v", err)
	}
	if got.BytesIn != 100 || got.BytesOut != 200 {
		t.Fatalf("expected in=100 out=200, got in=%d out=%d", got.BytesIn, got.BytesOut)
	}

	snapIn, snapOut, err := s.GetUserSnapshot(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserSnapshot: %v", err)
	}
	if snapIn != 50 || snapOut != 100 {
		t.Fatalf("expected snap_in=50 snap_out=100, got in=%d out=%d", snapIn, snapOut)
	}

	// Upsert (update) - cumulative bytes, overwritten snap
	if err := s.UpdateUserTraffic(ctx, "user1", 300, 400, 150, 200); err != nil {
		t.Fatalf("UpdateUserTraffic upsert: %v", err)
	}

	got, err = s.GetUserTraffic(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserTraffic after upsert: %v", err)
	}
	if got.BytesIn != 400 {
		t.Fatalf("expected cumulative bytes_in=400, got %d", got.BytesIn)
	}
	if got.BytesOut != 600 {
		t.Fatalf("expected cumulative bytes_out=600, got %d", got.BytesOut)
	}

	snapIn, snapOut, err = s.GetUserSnapshot(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserSnapshot after upsert: %v", err)
	}
	if snapIn != 150 || snapOut != 200 {
		t.Fatalf("expected snap_in=150 snap_out=200, got in=%d out=%d", snapIn, snapOut)
	}
}

func TestTrafficStore_GetUserTrafficNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)

	got, err := s.GetUserTraffic(context.Background(), "noone")
	if err != nil {
		t.Fatalf("GetUserTraffic: %v", err)
	}
	if got.Label != "noone" {
		t.Fatalf("expected label 'noone', got '%s'", got.Label)
	}
	if got.BytesIn != 0 || got.BytesOut != 0 {
		t.Fatalf("expected zero traffic, got in=%d out=%d", got.BytesIn, got.BytesOut)
	}
}

func TestTrafficStore_GetUserSnapshotNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)

	snapIn, snapOut, err := s.GetUserSnapshot(context.Background(), "noone")
	if err != nil {
		t.Fatalf("GetUserSnapshot: %v", err)
	}
	if snapIn != 0 || snapOut != 0 {
		t.Fatalf("expected 0,0 for nonexistent, got %d,%d", snapIn, snapOut)
	}
}

func TestTrafficStore_FlushTrafficWithMultipleUsers(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	global := model.TrafficSnapshot{BytesIn: 1000, BytesOut: 2000, SnapIn: 500, SnapOut: 800}
	users := map[string]model.TrafficSnapshot{
		"user1": {BytesIn: 100, BytesOut: 200, SnapIn: 50, SnapOut: 80},
		"user2": {BytesIn: 300, BytesOut: 400, SnapIn: 150, SnapOut: 200},
	}

	if err := s.FlushTraffic(ctx, global, users); err != nil {
		t.Fatalf("FlushTraffic: %v", err)
	}

	// Verify global
	g, err := s.GetGlobal(ctx)
	if err != nil {
		t.Fatalf("GetGlobal: %v", err)
	}
	if g.BytesIn != 1000 || g.BytesOut != 2000 {
		t.Fatalf("global: expected in=1000 out=2000, got in=%d out=%d", g.BytesIn, g.BytesOut)
	}
	if g.SnapIn != 500 || g.SnapOut != 800 {
		t.Fatalf("global snap: expected in=500 out=800, got in=%d out=%d", g.SnapIn, g.SnapOut)
	}

	// Verify user1
	u1, err := s.GetUserTraffic(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserTraffic user1: %v", err)
	}
	if u1.BytesIn != 100 || u1.BytesOut != 200 {
		t.Fatalf("user1: expected in=100 out=200, got in=%d out=%d", u1.BytesIn, u1.BytesOut)
	}

	// Verify user2
	u2, err := s.GetUserTraffic(ctx, "user2")
	if err != nil {
		t.Fatalf("GetUserTraffic user2: %v", err)
	}
	if u2.BytesIn != 300 || u2.BytesOut != 400 {
		t.Fatalf("user2: expected in=300 out=400, got in=%d out=%d", u2.BytesIn, u2.BytesOut)
	}
}

func TestTrafficStore_FlushTrafficAtomicity(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	// First flush
	s.FlushTraffic(ctx,
		model.TrafficSnapshot{BytesIn: 100, BytesOut: 200, SnapIn: 50, SnapOut: 100},
		map[string]model.TrafficSnapshot{
			"user1": {BytesIn: 10, BytesOut: 20, SnapIn: 5, SnapOut: 10},
		},
	)

	// Second flush - cumulative
	s.FlushTraffic(ctx,
		model.TrafficSnapshot{BytesIn: 500, BytesOut: 600, SnapIn: 250, SnapOut: 300},
		map[string]model.TrafficSnapshot{
			"user1": {BytesIn: 50, BytesOut: 60, SnapIn: 25, SnapOut: 30},
		},
	)

	// Both global and user should be updated atomically
	g, _ := s.GetGlobal(ctx)
	if g.BytesIn != 600 {
		t.Fatalf("expected cumulative global bytes_in=600, got %d", g.BytesIn)
	}

	u1, _ := s.GetUserTraffic(ctx, "user1")
	if u1.BytesIn != 60 {
		t.Fatalf("expected cumulative user1 bytes_in=60, got %d", u1.BytesIn)
	}
}

func TestTrafficStore_ListUserTraffic(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	s.UpdateUserTraffic(ctx, "alice", 100, 200, 0, 0)
	s.UpdateUserTraffic(ctx, "bob", 300, 400, 0, 0)

	users, err := s.ListUserTraffic(ctx)
	if err != nil {
		t.Fatalf("ListUserTraffic: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	// Ordered by label
	if users[0].Label != "alice" {
		t.Fatalf("expected first user alice, got %s", users[0].Label)
	}
	if users[1].Label != "bob" {
		t.Fatalf("expected second user bob, got %s", users[1].Label)
	}
}
