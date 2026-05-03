package store

import (
	"context"
	"testing"
	"time"

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

func TestTrafficStore_GetAllUserSnapshots(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	s.UpdateUserTraffic(ctx, "alice", 100, 200, 50, 80)
	s.UpdateUserTraffic(ctx, "bob", 300, 400, 150, 200)

	snaps, err := s.GetAllUserSnapshots(ctx)
	if err != nil {
		t.Fatalf("GetAllUserSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 users, got %d", len(snaps))
	}
	if snaps["alice"] != [2]int64{50, 80} {
		t.Errorf("alice snap = %v, want [50 80]", snaps["alice"])
	}
	if snaps["bob"] != [2]int64{150, 200} {
		t.Errorf("bob snap = %v, want [150 200]", snaps["bob"])
	}
}

func TestTrafficStore_GetAllUserSnapshots_Empty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)

	snaps, err := s.GetAllUserSnapshots(context.Background())
	if err != nil {
		t.Fatalf("GetAllUserSnapshots: %v", err)
	}
	if len(snaps) != 0 {
		t.Fatalf("expected empty, got %d", len(snaps))
	}
}

func TestTrafficStore_InsertHistoryBatch(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	ts := time.Now().Unix()
	users := map[string][2]int64{
		"alice": {1000, 2000},
		"bob":   {3000, 4000},
	}

	if err := s.InsertHistoryBatch(ctx, ts, 5000, 6000, users); err != nil {
		t.Fatalf("InsertHistoryBatch: %v", err)
	}

	// Verify global history
	records, err := s.GetHistory(ctx, ts-1, ts+1, "")
	if err != nil {
		t.Fatalf("GetHistory global: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 global record, got %d", len(records))
	}
	if records[0].BytesIn != 5000 || records[0].BytesOut != 6000 {
		t.Errorf("global = in=%d out=%d, want in=5000 out=6000", records[0].BytesIn, records[0].BytesOut)
	}

	// Verify user history
	aliceRecs, err := s.GetHistory(ctx, ts-1, ts+1, "alice")
	if err != nil {
		t.Fatalf("GetHistory alice: %v", err)
	}
	if len(aliceRecs) != 1 {
		t.Fatalf("expected 1 alice record, got %d", len(aliceRecs))
	}
	if aliceRecs[0].BytesIn != 1000 || aliceRecs[0].BytesOut != 2000 {
		t.Errorf("alice = in=%d out=%d, want in=1000 out=2000", aliceRecs[0].BytesIn, aliceRecs[0].BytesOut)
	}
}

func TestTrafficStore_InsertHistoryBatch_ZeroDeltas(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	ts := time.Now().Unix()
	// All zeros — nothing should be inserted
	users := map[string][2]int64{
		"alice": {0, 0},
	}

	if err := s.InsertHistoryBatch(ctx, ts, 0, 0, users); err != nil {
		t.Fatalf("InsertHistoryBatch: %v", err)
	}

	records, _ := s.GetHistory(ctx, ts-1, ts+1, "")
	if len(records) != 0 {
		t.Errorf("expected no records for zero deltas, got %d", len(records))
	}
}

func TestTrafficStore_GetHistory_Empty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	now := time.Now().Unix()
	records, err := s.GetHistory(ctx, now-3600, now, "nobody")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected empty, got %d", len(records))
	}
}

func TestTrafficStore_GetAggregatedHistory(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	hourSec := int64(3600)
	// Use fixed timestamps in the same hour bucket
	baseHour := int64(1700000000 / hourSec * hourSec) // aligned to hour boundary
	ts1 := baseHour + 600
	ts2 := baseHour + 1200
	users1 := map[string][2]int64{"alice": {100, 200}}
	users2 := map[string][2]int64{"alice": {300, 400}}

	s.InsertHistoryBatch(ctx, ts1, 500, 600, users1)
	s.InsertHistoryBatch(ctx, ts2, 700, 800, users2)

	// Aggregate by hour
	records, err := s.GetAggregatedHistory(ctx, baseHour, baseHour+hourSec, "alice", hourSec)
	if err != nil {
		t.Fatalf("GetAggregatedHistory: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 aggregated record, got %d", len(records))
	}
	if records[0].BytesIn != 400 {
		t.Errorf("aggregated bytes_in = %d, want 400", records[0].BytesIn)
	}
	if records[0].BytesOut != 600 {
		t.Errorf("aggregated bytes_out = %d, want 600", records[0].BytesOut)
	}
}

func TestTrafficStore_GetAggregatedHistory_Empty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	now := time.Now().Unix()
	records, err := s.GetAggregatedHistory(ctx, now-3600, now, "nobody", 3600)
	if err != nil {
		t.Fatalf("GetAggregatedHistory: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected empty, got %d", len(records))
	}
}

func TestTrafficStore_CleanOldHistory(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTrafficStore(db)
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert old record (2 days ago)
	users := map[string][2]int64{"old": {100, 200}}
	s.InsertHistoryBatch(ctx, now-172800, 1000, 2000, users)

	// Insert recent record
	users2 := map[string][2]int64{"recent": {300, 400}}
	s.InsertHistoryBatch(ctx, now-60, 500, 600, users2)

	if err := s.CleanOldHistory(ctx, 24*time.Hour); err != nil {
		t.Fatalf("CleanOldHistory: %v", err)
	}

	// Old record should be gone
	oldRecs, _ := s.GetHistory(ctx, now-172900, now-172700, "old")
	if len(oldRecs) != 0 {
		t.Errorf("old records should be cleaned, got %d", len(oldRecs))
	}

	// Recent record should remain
	recentRecs, _ := s.GetHistory(ctx, now-120, now, "recent")
	if len(recentRecs) != 1 {
		t.Errorf("recent records should remain, got %d", len(recentRecs))
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
