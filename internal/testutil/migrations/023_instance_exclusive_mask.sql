-- +migrate Up:
ALTER TABLE instances ADD COLUMN exclusive_mask TEXT NOT NULL DEFAULT '';

-- +migrate Down:
ALTER TABLE instances DROP COLUMN exclusive_mask;
