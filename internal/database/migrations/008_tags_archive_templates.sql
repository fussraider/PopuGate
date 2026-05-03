-- Tags, archive, and templates for secrets.
ALTER TABLE secrets ADD COLUMN tags TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets ADD COLUMN archived_at INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS secret_templates (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    max_conns   INTEGER NOT NULL DEFAULT 0,
    max_ips     INTEGER NOT NULL DEFAULT 0,
    quota_bytes INTEGER NOT NULL DEFAULT 0,
    expires_days INTEGER NOT NULL DEFAULT 0,
    notes       TEXT    NOT NULL DEFAULT ''
);
