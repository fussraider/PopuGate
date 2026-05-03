package service

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestTrafficService(t *testing.T) (*TrafficService, *store.TrafficStore, *store.SettingsStore) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	return NewTrafficService(trafficStore, settingsStore, nil, instanceStore), trafficStore, settingsStore
}

func seedTrafficData(t *testing.T, ts *store.TrafficStore) {
	t.Helper()
	ctx := context.Background()

	// Seed global traffic
	err := ts.UpdateGlobal(ctx, 1000, 2000, 500, 1000)
	if err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	// Seed user traffic
	err = ts.UpdateUserTraffic(ctx, "user1", 300, 400, 150, 200)
	if err != nil {
		t.Fatalf("UpdateUserTraffic user1: %v", err)
	}

	err = ts.UpdateUserTraffic(ctx, "user2", 500, 600, 250, 300)
	if err != nil {
		t.Fatalf("UpdateUserTraffic user2: %v", err)
	}
}

func seedTrafficHistory(t *testing.T, ts *store.TrafficStore) {
	t.Helper()
	ctx := context.Background()

	// Insert history records for different timestamps
	now := int64(1714700000)
	users := map[string][2]int64{
		"user1": {100, 200},
		"user2": {300, 400},
	}

	for i := 0; i < 3; i++ {
		tsNow := now + int64(i*3600) // one hour apart
		err := ts.InsertHistoryBatch(ctx, tsNow, int64(50*(i+1)), int64(60*(i+1)), users)
		if err != nil {
			t.Fatalf("InsertHistoryBatch %d: %v", i, err)
		}
	}
}

func TestTrafficService_GetReport(t *testing.T) {
	svc, ts, _ := newTestTrafficService(t)
	seedTrafficData(t, ts)
	ctx := context.Background()

	report, err := svc.GetReport(ctx)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}

	// Global traffic should reflect the seeded deltas
	if report.Global.TotalIn != 1000 {
		t.Errorf("Global.TotalIn = %d, want 1000", report.Global.TotalIn)
	}
	if report.Global.TotalOut != 2000 {
		t.Errorf("Global.TotalOut = %d, want 2000", report.Global.TotalOut)
	}

	// Should have two user entries
	if len(report.Users) != 2 {
		t.Fatalf("len(Users) = %d, want 2", len(report.Users))
	}
}

func TestTrafficService_GetReport_Empty(t *testing.T) {
	svc, _, _ := newTestTrafficService(t)
	ctx := context.Background()

	report, err := svc.GetReport(ctx)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}

	// With no data flushed, GetGlobal returns an error which propagates.
	// The traffic_global row starts at zeros from migration.
	if report.Global.TotalIn != 0 {
		t.Errorf("Global.TotalIn = %d, want 0", report.Global.TotalIn)
	}
	if report.Global.TotalOut != 0 {
		t.Errorf("Global.TotalOut = %d, want 0", report.Global.TotalOut)
	}
	if len(report.Users) != 0 {
		t.Errorf("len(Users) = %d, want 0", len(report.Users))
	}
}

func TestTrafficService_GetUserTraffic(t *testing.T) {
	svc, ts, _ := newTestTrafficService(t)
	seedTrafficData(t, ts)
	ctx := context.Background()

	ut, err := svc.GetUserTraffic(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserTraffic: %v", err)
	}
	if ut.Label != "user1" {
		t.Errorf("Label = %q, want %q", ut.Label, "user1")
	}
	if ut.BytesIn != 300 {
		t.Errorf("BytesIn = %d, want 300", ut.BytesIn)
	}
	if ut.BytesOut != 400 {
		t.Errorf("BytesOut = %d, want 400", ut.BytesOut)
	}
}

func TestTrafficService_GetUserTraffic_NotFound(t *testing.T) {
	svc, _, _ := newTestTrafficService(t)
	ctx := context.Background()

	ut, err := svc.GetUserTraffic(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetUserTraffic: %v", err)
	}
	// Should return a zero-traffic record with the label
	if ut.Label != "nonexistent" {
		t.Errorf("Label = %q, want %q", ut.Label, "nonexistent")
	}
	if ut.BytesIn != 0 || ut.BytesOut != 0 {
		t.Errorf("BytesIn=%d, BytesOut=%d, want both 0", ut.BytesIn, ut.BytesOut)
	}
}

func TestTrafficService_GetHistory_NoAggregate(t *testing.T) {
	svc, ts, _ := newTestTrafficService(t)
	seedTrafficHistory(t, ts)
	ctx := context.Background()

	now := int64(1714700000)
	start := now - 1
	end := now + 3*3600 + 1

	records, err := svc.GetHistory(ctx, start, end, "user1", "")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("len(records) = %d, want 3", len(records))
	}
}

func TestTrafficService_GetHistory_HourAggregate(t *testing.T) {
	svc, ts, _ := newTestTrafficService(t)
	seedTrafficHistory(t, ts)
	ctx := context.Background()

	now := int64(1714700000)
	start := now - 1
	end := now + 3*3600 + 1

	records, err := svc.GetHistory(ctx, start, end, "user1", "hour")
	if err != nil {
		t.Fatalf("GetHistory hour: %v", err)
	}
	if len(records) == 0 {
		t.Error("expected at least one aggregated record")
	}

	// Each record should have aggregated bytes from the history entries
	for _, r := range records {
		if r.BytesIn <= 0 || r.BytesOut <= 0 {
			t.Errorf("aggregated record should have positive traffic: in=%d out=%d", r.BytesIn, r.BytesOut)
		}
	}
}

func TestTrafficService_GetHistory_DayAggregate(t *testing.T) {
	svc, ts, _ := newTestTrafficService(t)
	seedTrafficHistory(t, ts)
	ctx := context.Background()

	now := int64(1714700000)
	start := now - 1
	end := now + 3*3600 + 1

	records, err := svc.GetHistory(ctx, start, end, "user1", "day")
	if err != nil {
		t.Fatalf("GetHistory day: %v", err)
	}

	// All records within a few hours should aggregate into a single day bucket
	if len(records) != 1 {
		t.Errorf("len(records) = %d, want 1 (same day)", len(records))
	}

	if records[0].BytesIn <= 0 || records[0].BytesOut <= 0 {
		t.Errorf("aggregated record should have positive traffic: in=%d out=%d", records[0].BytesIn, records[0].BytesOut)
	}
}

func TestTrafficService_GetHistory_EmptyRange(t *testing.T) {
	svc, _, _ := newTestTrafficService(t)
	ctx := context.Background()

	records, err := svc.GetHistory(ctx, 0, 0, "user1", "")
	if err != nil {
		t.Fatalf("GetHistory empty: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0 for empty range", len(records))
	}
}

func TestTrafficService_GetHistory_GlobalLabel(t *testing.T) {
	svc, ts, _ := newTestTrafficService(t)
	seedTrafficHistory(t, ts)
	ctx := context.Background()

	now := int64(1714700000)
	start := now - 1
	end := now + 3*3600 + 1

	// Get history for global (empty label)
	records, err := svc.GetHistory(ctx, start, end, "", "")
	if err != nil {
		t.Fatalf("GetHistory global: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("len(records) = %d, want 3 global records", len(records))
	}
}
