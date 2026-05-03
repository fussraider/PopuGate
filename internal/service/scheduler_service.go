package service

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"

	"github.com/fussraider/PopuGate/internal/scheduler"
	"github.com/fussraider/PopuGate/internal/store"
)

// SchedulerService handles scheduler business logic.
type SchedulerService struct {
	store *store.SchedulerStore
	sched *scheduler.Scheduler
}

// NewSchedulerService creates a new SchedulerService.
func NewSchedulerService(store *store.SchedulerStore, sched *scheduler.Scheduler) *SchedulerService {
	return &SchedulerService{store: store, sched: sched}
}

// ListTasks returns all tasks with their current status and last run info.
func (svc *SchedulerService) ListTasks(ctx context.Context) ([]scheduler.TaskStatus, error) {
	statuses := svc.sched.GetTaskStatuses()

	for i := range statuses {
		latest, err := svc.store.GetLatestHistory(ctx, statuses[i].Name)
		if err != nil {
			return nil, fmt.Errorf("get latest history for %s: %w", statuses[i].Name, err)
		}
		if latest != nil {
			statuses[i].LastRun = latest
		}

		ovr, err := svc.store.GetOverride(ctx, statuses[i].Name)
		if err != nil {
			return nil, fmt.Errorf("get override for %s: %w", statuses[i].Name, err)
		}
		if ovr != nil {
			statuses[i].IsOverridden = true
		}
	}

	return statuses, nil
}

// UpdateTask changes a task's enabled state and/or schedule.
func (svc *SchedulerService) UpdateTask(ctx context.Context, name string, enabled *bool, schedule *string) error {
	if !scheduler.KnownTaskNames()[name] {
		return fmt.Errorf("unknown task: %s", name)
	}

	// Validate cron expression if provided
	if schedule != nil && *schedule != "" {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(*schedule); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	}

	// Get existing override or create one from defaults
	ovr, err := svc.store.GetOverride(ctx, name)
	if err != nil {
		return fmt.Errorf("get override: %w", err)
	}
	if ovr == nil {
		ovr = &scheduler.TaskOverride{
			TaskName: name,
			Enabled:  true,
		}
	}

	if enabled != nil {
		ovr.Enabled = *enabled
	}
	if schedule != nil {
		ovr.CustomSchedule = *schedule
	}

	// If fully reverted to defaults, delete the override row
	defaultSchedule := scheduler.DefaultScheduleFor(name)
	if ovr.Enabled && (ovr.CustomSchedule == "" || ovr.CustomSchedule == defaultSchedule) {
		svc.store.DeleteOverride(ctx, name)
	} else {
		if err := svc.store.UpsertOverride(ctx, ovr); err != nil {
			return fmt.Errorf("save override: %w", err)
		}
	}

	// Apply at runtime
	defaults := svc.sched.GetDefaults()
	for _, dt := range defaults {
		if dt.Name != name {
			continue
		}

		if !ovr.Enabled {
			svc.sched.RemoveTask(name)
			return nil
		}

		effectiveSchedule := dt.Schedule
		if ovr.CustomSchedule != "" {
			effectiveSchedule = ovr.CustomSchedule
		}

		task := scheduler.Task{
			Name:     dt.Name,
			Schedule: effectiveSchedule,
			Timeout:  scheduler.DefaultTimeoutFor(dt.Name),
			Fn:       dt.Fn,
		}
		return svc.sched.AddOrUpdateTask(task)
	}

	return nil
}

// RunTaskNow manually triggers a task.
func (svc *SchedulerService) RunTaskNow(ctx context.Context, name string) (*scheduler.ExecutionRecord, error) {
	if !scheduler.KnownTaskNames()[name] {
		return nil, fmt.Errorf("unknown task: %s", name)
	}
	return svc.sched.RunTaskNow(name)
}

// GetHistory returns execution history for a specific task.
func (svc *SchedulerService) GetHistory(ctx context.Context, taskName string, limit, offset int) ([]scheduler.ExecutionRecord, error) {
	return svc.store.ListHistoryByTask(ctx, taskName, limit, offset)
}

// GetAllHistory returns execution history for all tasks.
func (svc *SchedulerService) GetAllHistory(ctx context.Context, limit, offset int) ([]scheduler.ExecutionRecord, error) {
	return svc.store.ListHistory(ctx, limit, offset)
}
