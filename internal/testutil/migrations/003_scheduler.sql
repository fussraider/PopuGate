-- Scheduler management tables

-- Per-task configuration overrides.
-- Only rows for tasks that have been explicitly overridden need to exist.
-- Absence of a row means: use the default from DefaultTasks() (enabled=true, default schedule).
CREATE TABLE IF NOT EXISTS scheduler_task_overrides (
    task_name       TEXT PRIMARY KEY,
    enabled         INTEGER NOT NULL DEFAULT 1,
    custom_schedule TEXT    NOT NULL DEFAULT '',
    updated_at      INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Task execution history.
CREATE TABLE IF NOT EXISTS scheduler_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_name   TEXT    NOT NULL,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    status      TEXT    NOT NULL DEFAULT 'success' CHECK(status IN ('success','error')),
    error       TEXT    NOT NULL DEFAULT '',
    output      TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_scheduler_history_task    ON scheduler_history(task_name);
CREATE INDEX IF NOT EXISTS idx_scheduler_history_started ON scheduler_history(started_at);
