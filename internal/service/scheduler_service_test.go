package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/scheduler"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestSchedulerService(t *testing.T) (*SchedulerService, *scheduler.Scheduler) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	schedulerStore := store.NewSchedulerStore(db)
	sched := scheduler.New()

	// Add a test task with a dummy function
	tasks := []scheduler.Task{
		{
			Name:     "traffic-flush",
			Schedule: "0 */1 * * * *",
			Fn: func(ctx context.Context) error {
				return nil
			},
		},
	}
	sched.StartWith(tasks, nil, schedulerStore)

	t.Cleanup(func() { sched.Stop() })

	return NewSchedulerService(schedulerStore, sched), sched
}

func TestSchedulerService_ListTasks(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	tasks, err := svc.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least one task")
	}

	// Find traffic-flush task
	var found bool
	for _, ts := range tasks {
		if ts.Name == "traffic-flush" {
			found = true
			if !ts.Enabled {
				t.Error("traffic-flush should be enabled")
			}
			if ts.DefaultSchedule == "" {
				t.Error("DefaultSchedule should not be empty")
			}
			if ts.EffectiveSchedule == "" {
				t.Error("EffectiveSchedule should not be empty")
			}
		}
	}
	if !found {
		t.Error("traffic-flush task not found in list")
	}
}

func TestSchedulerService_UpdateTask_UnknownTask(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	err := svc.UpdateTask(ctx, "nonexistent-task", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
	if !strings.Contains(err.Error(), "unknown task") {
		t.Errorf("error = %q, want 'unknown task'", err.Error())
	}
}

func TestSchedulerService_UpdateTask_InvalidCron(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	invalidCron := "not-a-cron"
	err := svc.UpdateTask(ctx, "traffic-flush", nil, &invalidCron)
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
	if !strings.Contains(err.Error(), "invalid cron") {
		t.Errorf("error = %q, want 'invalid cron'", err.Error())
	}
}

func TestSchedulerService_UpdateTask_Disable(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	enabled := false
	err := svc.UpdateTask(ctx, "traffic-flush", &enabled, nil)
	if err != nil {
		t.Fatalf("UpdateTask disable: %v", err)
	}

	tasks, err := svc.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	for _, ts := range tasks {
		if ts.Name == "traffic-flush" && ts.Enabled {
			t.Error("traffic-flush should be disabled")
		}
	}
}

func TestSchedulerService_UpdateTask_Enable(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	// First disable
	disabled := false
	err := svc.UpdateTask(ctx, "traffic-flush", &disabled, nil)
	if err != nil {
		t.Fatalf("UpdateTask disable: %v", err)
	}

	// Then re-enable
	enabled := true
	err = svc.UpdateTask(ctx, "traffic-flush", &enabled, nil)
	if err != nil {
		t.Fatalf("UpdateTask enable: %v", err)
	}

	tasks, err := svc.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	for _, ts := range tasks {
		if ts.Name == "traffic-flush" && !ts.Enabled {
			t.Error("traffic-flush should be enabled after re-enabling")
		}
	}
}

func TestSchedulerService_UpdateTask_ChangeSchedule(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	newSchedule := "0 */2 * * * *"
	enabled := true
	err := svc.UpdateTask(ctx, "traffic-flush", &enabled, &newSchedule)
	if err != nil {
		t.Fatalf("UpdateTask schedule: %v", err)
	}

	tasks, err := svc.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	for _, ts := range tasks {
		if ts.Name == "traffic-flush" {
			if ts.EffectiveSchedule != newSchedule {
				t.Errorf("EffectiveSchedule = %q, want %q", ts.EffectiveSchedule, newSchedule)
			}
			if !ts.IsOverridden {
				t.Error("task should be marked as overridden after schedule change")
			}
		}
	}
}

func TestSchedulerService_RunTaskNow_UnknownTask(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	_, err := svc.RunTaskNow(ctx, "nonexistent-task")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
	if !strings.Contains(err.Error(), "unknown task") {
		t.Errorf("error = %q, want 'unknown task'", err.Error())
	}
}

func TestSchedulerService_RunTaskNow_ValidTask(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	rec, err := svc.RunTaskNow(ctx, "traffic-flush")
	if err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil execution record")
	}
	if rec.TaskName != "traffic-flush" {
		t.Errorf("TaskName = %q, want %q", rec.TaskName, "traffic-flush")
	}
	if rec.Status != "success" {
		t.Errorf("Status = %q, want %q", rec.Status, "success")
	}
	if rec.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", rec.DurationMs)
	}
	if rec.StartedAt == 0 || rec.FinishedAt == 0 {
		t.Error("StartedAt and FinishedAt should be non-zero")
	}
}

