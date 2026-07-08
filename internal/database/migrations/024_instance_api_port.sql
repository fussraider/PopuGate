-- +migrate Up:
ALTER TABLE instances ADD COLUMN api_port INTEGER NOT NULL DEFAULT 0;

-- +migrate Down:
ALTER TABLE instances DROP COLUMN api_port;
