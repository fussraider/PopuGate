-- +migrate Up:
ALTER TABLE instances ADD COLUMN client_mss_bulk INTEGER NOT NULL DEFAULT 0;

-- +migrate Down:
ALTER TABLE instances DROP COLUMN client_mss_bulk;
