-- +migrate Up:
-- Expose use_middle_proxy as a configurable engine setting (ADR-001).
-- Default 'true' preserves the previous hardcoded behavior (MiddleProxy / ad_tag on).
-- Must be set to 'false' to use shadowsocks upstreams.
INSERT OR IGNORE INTO settings (key, value) VALUES ('use_middle_proxy', 'true');

-- +migrate Down:
DELETE FROM settings WHERE key = 'use_middle_proxy';
