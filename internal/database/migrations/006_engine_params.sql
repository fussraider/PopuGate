-- Engine parameter additions for telemt v3.4.x

-- Mask relay: max bytes per direction on mask relay connection (0 = engine default)
INSERT OR IGNORE INTO settings (key, value) VALUES ('mask_relay_max_bytes', '0');

-- Custom Telegram infrastructure URLs for restricted regions
INSERT OR IGNORE INTO settings (key, value) VALUES ('proxy_secret_url', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('proxy_config_v4_url', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('proxy_config_v6_url', '');

-- Secret auto-rotate: rotate secrets older than N days (0 = disabled)
INSERT OR IGNORE INTO settings (key, value) VALUES ('secret_auto_rotate_days', '0');

-- Maintenance mode: reject new proxy connections (existing stay alive)
INSERT OR IGNORE INTO settings (key, value) VALUES ('maintenance_mode', 'false');
