-- Add connections count to traffic history for sparkline charts.
ALTER TABLE traffic_history ADD COLUMN connections INTEGER NOT NULL DEFAULT 0;
