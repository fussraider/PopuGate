-- +migrate Up:
ALTER TABLE upstreams ADD COLUMN auto_disabled INTEGER DEFAULT 0;
ALTER TABLE upstreams ADD COLUMN auto_disabled_at INTEGER DEFAULT 0;

-- +migrate Down:
ALTER TABLE upstreams DROP COLUMN auto_disabled;
ALTER TABLE upstreams DROP COLUMN auto_disabled_at;