func TestSchedulerService_RunTaskNow_ErrorTask(t *testing.T) {
	db := testutil.OpenTestDB(t)
	schedulerStore := store.NewSchedulerStore(db)
	sched := scheduler.New()

	tasks := []scheduler.Task{
		{
			Name:     "traffic-flush",
			Schedule: "0 */1 * * * *",
			Fn: func(ctx context.Context) error {
				return context.DeadlineExceeded
			},
		},
	}
	sched.StartWith(tasks, nil, schedulerStore)
	defer sched.Stop()

	svc := NewSchedulerService(schedulerStore, sched)
	ctx := context.Background()

	rec, err := svc.RunTaskNow(ctx, "traffic-flush")
	if err == nil {
		t.Fatal("expected error from task function")
	}
	if rec.Status != "error" {
		t.Errorf("Status = %q, want %q", rec.Status, "error")
	}
	if rec.Error == "" {
		t.Error("Error should not be empty for failed task")
	}
}

func TestSchedulerService_GetHistory(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	// Run a task to create history
	_, _ = svc.RunTaskNow(ctx, "traffic-flush")

	history, err := svc.GetHistory(ctx, "traffic-flush", 10, 0)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected at least one history record")
	}

	rec := history[0]
	if rec.TaskName != "traffic-flush" {
		t.Errorf("TaskName = %q, want %q", rec.TaskName, "traffic-flush")
	}
	if rec.Status != "success" {
		t.Errorf("Status = %q, want %q", rec.Status, "success")
	}
}

func TestSchedulerService_GetHistory_Empty(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	history, err := svc.GetHistory(ctx, "traffic-flush", 10, 0)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("len(history) = %d, want 0 for no runs", len(history))
	}
}

func TestSchedulerService_GetAllHistory(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	// Run task twice to create multiple history records
	_, _ = svc.RunTaskNow(ctx, "traffic-flush")
	time.Sleep(10 * time.Millisecond) // ensure different timestamps
	_, _ = svc.RunTaskNow(ctx, "traffic-flush")

	history, err := svc.GetAllHistory(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetAllHistory: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("len(history) = %d, want 2", len(history))
	}

	// Should be ordered by started_at DESC
	if history[0].StartedAt < history[1].StartedAt {
		t.Error("history should be ordered by started_at DESC")
	}
}

func TestSchedulerService_GetAllHistory_Pagination(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	// Create 3 history records
	for i := 0; i < 3; i++ {
		_, _ = svc.RunTaskNow(ctx, "traffic-flush")
		time.Sleep(10 * time.Millisecond)
	}

	// Get first page
	page1, err := svc.GetAllHistory(ctx, 2, 0)
	if err != nil {
		t.Fatalf("GetAllHistory page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("len(page1) = %d, want 2", len(page1))
	}

	// Get second page
	page2, err := svc.GetAllHistory(ctx, 2, 2)
	if err != nil {
		t.Fatalf("GetAllHistory page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("len(page2) = %d, want 1", len(page2))
	}
}

func TestSchedulerService_ListTasks_WithLastRun(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	// Run the task
	_, _ = svc.RunTaskNow(ctx, "traffic-flush")

	tasks, err := svc.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	for _, ts := range tasks {
		if ts.Name == "traffic-flush" {
			if ts.LastRun == nil {
				t.Fatal("LastRun should not be nil after running task")
			}
			if ts.LastRun.Status != "success" {
				t.Errorf("LastRun.Status = %q, want %q", ts.LastRun.Status, "success")
			}
		}
	}
}

func TestSchedulerService_UpdateTask_RevertToDefault(t *testing.T) {
	svc, _ := newTestSchedulerService(t)
	ctx := context.Background()

	// Change schedule to non-default
	newSchedule := "0 */2 * * * *"
	enabled := true
	err := svc.UpdateTask(ctx, "traffic-flush", &enabled, &newSchedule)
	if err != nil {
		t.Fatalf("UpdateTask schedule: %v", err)
	}

	// Revert to default schedule
	defaultSchedule := "0 */1 * * * *"
	err = svc.UpdateTask(ctx, "traffic-flush", &enabled, &defaultSchedule)
	if err != nil {
		t.Fatalf("UpdateTask revert: %v", err)
	}

	tasks, err := svc.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	for _, ts := range tasks {
		if ts.Name == "traffic-flush" {
			if ts.IsOverridden {
				t.Error("task should not be overridden after reverting to default schedule")
			}
		}
	}
}
