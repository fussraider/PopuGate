-- 013_instance_antiblock.sql
-- Anti-blocking: per-instance TCPMSS fragmentation and TLS fronting content

ALTER TABLE instances ADD COLUMN tcp_mss_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE instances ADD COLUMN tcp_mss INTEGER NOT NULL DEFAULT 88;
ALTER TABLE instances ADD COLUMN tls_fronting INTEGER NOT NULL DEFAULT 0;
