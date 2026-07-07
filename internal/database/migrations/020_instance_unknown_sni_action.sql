-- +migrate Up:
ALTER TABLE instances ADD COLUMN unknown_sni_action TEXT NOT NULL DEFAULT 'mask';

-- +migrate Down:
ALTER TABLE instances DROP COLUMN unknown_sni_action;
