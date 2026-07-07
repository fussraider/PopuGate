-- +migrate Up:
ALTER TABLE secrets ADD COLUMN rate_limit_up_bps INTEGER NOT NULL DEFAULT 0;
ALTER TABLE secrets ADD COLUMN rate_limit_down_bps INTEGER NOT NULL DEFAULT 0;

-- +migrate Down:
ALTER TABLE secrets DROP COLUMN rate_limit_up_bps;
ALTER TABLE secrets DROP COLUMN rate_limit_down_bps;
