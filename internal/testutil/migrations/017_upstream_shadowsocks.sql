-- +migrate Up:
-- Add shadowsocks upstream support: new `url` column + relax the `type` CHECK
-- constraint to allow 'shadowsocks'. SQLite cannot ALTER a CHECK constraint, so the
-- table is rebuilt (create-copy-drop-rename) preserving every existing column.
-- The whole migration runs inside one transaction (see internal/database/db.go), so a
-- failure rolls back cleanly with no orphaned upstreams_new table.

CREATE TABLE upstreams_new (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT    NOT NULL UNIQUE,
    type     TEXT    NOT NULL DEFAULT 'direct' CHECK(type IN ('direct','socks5','socks4','shadowsocks')),
    address  TEXT    NOT NULL DEFAULT '',
    username TEXT    NOT NULL DEFAULT '',
    password TEXT    NOT NULL DEFAULT '',
    weight   INTEGER NOT NULL DEFAULT 10 CHECK(weight >= 1 AND weight <= 100),
    iface    TEXT    NOT NULL DEFAULT '',
    enabled  INTEGER NOT NULL DEFAULT 1,
    last_check_at    INTEGER DEFAULT 0,
    last_check_ok    INTEGER,
    latency_ms       INTEGER DEFAULT 0,
    last_error       TEXT    DEFAULT '',
    fail_count       INTEGER DEFAULT 0,
    auto_disabled    INTEGER DEFAULT 0,
    auto_disabled_at INTEGER DEFAULT 0,
    url      TEXT    NOT NULL DEFAULT ''
);

INSERT INTO upstreams_new (id, name, type, address, username, password, weight, iface, enabled,
                           last_check_at, last_check_ok, latency_ms, last_error, fail_count,
                           auto_disabled, auto_disabled_at)
    SELECT id, name, type, address, username, password, weight, iface, enabled,
           last_check_at, last_check_ok, latency_ms, last_error, fail_count,
           auto_disabled, auto_disabled_at
    FROM upstreams;

DROP TABLE upstreams;

ALTER TABLE upstreams_new RENAME TO upstreams;

-- +migrate Down:
-- Rebuild back to the pre-shadowsocks schema (3-type CHECK, no `url` column).
-- NOTE: this fails if any rows with type='shadowsocks' remain — delete them first.

CREATE TABLE upstreams_old (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT    NOT NULL UNIQUE,
    type     TEXT    NOT NULL DEFAULT 'direct' CHECK(type IN ('direct','socks5','socks4')),
    address  TEXT    NOT NULL DEFAULT '',
    username TEXT    NOT NULL DEFAULT '',
    password TEXT    NOT NULL DEFAULT '',
    weight   INTEGER NOT NULL DEFAULT 10 CHECK(weight >= 1 AND weight <= 100),
    iface    TEXT    NOT NULL DEFAULT '',
    enabled  INTEGER NOT NULL DEFAULT 1,
    last_check_at    INTEGER DEFAULT 0,
    last_check_ok    INTEGER,
    latency_ms       INTEGER DEFAULT 0,
    last_error       TEXT    DEFAULT '',
    fail_count       INTEGER DEFAULT 0,
    auto_disabled    INTEGER DEFAULT 0,
    auto_disabled_at INTEGER DEFAULT 0
);

INSERT INTO upstreams_old (id, name, type, address, username, password, weight, iface, enabled,
                           last_check_at, last_check_ok, latency_ms, last_error, fail_count,
                           auto_disabled, auto_disabled_at)
    SELECT id, name, type, address, username, password, weight, iface, enabled,
           last_check_at, last_check_ok, latency_ms, last_error, fail_count,
           auto_disabled, auto_disabled_at
    FROM upstreams;

DROP TABLE upstreams;

ALTER TABLE upstreams_old RENAME TO upstreams;
