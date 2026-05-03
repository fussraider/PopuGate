package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/fussraider/PopuGate/pkg/logger"
)

var log = logger.WithScope("scheduler")

const defaultTaskTimeout = 30 * time.Second

// HistoryWriter persists execution records. Implemented by the store layer.
type HistoryWriter interface {
	InsertHistory(ctx context.Context, rec *ExecutionRecord) error
	CleanOldHistory(ctx context.Context, maxAge time.Duration) error
}

// ExecutionRecord is a single task execution result.
type ExecutionRecord struct {
	ID         int64  `json:"id"`
	TaskName   string `json:"task_name"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	DurationMs int64  `json:"duration_ms"`
	Status     string `json:"status"` // "success" or "error"
	Error      string `json:"error"`
	Output     string `json:"output"`
}

// TaskOverride holds per-task configuration from the database.
type TaskOverride struct {
	TaskName       string `json:"task_name"`
	Enabled        bool   `json:"enabled"`
	CustomSchedule string `json:"custom_schedule"` // empty = use default
	UpdatedAt      int64  `json:"updated_at"`
}

// TaskStatus is the runtime status of a task (for the API).
type TaskStatus struct {
	Name              string           `json:"name"`
	DefaultSchedule   string           `json:"default_schedule"`
	EffectiveSchedule string           `json:"effective_schedule"`
	Enabled           bool             `json:"enabled"`
	IsOverridden      bool             `json:"is_overridden"`
	Timeout           string           `json:"timeout"`
	LastRun           *ExecutionRecord `json:"last_run,omitempty"`
}

// Scheduler runs periodic tasks.
type Scheduler struct {
	cron     *cron.Cron
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	entries  map[string]cron.EntryID
	taskDefs map[string]*Task
	defaults []Task
	history  HistoryWriter
}

// New creates a new Scheduler.
func New() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:     cron.New(cron.WithSeconds()),
		ctx:      ctx,
		cancel:   cancel,
		entries:  make(map[string]cron.EntryID),
		taskDefs: make(map[string]*Task),
	}
}

// Task is a periodic task definition.
type Task struct {
	Name     string
	Schedule string        // cron expression
	Timeout  time.Duration // per-task timeout (0 = defaultTaskTimeout)
	Fn       func(ctx context.Context) error
}

// StartWith begins all scheduled tasks with DB overrides and execution tracking.
func (s *Scheduler) StartWith(tasks []Task, overrides map[string]TaskOverride, history HistoryWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.defaults = make([]Task, len(tasks))
	s.history = history

	for i := range tasks {
		task := tasks[i]
		s.defaults[i] = task

		if task.Fn == nil {
			log.Warnf("skip scheduling %s: no function provided", task.Name)
			continue
		}

		s.taskDefs[task.Name] = &task

		// Apply overrides
		schedule := task.Schedule
		if ovr, ok := overrides[task.Name]; ok {
			if !ovr.Enabled {
				log.Infof("disabled by override: %s", task.Name)
				continue
			}
			if ovr.CustomSchedule != "" {
				schedule = ovr.CustomSchedule
			}
		}

		s.addTaskToCron(&task, schedule)
	}

	s.cron.Start()

	// Clean old history on startup
	if s.history != nil {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.history.CleanOldHistory(cleanCtx, 7*24*time.Hour); err != nil {
			log.Warnf("history cleanup on start: %v", err)
		}
	}
}

// Stop gracefully stops the scheduler and cancels running task contexts.
func (s *Scheduler) Stop() {
	s.cancel()
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// AddOrUpdateTask removes an existing cron entry (if any) and adds a new one.
func (s *Scheduler) AddOrUpdateTask(task Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task.Fn == nil {
		return fmt.Errorf("task %s has no function", task.Name)
	}

	s.taskDefs[task.Name] = &task

	// Remove existing entry
	if id, ok := s.entries[task.Name]; ok {
		s.cron.Remove(id)
		delete(s.entries, task.Name)
	}

	return s.addTaskToCron(&task, task.Schedule)
}

// RemoveTask removes a task's cron entry by name.
func (s *Scheduler) RemoveTask(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.entries[name]; ok {
		s.cron.Remove(id)
		delete(s.entries, name)
	}
}

// GetTaskStatuses returns the current status of all known tasks.
func (s *Scheduler) GetTaskStatuses() []TaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make([]TaskStatus, 0, len(s.defaults))
	for _, dt := range s.defaults {
		effectiveSchedule := dt.Schedule

		// Check if taskDefs has a modified schedule (from runtime override)
		if td, ok := s.taskDefs[dt.Name]; ok {
			effectiveSchedule = td.Schedule
		}

		// Enabled = has a cron entry
		_, enabled := s.entries[dt.Name]

		isOverridden := effectiveSchedule != dt.Schedule

		timeout := defaultTaskTimeout.String()
		if dt.Timeout > 0 {
			timeout = dt.Timeout.String()
		}

		statuses = append(statuses, TaskStatus{
			Name:              dt.Name,
			DefaultSchedule:   dt.Schedule,
			EffectiveSchedule: effectiveSchedule,
			Enabled:           enabled,
			IsOverridden:      isOverridden,
			Timeout:           timeout,
		})
	}
	return statuses
}

// GetDefaults returns the original default task definitions.
func (s *Scheduler) GetDefaults() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, len(s.defaults))
	copy(out, s.defaults)
	return out
}

// RunTaskNow manually triggers a task by name.
func (s *Scheduler) RunTaskNow(name string) (*ExecutionRecord, error) {
	s.mu.Lock()
	task, ok := s.taskDefs[name]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("task %s not found", name)
	}
	fn := task.Fn
	timeout := task.Timeout
	if timeout == 0 {
		timeout = defaultTaskTimeout
	}
	s.mu.Unlock()

	start := time.Now()
	rec := &ExecutionRecord{
		TaskName:  name,
		StartedAt: start.Unix(),
	}

	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()

	err := fn(ctx)

	rec.FinishedAt = time.Now().Unix()
	rec.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		rec.Status = "error"
		rec.Error = err.Error()
	} else {
		rec.Status = "success"
	}

	if s.history != nil {
		histCtx, histCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer histCancel()
		if histErr := s.history.InsertHistory(histCtx, rec); histErr != nil {
			log.Errorf("failed to write history for %s: %v", name, histErr)
		}
	}

	return rec, err
}

func (s *Scheduler) addTaskToCron(task *Task, schedule string) error {
	wrapped := s.wrapTask(task)
	id, err := s.cron.AddFunc(schedule, wrapped)
	if err != nil {
		log.Errorf("failed to schedule %s: %v", task.Name, err)
		return err
	}
	s.entries[task.Name] = id
	log.Infof("scheduled: %s (%s)", task.Name, schedule)
	return nil
}

func (s *Scheduler) wrapTask(task *Task) func() {
	return func() {
		start := time.Now()
		rec := &ExecutionRecord{
			TaskName:  task.Name,
			StartedAt: start.Unix(),
		}

		timeout := task.Timeout
		if timeout == 0 {
			timeout = defaultTaskTimeout
		}
		ctx, cancel := context.WithTimeout(s.ctx, timeout)
		defer cancel()

		err := task.Fn(ctx)

		rec.FinishedAt = time.Now().Unix()
		rec.DurationMs = time.Since(start).Milliseconds()
		if err != nil {
			rec.Status = "error"
			rec.Error = err.Error()
			log.Errorf("%s error: %v", task.Name, err)
		} else {
			rec.Status = "success"
		}

		if s.history != nil {
			histCtx, histCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer histCancel()
			if histErr := s.history.InsertHistory(histCtx, rec); histErr != nil {
				log.Errorf("failed to write history for %s: %v", task.Name, histErr)
			}
		}
	}
}

// DefaultTasks returns the standard set of periodic tasks.
// Each task's Fn should be set by the caller with access to services.
func DefaultTasks() []Task {
	return []Task{
		{Name: "traffic-flush", Schedule: "0 */1 * * * *"},                              // every minute
		{Name: "quota-check", Schedule: "0 */5 * * * *"},                                // every 5 min
		{Name: "expiry-check", Schedule: "0 */5 * * * *"},                               // every 5 min
		{Name: "health-check", Schedule: "0 */5 * * * *"},                               // every 5 min
		{Name: "telegram-report", Schedule: "0 0 */6 * * *"},                            // every 6 hours
		{Name: "replication-sync", Schedule: "0 */1 * * * *", Timeout: 3 * time.Minute}, // SSH transfers can be slow
		{Name: "update-check", Schedule: "0 0 */6 * * *"},                               // every 6 hours
		{Name: "telemt-check", Schedule: "0 0 */6 * * *", Timeout: 30 * time.Second},    // every 6 hours
		{Name: "token-cleanup", Schedule: "0 0 */1 * * *"},                              // every hour
		{Name: "daily-backup", Schedule: "0 0 3 * * *", Timeout: 5 * time.Minute},       // daily at 3:00
		{Name: "backup-cleanup", Schedule: "0 30 3 * * *"},                              // daily at 3:30
		{Name: "history-cleanup", Schedule: "0 0 4 * * *"},                              // daily at 4:00
		{Name: "quota-reset", Schedule: "0 0 0 1 * *", Timeout: 2 * time.Minute},        // monthly on 1st
		{Name: "auto-rotate", Schedule: "0 0 4 * * *"},                                  // daily at 4:00
	}
}

// KnownTaskNames returns the set of valid task names.
func KnownTaskNames() map[string]bool {
	names := make(map[string]bool)
	for _, t := range DefaultTasks() {
		names[t.Name] = true
	}
	return names
}

// DefaultScheduleFor returns the default schedule for a task name, or empty string.
func DefaultScheduleFor(name string) string {
	for _, t := range DefaultTasks() {
		if t.Name == name {
			return t.Schedule
		}
	}
	return ""
}

// DefaultTimeoutFor returns the default timeout for a task name.
func DefaultTimeoutFor(name string) time.Duration {
	for _, t := range DefaultTasks() {
		if t.Name == name {
			if t.Timeout > 0 {
				return t.Timeout
			}
			return defaultTaskTimeout
		}
	}
	return defaultTaskTimeout
}
