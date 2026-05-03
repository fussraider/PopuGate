-- telemt engine configuration (version, commit, repo)
-- Empty values mean "use default / env fallback".
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('telemt_version', ''),
    ('telemt_commit', ''),
    ('telemt_repo', '');

-- Cache for latest available telemt release (updated by scheduler)
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('telemt_latest_version', ''),
    ('telemt_latest_commit', ''),
    ('telemt_latest_checked', '');

-- Update progress tracking
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('telemt_updating', 'false'),
    ('telemt_updating_to', '');

-- Releases list cache (JSON blob)
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('telemt_releases_cache', '');
