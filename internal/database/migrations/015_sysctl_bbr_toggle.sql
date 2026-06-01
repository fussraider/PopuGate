-- Add sysctl network optimizations and rollback backup settings to the key-value settings table.
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('sysctl_optimizations_enabled', 'false'),
    ('original_qdisc', ''),
    ('original_congestion_control', ''),
    ('original_fastopen', '');
