package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.cron == nil {
		t.Error("cron is nil")
	}
	if s.entries == nil {
		t.Error("entries map is nil")
	}
	if s.taskDefs == nil {
		t.Error("taskDefs map is nil")
	}
}

func TestDefaultTasks(t *testing.T) {
	tasks := DefaultTasks()
	if len(tasks) == 0 {
		t.Fatal("DefaultTasks returned empty")
	}

	names := make(map[string]bool)
	for _, task := range tasks {
		if task.Name == "" {
			t.Error("task has empty name")
		}
		if task.Schedule == "" {
			t.Errorf("task %s has empty schedule", task.Name)
		}
		if names[task.Name] {
			t.Errorf("duplicate task name: %s", task.Name)
		}
		names[task.Name] = true
	}

	expected := []string{
		"traffic-flush", "quota-check", "expiry-check", "health-check",
		"telegram-report", "replication-sync", "update-check", "telemt-check",
		"token-cleanup", "daily-backup", "backup-cleanup", "history-cleanup",
		"docker-host-check", "auto-update", "quota-reset", "auto-rotate",
		"upstream-health", "fronting-update",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing task: %s", name)
		}
	}
}

func TestKnownTaskNames(t *testing.T) {
	names := KnownTaskNames()
	if len(names) == 0 {
		t.Fatal("KnownTaskNames returned empty")
	}
	if !names["traffic-flush"] {
		t.Error("traffic-flush should be known")
	}
	if names["nonexistent"] {
		t.Error("nonexistent should not be known")
	}
}

func TestDefaultScheduleFor(t *testing.T) {
	schedule := DefaultScheduleFor("traffic-flush")
	if schedule == "" {
		t.Error("expected non-empty schedule for traffic-flush")
	}
	if schedule != "0 */1 * * * *" {
		t.Errorf("traffic-flush schedule = %q, want %q", schedule, "0 */1 * * * *")
	}

	unknown := DefaultScheduleFor("nonexistent")
	if unknown != "" {
		t.Errorf("nonexistent schedule = %q, want empty", unknown)
	}
}

func TestDefaultTimeoutFor(t *testing.T) {
	timeout := DefaultTimeoutFor("replication-sync")
	if timeout != 3*time.Minute {
		t.Errorf("replication-sync timeout = %v, want %v", timeout, 3*time.Minute)
	}

	defaultTimeout := DefaultTimeoutFor("traffic-flush")
	if defaultTimeout != defaultTaskTimeout {
		t.Errorf("traffic-flush timeout = %v, want %v", defaultTimeout, defaultTaskTimeout)
	}

	unknown := DefaultTimeoutFor("nonexistent")
	if unknown != defaultTaskTimeout {
		t.Errorf("nonexistent timeout = %v, want %v", unknown, defaultTaskTimeout)
	}
}

func TestStartWith_BasicTask(t *testing.T) {
	s := New()
	var called atomic.Int32

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 */1 * * * *",
			Fn: func(ctx context.Context) error {
				called.Add(1)
				return nil
			},
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 task status, got %d", len(statuses))
	}
	if statuses[0].Name != "test-task" {
		t.Errorf("task name = %q, want %q", statuses[0].Name, "test-task")
	}
	if !statuses[0].Enabled {
		t.Error("task should be enabled")
	}
}

func TestStartWith_NilFunction(t *testing.T) {
	s := New()

	tasks := []Task{
		{Name: "nil-fn", Schedule: "0 */1 * * * *", Fn: nil},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 task status, got %d", len(statuses))
	}
	if statuses[0].Enabled {
		t.Error("task with nil Fn should not be enabled")
	}
}

func TestStartWith_DisabledOverride(t *testing.T) {
	s := New()
	var called atomic.Int32

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 */1 * * * *",
			Fn: func(ctx context.Context) error {
				called.Add(1)
				return nil
			},
		},
	}

	overrides := map[string]TaskOverride{
		"test-task": {TaskName: "test-task", Enabled: false},
	}

	s.StartWith(tasks, overrides, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	if statuses[0].Enabled {
		t.Error("task should be disabled by override")
	}
}

func TestStartWith_CustomSchedule(t *testing.T) {
	s := New()
	var called atomic.Int32

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 */1 * * * *",
			Fn: func(ctx context.Context) error {
				called.Add(1)
				return nil
			},
		},
	}

	overrides := map[string]TaskOverride{
		"test-task": {TaskName: "test-task", Enabled: true, CustomSchedule: "0 */5 * * * *"},
	}

	s.StartWith(tasks, overrides, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	// The cron job uses the overridden schedule, but GetTaskStatuses
	// reflects the schedule stored in taskDefs (which is the original).
	// The task is still enabled and runs on the overridden schedule.
	if !statuses[0].Enabled {
		t.Error("task should be enabled")
	}
}

func TestStartWith_HistoryCleanup(t *testing.T) {
	s := New()
	var cleaned atomic.Int32

	mockHistory := &mockHistoryWriter{
		cleanFn: func(ctx context.Context, maxAge time.Duration) error {
			cleaned.Add(1)
			if maxAge != 7*24*time.Hour {
				t.Errorf("maxAge = %v, want %v", maxAge, 7*24*time.Hour)
			}
			return nil
		},
	}

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 */1 * * * *",
			Fn:       func(ctx context.Context) error { return nil },
		},
	}

	s.StartWith(tasks, nil, mockHistory)
	defer s.Stop()

	if cleaned.Load() != 1 {
		t.Errorf("expected 1 cleanup call, got %d", cleaned.Load())
	}
}

