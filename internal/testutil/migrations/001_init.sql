-- PopuGate SQLite Schema v1

-- Application settings (key-value)
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO settings (key, value) VALUES
    ('proxy_port', '443'),
    ('proxy_metrics_port', '9090'),
    ('proxy_domain', 'cloudflare.com'),
    ('proxy_concurrency', '8192'),
    ('proxy_cpus', ''),
    ('proxy_memory', ''),
    ('custom_ip', ''),
    ('fake_cert_len', '2048'),
    ('proxy_protocol', 'false'),
    ('proxy_protocol_trusted_cidrs', ''),
    ('ad_tag', ''),
    ('geoblock_mode', 'blacklist'),
    ('blocklist_countries', ''),
    ('masking_enabled', 'true'),
    ('masking_host', ''),
    ('masking_port', '443'),
    ('unknown_sni_action', 'mask'),
    ('telegram_enabled', 'false'),
    ('telegram_bot_token', ''),
    ('telegram_chat_id', ''),
    ('telegram_interval', '6'),
    ('telegram_alerts_enabled', 'true'),
    ('telegram_server_label', 'PopuGate'),
    ('auto_update_enabled', 'true'),
    ('replication_enabled', 'false'),
    ('replication_role', 'standalone'),
    ('replication_sync_interval', '60'),
    ('replication_ssh_port', '22'),
    ('replication_ssh_user', 'root'),
    ('replication_delete_extra', 'true'),
    ('replication_ssh_key_path', ''),
    ('replication_exclude', 'relay_stats,backups,connection.log,.ssh,settings.db,replication'),
    ('replication_restart_on_change', 'true'),
    ('replication_log', ''),
    ('auth_password_hash', ''),
    ('jwt_secret', '');

-- MTProto secrets
CREATE TABLE IF NOT EXISTS secrets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    label       TEXT    NOT NULL UNIQUE,
    secret_key  TEXT    NOT NULL,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    enabled     INTEGER NOT NULL DEFAULT 1,
    max_conns   INTEGER NOT NULL DEFAULT 0,
    max_ips     INTEGER NOT NULL DEFAULT 0,
    quota_bytes INTEGER NOT NULL DEFAULT 0,
    expires_at  TEXT    NOT NULL DEFAULT '0',
    notes       TEXT    NOT NULL DEFAULT ''
);

-- Proxy upstreams
CREATE TABLE IF NOT EXISTS upstreams (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT    NOT NULL UNIQUE,
    type     TEXT    NOT NULL DEFAULT 'direct' CHECK(type IN ('direct','socks5','socks4')),
    address  TEXT    NOT NULL DEFAULT '',
    username TEXT    NOT NULL DEFAULT '',
    password TEXT    NOT NULL DEFAULT '',
    weight   INTEGER NOT NULL DEFAULT 10 CHECK(weight >= 1 AND weight <= 100),
    iface    TEXT    NOT NULL DEFAULT '',
    enabled  INTEGER NOT NULL DEFAULT 1
);

-- Multi-port instances
CREATE TABLE IF NOT EXISTS instances (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    port         INTEGER NOT NULL UNIQUE CHECK(port >= 1 AND port <= 65535),
    metrics_port INTEGER NOT NULL DEFAULT 9091,
    enabled      INTEGER NOT NULL DEFAULT 1,
    label        TEXT    NOT NULL DEFAULT ''
);

-- Replication slaves
CREATE TABLE IF NOT EXISTS slaves (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    host        TEXT    NOT NULL UNIQUE,
    port        INTEGER NOT NULL DEFAULT 22 CHECK(port >= 1 AND port <= 65535),
    label       TEXT    NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    last_sync   INTEGER NOT NULL DEFAULT 0,
    status      TEXT    NOT NULL DEFAULT 'unknown' CHECK(status IN ('ok','error','unknown'))
);

-- Cumulative traffic (global singleton)
CREATE TABLE IF NOT EXISTS traffic_global (
    id       INTEGER PRIMARY KEY CHECK(id = 1),
    bytes_in  INTEGER NOT NULL DEFAULT 0,
    bytes_out INTEGER NOT NULL DEFAULT 0,
    snap_in   INTEGER NOT NULL DEFAULT 0,
    snap_out  INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO traffic_global (id) VALUES (1);

-- Cumulative traffic (per-user)
CREATE TABLE IF NOT EXISTS traffic_user (
    label    TEXT PRIMARY KEY,
    bytes_in  INTEGER NOT NULL DEFAULT 0,
    bytes_out INTEGER NOT NULL DEFAULT 0,
    snap_in   INTEGER NOT NULL DEFAULT 0,
    snap_out  INTEGER NOT NULL DEFAULT 0
);

-- Geo-block CIDR cache
CREATE TABLE IF NOT EXISTS geoblock_cache (
    country_code  TEXT PRIMARY KEY,
    file_path     TEXT NOT NULL,
    downloaded_at INTEGER NOT NULL DEFAULT 0
);

-- Quota alert dedup
CREATE TABLE IF NOT EXISTS quota_alerts (
    label   TEXT NOT NULL,
    percent INTEGER NOT NULL,
    PRIMARY KEY (label, percent)
);

-- Expiry alert dedup
CREATE TABLE IF NOT EXISTS expiry_alerts (
    label TEXT NOT NULL,
    date  TEXT NOT NULL,
    kind  TEXT NOT NULL DEFAULT 'warning',
    PRIMARY KEY (label, date, kind)
);

-- JWT token blocklist
CREATE TABLE IF NOT EXISTS token_blocklist (
    jti        TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_secrets_label    ON secrets(label);
CREATE INDEX IF NOT EXISTS idx_secrets_enabled  ON secrets(enabled);
CREATE INDEX IF NOT EXISTS idx_upstreams_name   ON upstreams(name);
CREATE INDEX IF NOT EXISTS idx_instances_port   ON instances(port);
CREATE INDEX IF NOT EXISTS idx_slaves_host      ON slaves(host);
CREATE INDEX IF NOT EXISTS idx_traffic_user_lbl ON traffic_user(label);
CREATE INDEX IF NOT EXISTS idx_token_blocklist  ON token_blocklist(expires_at);
