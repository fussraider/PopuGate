-- Traffic history: timestamped snapshots for charting.
CREATE TABLE IF NOT EXISTS traffic_history (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    bytes_in  INTEGER NOT NULL DEFAULT 0,
    bytes_out INTEGER NOT NULL DEFAULT 0,
    label     TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_traffic_history_ts_lbl ON traffic_history(timestamp, label);