func TestRunTaskNow(t *testing.T) {
	s := New()
	var called atomic.Int32

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 0 1 1 1 *", // never runs naturally
			Fn: func(ctx context.Context) error {
				called.Add(1)
				return nil
			},
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	rec, err := s.RunTaskNow("test-task")
	if err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}
	if called.Load() != 1 {
		t.Errorf("expected 1 call, got %d", called.Load())
	}
	if rec.Status != "success" {
		t.Errorf("status = %q, want %q", rec.Status, "success")
	}
	if rec.DurationMs < 0 {
		t.Error("duration should be non-negative")
	}
	if rec.TaskName != "test-task" {
		t.Errorf("task name = %q, want %q", rec.TaskName, "test-task")
	}
}

func TestRunTaskNow_Error(t *testing.T) {
	s := New()

	tasks := []Task{
		{
			Name:     "fail-task",
			Schedule: "0 0 1 1 1 *",
			Fn: func(ctx context.Context) error {
				return context.DeadlineExceeded
			},
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	rec, err := s.RunTaskNow("fail-task")
	if err == nil {
		t.Fatal("expected error")
	}
	if rec.Status != "error" {
		t.Errorf("status = %q, want %q", rec.Status, "error")
	}
	if rec.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRunTaskNow_NotFound(t *testing.T) {
	s := New()
	s.StartWith([]Task{}, nil, nil)
	defer s.Stop()

	_, err := s.RunTaskNow("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
}

func TestRunTaskNow_RecordsHistory(t *testing.T) {
	s := New()
	var inserted atomic.Int32

	mockHistory := &mockHistoryWriter{
		insertFn: func(ctx context.Context, rec *ExecutionRecord) error {
			inserted.Add(1)
			return nil
		},
	}

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 0 1 1 1 *",
			Fn:       func(ctx context.Context) error { return nil },
		},
	}

	s.StartWith(tasks, nil, mockHistory)
	defer s.Stop()

	_, err := s.RunTaskNow("test-task")
	if err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}
	if inserted.Load() != 1 {
		t.Errorf("expected 1 history insert, got %d", inserted.Load())
	}
}

func TestAddOrUpdateTask(t *testing.T) {
	s := New()

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 0 1 1 1 *",
			Fn:       func(ctx context.Context) error { return nil },
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	err := s.AddOrUpdateTask(Task{
		Name:     "test-task",
		Schedule: "0 */1 * * * *",
		Fn:       func(ctx context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("AddOrUpdateTask: %v", err)
	}

	statuses := s.GetTaskStatuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Enabled {
		t.Error("task should be enabled after update")
	}
}

func TestAddOrUpdateTask_NilFn(t *testing.T) {
	s := New()
	s.StartWith([]Task{}, nil, nil)
	defer s.Stop()

	err := s.AddOrUpdateTask(Task{Name: "bad", Schedule: "0 */1 * * * *", Fn: nil})
	if err == nil {
		t.Fatal("expected error for nil Fn")
	}
}

func TestRemoveTask(t *testing.T) {
	s := New()

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 */1 * * * *",
			Fn:       func(ctx context.Context) error { return nil },
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	if !statuses[0].Enabled {
		t.Fatal("task should be enabled initially")
	}

	s.RemoveTask("test-task")

	statuses = s.GetTaskStatuses()
	if statuses[0].Enabled {
		t.Error("task should be disabled after removal")
	}
}

func TestRemoveTask_NonExistent(t *testing.T) {
	s := New()
	s.StartWith([]Task{}, nil, nil)
	defer s.Stop()

	s.RemoveTask("nonexistent") // should not panic
}

func TestGetTaskStatuses_Timeout(t *testing.T) {
	s := New()

	tasks := []Task{
		{
			Name:     "test-task",
			Schedule: "0 */1 * * * *",
			Timeout:  5 * time.Minute,
			Fn:       func(ctx context.Context) error { return nil },
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	if statuses[0].Timeout != "5m0s" {
		t.Errorf("timeout = %q, want %q", statuses[0].Timeout, "5m0s")
	}
}

func TestGetDefaults(t *testing.T) {
	s := New()

	tasks := []Task{
		{Name: "a", Schedule: "0 */1 * * * *", Fn: func(ctx context.Context) error { return nil }},
		{Name: "b", Schedule: "0 */2 * * * *", Fn: func(ctx context.Context) error { return nil }},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	defaults := s.GetDefaults()
	if len(defaults) != 2 {
		t.Fatalf("expected 2 defaults, got %d", len(defaults))
	}

	// Verify it's a copy - modifying returned slice shouldn't affect internal state
	defaults[0] = Task{Name: "modified"}
	check := s.GetDefaults()
	if check[0].Name == "modified" {
		t.Error("GetDefaults should return a copy")
	}
}

func TestStartWith_InvalidCron(t *testing.T) {
	s := New()

	tasks := []Task{
		{
			Name:     "bad-cron",
			Schedule: "invalid",
			Fn:       func(ctx context.Context) error { return nil },
		},
	}

	s.StartWith(tasks, nil, nil)
	defer s.Stop()

	statuses := s.GetTaskStatuses()
	if statuses[0].Enabled {
		t.Error("task with invalid cron should not be enabled")
	}
}

// mockHistoryWriter implements HistoryWriter for testing.
type mockHistoryWriter struct {
	insertFn func(ctx context.Context, rec *ExecutionRecord) error
	cleanFn  func(ctx context.Context, maxAge time.Duration) error
}

func (m *mockHistoryWriter) InsertHistory(ctx context.Context, rec *ExecutionRecord) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, rec)
	}
	return nil
}

func (m *mockHistoryWriter) CleanOldHistory(ctx context.Context, maxAge time.Duration) error {
	if m.cleanFn != nil {
		return m.cleanFn(ctx, maxAge)
	}
	return nil
}
