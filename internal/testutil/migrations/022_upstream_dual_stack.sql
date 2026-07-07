-- +migrate Up:
ALTER TABLE upstreams ADD COLUMN ipv4 INTEGER;
ALTER TABLE upstreams ADD COLUMN ipv6 INTEGER;
ALTER TABLE upstreams ADD COLUMN prefer INTEGER NOT NULL DEFAULT 0;
ALTER TABLE upstreams ADD COLUMN bindtodevice TEXT NOT NULL DEFAULT '';

-- +migrate Down:
ALTER TABLE upstreams DROP COLUMN ipv4;
ALTER TABLE upstreams DROP COLUMN ipv6;
ALTER TABLE upstreams DROP COLUMN prefer;
ALTER TABLE upstreams DROP COLUMN bindtodevice;
