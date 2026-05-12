-- 010_instance_per_proxy.sql
-- Per-instance proxy configuration: each instance is a fully independent proxy

-- Add new columns to instances table
ALTER TABLE instances ADD COLUMN tls_domain TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN tls_domains TEXT NOT NULL DEFAULT '[]';
ALTER TABLE instances ADD COLUMN fake_tls INTEGER NOT NULL DEFAULT 1;
ALTER TABLE instances ADD COLUMN mask_host TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN mask_port INTEGER NOT NULL DEFAULT 443;
ALTER TABLE instances ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';

-- Migrate global proxy settings to the existing (default) instance
UPDATE instances SET
    tls_domain = COALESCE((SELECT value FROM settings WHERE key = 'proxy_domain'), 'cloudflare.com'),
    fake_tls = COALESCE(
        (SELECT CASE WHEN value = 'true' THEN 1 ELSE 0 END FROM settings WHERE key = 'masking_enabled'),
        1
    ),
    mask_host = COALESCE((SELECT value FROM settings WHERE key = 'masking_host'), ''),
    mask_port = COALESCE(
        (SELECT CAST(value AS INTEGER) FROM settings WHERE key = 'masking_port'),
        443
    );

-- Recreate traffic_user with instance_id for per-instance tracking
CREATE TABLE IF NOT EXISTS traffic_user_new (
    label       TEXT    NOT NULL,
    instance_id INTEGER NOT NULL DEFAULT 0,
    bytes_in    INTEGER NOT NULL DEFAULT 0,
    bytes_out   INTEGER NOT NULL DEFAULT 0,
    snap_in     INTEGER NOT NULL DEFAULT 0,
    snap_out    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (label, instance_id)
);

-- Copy existing traffic data, associating it with the first instance
INSERT OR IGNORE INTO traffic_user_new (label, instance_id, bytes_in, bytes_out, snap_in, snap_out)
SELECT label, COALESCE((SELECT MIN(id) FROM instances), 0), bytes_in, bytes_out, snap_in, snap_out
FROM traffic_user;

-- Swap tables
DROP TABLE traffic_user;
ALTER TABLE traffic_user_new RENAME TO traffic_user;

-- Recreate index
CREATE INDEX IF NOT EXISTS idx_traffic_user_lbl ON traffic_user(label);
