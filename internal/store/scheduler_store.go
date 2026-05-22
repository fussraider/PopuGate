package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fussraider/PopuGate/internal/scheduler"
)

// SchedulerStore handles scheduler persistence.
type SchedulerStore struct {
	db *sql.DB
}

// NewSchedulerStore creates a new SchedulerStore.
func NewSchedulerStore(db *sql.DB) *SchedulerStore {
	return &SchedulerStore{db: db}
}

// GetOverrides returns all task overrides.
func (s *SchedulerStore) GetOverrides(ctx context.Context) ([]scheduler.TaskOverride, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT task_name, enabled, custom_schedule, updated_at
		FROM scheduler_task_overrides
	`)
	if err != nil {
		return nil, fmt.Errorf("get overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []scheduler.TaskOverride
	for rows.Next() {
		var o scheduler.TaskOverride
		var enabled int
		if err := rows.Scan(&o.TaskName, &enabled, &o.CustomSchedule, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan override: %w", err)
		}
		o.Enabled = intToBool(enabled)
		result = append(result, o)
	}
	return result, rows.Err()
}

// GetOverride returns a single task override.
func (s *SchedulerStore) GetOverride(ctx context.Context, taskName string) (*scheduler.TaskOverride, error) {
	var o scheduler.TaskOverride
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT task_name, enabled, custom_schedule, updated_at
		FROM scheduler_task_overrides WHERE task_name = ?
	`, taskName).Scan(&o.TaskName, &enabled, &o.CustomSchedule, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get override %s: %w", taskName, err)
	}
	o.Enabled = intToBool(enabled)
	return &o, nil
}

// UpsertOverride inserts or updates a task override.
func (s *SchedulerStore) UpsertOverride(ctx context.Context, o *scheduler.TaskOverride) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scheduler_task_overrides (task_name, enabled, custom_schedule, updated_at)
		VALUES (?, ?, ?, strftime('%s','now'))
		ON CONFLICT(task_name) DO UPDATE SET
			enabled = excluded.enabled,
			custom_schedule = excluded.custom_schedule,
			updated_at = excluded.updated_at
	`, o.TaskName, boolToInt(o.Enabled), o.CustomSchedule)
	if err != nil {
		return fmt.Errorf("upsert override %s: %w", o.TaskName, err)
	}
	return nil
}

// DeleteOverride removes a task override (revert to default).
func (s *SchedulerStore) DeleteOverride(ctx context.Context, taskName string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM scheduler_task_overrides WHERE task_name = ?`, taskName)
	if err != nil {
		return fmt.Errorf("delete override %s: %w", taskName, err)
	}
	return nil
}

// InsertHistory persists a task execution record.
func (s *SchedulerStore) InsertHistory(ctx context.Context, rec *scheduler.ExecutionRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scheduler_history (task_name, started_at, finished_at, duration_ms, status, error, output)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, rec.TaskName, rec.StartedAt, rec.FinishedAt, rec.DurationMs, rec.Status, rec.Error, rec.Output)
	if err != nil {
		return fmt.Errorf("insert history %s: %w", rec.TaskName, err)
	}
	return nil
}

// ListHistory returns recent execution history for all tasks.
func (s *SchedulerStore) ListHistory(ctx context.Context, limit, offset int) ([]scheduler.ExecutionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_name, started_at, finished_at, duration_ms, status, error, output
		FROM scheduler_history ORDER BY started_at DESC LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanHistory(rows)
}

// ListHistoryByTask returns recent execution history for a specific task.
func (s *SchedulerStore) ListHistoryByTask(ctx context.Context, taskName string, limit, offset int) ([]scheduler.ExecutionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_name, started_at, finished_at, duration_ms, status, error, output
		FROM scheduler_history WHERE task_name = ? ORDER BY started_at DESC LIMIT ? OFFSET ?
	`, taskName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list history for %s: %w", taskName, err)
	}
	defer func() { _ = rows.Close() }()
	return scanHistory(rows)
}

// GetLatestHistory returns the most recent execution record for a task.
func (s *SchedulerStore) GetLatestHistory(ctx context.Context, taskName string) (*scheduler.ExecutionRecord, error) {
	var rec scheduler.ExecutionRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_name, started_at, finished_at, duration_ms, status, error, output
		FROM scheduler_history WHERE task_name = ? ORDER BY started_at DESC LIMIT 1
	`, taskName).Scan(&rec.ID, &rec.TaskName, &rec.StartedAt, &rec.FinishedAt,
		&rec.DurationMs, &rec.Status, &rec.Error, &rec.Output)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest history %s: %w", taskName, err)
	}
	return &rec, nil
}

// CleanOldHistory removes execution records older than maxAge.
func (s *SchedulerStore) CleanOldHistory(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge).Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM scheduler_history WHERE started_at < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("clean old history: %w", err)
	}
	return nil
}

func scanHistory(rows *sql.Rows) ([]scheduler.ExecutionRecord, error) {
	var result []scheduler.ExecutionRecord
	for rows.Next() {
		var rec scheduler.ExecutionRecord
		if err := rows.Scan(&rec.ID, &rec.TaskName, &rec.StartedAt, &rec.FinishedAt,
			&rec.DurationMs, &rec.Status, &rec.Error, &rec.Output); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}
