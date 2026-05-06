-- +migrate Up:
ALTER TABLE upstreams ADD COLUMN last_check_at INTEGER DEFAULT 0;
ALTER TABLE upstreams ADD COLUMN last_check_ok INTEGER;
ALTER TABLE upstreams ADD COLUMN latency_ms INTEGER DEFAULT 0;
ALTER TABLE upstreams ADD COLUMN last_error TEXT DEFAULT '';
ALTER TABLE upstreams ADD COLUMN fail_count INTEGER DEFAULT 0;

-- +migrate Down:
ALTER TABLE upstreams DROP COLUMN last_check_at;
ALTER TABLE upstreams DROP COLUMN last_check_ok;
ALTER TABLE upstreams DROP COLUMN latency_ms;
ALTER TABLE upstreams DROP COLUMN last_error;
ALTER TABLE upstreams DROP COLUMN fail_count;
