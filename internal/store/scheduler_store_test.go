package store

import (
	"context"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/scheduler"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestSchedulerStore_GetOverridesEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)

	overrides, err := s.GetOverrides(context.Background())
	if err != nil {
		t.Fatalf("GetOverrides: %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("expected empty, got %d", len(overrides))
	}
}

func TestSchedulerStore_UpsertAndGetOverride(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	ovr := &scheduler.TaskOverride{
		TaskName:       "traffic-flush",
		Enabled:        false,
		CustomSchedule: "0 */2 * * * *",
	}
	if err := s.UpsertOverride(ctx, ovr); err != nil {
		t.Fatalf("UpsertOverride: %v", err)
	}

	got, err := s.GetOverride(ctx, "traffic-flush")
	if err != nil {
		t.Fatalf("GetOverride: %v", err)
	}
	if got == nil {
		t.Fatal("expected override, got nil")
	}
	if got.TaskName != "traffic-flush" {
		t.Errorf("TaskName = %q, want %q", got.TaskName, "traffic-flush")
	}
	if got.Enabled {
		t.Error("Enabled = true, want false")
	}
	if got.CustomSchedule != "0 */2 * * * *" {
		t.Errorf("CustomSchedule = %q, want %q", got.CustomSchedule, "0 */2 * * * *")
	}
}

func TestSchedulerStore_GetOverrideNotFound(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)

	got, err := s.GetOverride(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetOverride: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent override")
	}
}

func TestSchedulerStore_DeleteOverride(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	ovr := &scheduler.TaskOverride{TaskName: "quota-check", Enabled: false}
	if err := s.UpsertOverride(ctx, ovr); err != nil {
		t.Fatalf("UpsertOverride: %v", err)
	}

	if err := s.DeleteOverride(ctx, "quota-check"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}

	got, _ := s.GetOverride(ctx, "quota-check")
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSchedulerStore_InsertAndGetHistory(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	rec := &scheduler.ExecutionRecord{
		TaskName:   "traffic-flush",
		StartedAt:  time.Now().Unix() - 1,
		FinishedAt: time.Now().Unix(),
		DurationMs: 1000,
		Status:     "success",
	}
	if err := s.InsertHistory(ctx, rec); err != nil {
		t.Fatalf("InsertHistory: %v", err)
	}

	latest, err := s.GetLatestHistory(ctx, "traffic-flush")
	if err != nil {
		t.Fatalf("GetLatestHistory: %v", err)
	}
	if latest == nil {
		t.Fatal("expected record, got nil")
	}
	if latest.TaskName != "traffic-flush" {
		t.Errorf("TaskName = %q, want %q", latest.TaskName, "traffic-flush")
	}
	if latest.Status != "success" {
		t.Errorf("Status = %q, want %q", latest.Status, "success")
	}
	if latest.DurationMs != 1000 {
		t.Errorf("DurationMs = %d, want 1000", latest.DurationMs)
	}
}

func TestSchedulerStore_GetLatestHistoryEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)

	got, err := s.GetLatestHistory(context.Background(), "traffic-flush")
	if err != nil {
		t.Fatalf("GetLatestHistory: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for no history")
	}
}

func TestSchedulerStore_ListHistoryByTask(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		rec := &scheduler.ExecutionRecord{
			TaskName:   "health-check",
			StartedAt:  now - int64(5-i)*60,
			FinishedAt: now - int64(5-i)*60 + 1,
			DurationMs: int64(i * 100),
			Status:     "success",
		}
		if err := s.InsertHistory(ctx, rec); err != nil {
			t.Fatalf("InsertHistory %d: %v", i, err)
		}
	}

	records, err := s.ListHistoryByTask(ctx, "health-check", 3, 0)
	if err != nil {
		t.Fatalf("ListHistoryByTask: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Should be newest first
	if records[0].StartedAt < records[1].StartedAt {
		t.Error("expected newest first")
	}
}

func TestSchedulerStore_CleanOldHistory(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert old record (10 days ago)
	old := &scheduler.ExecutionRecord{
		TaskName:   "token-cleanup",
		StartedAt:  now - 864000,
		FinishedAt: now - 864000 + 1,
		DurationMs: 500,
		Status:     "success",
	}
	if err := s.InsertHistory(ctx, old); err != nil {
		t.Fatalf("InsertHistory old: %v", err)
	}

	// Insert recent record
	recent := &scheduler.ExecutionRecord{
		TaskName:   "token-cleanup",
		StartedAt:  now - 60,
		FinishedAt: now - 59,
		DurationMs: 1000,
		Status:     "success",
	}
	if err := s.InsertHistory(ctx, recent); err != nil {
		t.Fatalf("InsertHistory recent: %v", err)
	}

	// Clean records older than 1 day
	if err := s.CleanOldHistory(ctx, 24*time.Hour); err != nil {
		t.Fatalf("CleanOldHistory: %v", err)
	}

	records, _ := s.ListHistoryByTask(ctx, "token-cleanup", 10, 0)
	if len(records) != 1 {
		t.Fatalf("expected 1 record after cleanup, got %d", len(records))
	}
	if records[0].DurationMs != 1000 {
		t.Errorf("kept wrong record: DurationMs = %d, want 1000", records[0].DurationMs)
	}
}

func TestSchedulerStore_ListHistory(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	now := time.Now().Unix()
	tasks := []string{"task-a", "task-b", "task-c"}
	for i, name := range tasks {
		_ = s.InsertHistory(ctx, &scheduler.ExecutionRecord{
			TaskName:   name,
			StartedAt:  now - int64(len(tasks)-i)*60,
			FinishedAt: now - int64(len(tasks)-i)*60 + 1,
			DurationMs: int64(i * 100),
			Status:     "success",
		})
	}

	records, err := s.ListHistory(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Should be newest first
	if records[0].StartedAt < records[1].StartedAt {
		t.Error("expected newest first")
	}
}

func TestSchedulerStore_ListHistory_Pagination(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		_ = s.InsertHistory(ctx, &scheduler.ExecutionRecord{
			TaskName:   "pag-task",
			StartedAt:  now - int64(5-i)*10,
			FinishedAt: now - int64(5-i)*10 + 1,
			DurationMs: int64(i),
			Status:     "success",
		})
	}

	// Page 1: limit=2, offset=0
	page1, err := s.ListHistory(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListHistory page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: expected 2, got %d", len(page1))
	}

	// Page 2: limit=2, offset=2
	page2, err := s.ListHistory(ctx, 2, 2)
	if err != nil {
		t.Fatalf("ListHistory page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2: expected 2, got %d", len(page2))
	}

	// Pages should not overlap
	if page1[0].ID == page2[0].ID {
		t.Error("pages should have different records")
	}
}

func TestSchedulerStore_HistoryWithError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSchedulerStore(db)
	ctx := context.Background()

	rec := &scheduler.ExecutionRecord{
		TaskName:   "update-check",
		StartedAt:  time.Now().Unix() - 1,
		FinishedAt: time.Now().Unix(),
		DurationMs: 2000,
		Status:     "error",
		Error:      "connection refused",
	}
	if err := s.InsertHistory(ctx, rec); err != nil {
		t.Fatalf("InsertHistory: %v", err)
	}

	latest, _ := s.GetLatestHistory(ctx, "update-check")
	if latest == nil {
		t.Fatal("expected record")
	}
	if latest.Status != "error" {
		t.Errorf("Status = %q, want %q", latest.Status, "error")
	}
	if latest.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", latest.Error, "connection refused")
	}
}
