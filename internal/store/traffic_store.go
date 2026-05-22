package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// TrafficStore handles cumulative traffic persistence.
type TrafficStore struct {
	db *sql.DB
}

// NewTrafficStore creates a new TrafficStore.
func NewTrafficStore(db *sql.DB) *TrafficStore {
	return &TrafficStore{db: db}
}

// GetGlobal returns cumulative global traffic.
func (s *TrafficStore) GetGlobal(ctx context.Context) (*model.TrafficSnapshot, error) {
	var t model.TrafficSnapshot
	err := s.db.QueryRowContext(ctx, `
		SELECT bytes_in, bytes_out, snap_in, snap_out FROM traffic_global WHERE id = 1
	`).Scan(&t.BytesIn, &t.BytesOut, &t.SnapIn, &t.SnapOut)
	if err != nil {
		return nil, fmt.Errorf("get global traffic: %w", err)
	}
	return &t, nil
}

// UpdateGlobal adds deltas to global traffic and updates the snapshot.
func (s *TrafficStore) UpdateGlobal(ctx context.Context, deltaIn, deltaOut, snapIn, snapOut int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE traffic_global SET
			bytes_in = bytes_in + ?,
			bytes_out = bytes_out + ?,
			snap_in = ?,
			snap_out = ?
		WHERE id = 1
	`, deltaIn, deltaOut, snapIn, snapOut)
	return err
}

// ResetGlobal clears global traffic counters.
func (s *TrafficStore) ResetGlobal(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE traffic_global SET bytes_in = 0, bytes_out = 0, snap_in = 0, snap_out = 0 WHERE id = 1
	`)
	return err
}

// ListUserTraffic returns per-user traffic summed across all instances.
func (s *TrafficStore) ListUserTraffic(ctx context.Context) ([]model.UserTraffic, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT label, SUM(bytes_in), SUM(bytes_out) FROM traffic_user GROUP BY label ORDER BY label
	`)
	if err != nil {
		return nil, fmt.Errorf("list user traffic: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := make([]model.UserTraffic, 0)
	for rows.Next() {
		var u model.UserTraffic
		if err := rows.Scan(&u.Label, &u.BytesIn, &u.BytesOut); err != nil {
			return nil, fmt.Errorf("scan user traffic: %w", err)
		}
		stats = append(stats, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user traffic: %w", err)
	}
	return stats, nil
}

// GetUserTraffic returns total traffic for a specific user across all instances.
func (s *TrafficStore) GetUserTraffic(ctx context.Context, label string) (*model.UserTraffic, error) {
	var u model.UserTraffic
	err := s.db.QueryRowContext(ctx, `
		SELECT label, COALESCE(SUM(bytes_in), 0), COALESCE(SUM(bytes_out), 0)
		FROM traffic_user WHERE label = ? GROUP BY label
	`, label).Scan(&u.Label, &u.BytesIn, &u.BytesOut)
	if err == sql.ErrNoRows {
		return &model.UserTraffic{Label: label}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user traffic %s: %w", label, err)
	}
	return &u, nil
}

// GetUserSnapshot returns the raw Prometheus snapshot for a user on a specific instance.
func (s *TrafficStore) GetUserSnapshot(ctx context.Context, label string, instanceID int64) (snapIn, snapOut int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT snap_in, snap_out FROM traffic_user WHERE label = ? AND instance_id = ?
	`, label, instanceID).Scan(&snapIn, &snapOut)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

// GetAllUserSnapshots returns all user snapshots for a specific instance in a single query.
func (s *TrafficStore) GetAllUserSnapshots(ctx context.Context, instanceID int64) (map[string][2]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT label, snap_in, snap_out FROM traffic_user WHERE instance_id = ?`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get user snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string][2]int64)
	for rows.Next() {
		var label string
		var snapIn, snapOut int64
		if err := rows.Scan(&label, &snapIn, &snapOut); err != nil {
			return nil, fmt.Errorf("scan user snapshot: %w", err)
		}
		result[label] = [2]int64{snapIn, snapOut}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user snapshots: %w", err)
	}
	return result, nil
}

// UpdateUserTraffic adds deltas to a user's traffic for a specific instance.
func (s *TrafficStore) UpdateUserTraffic(ctx context.Context, label string, instanceID int64, deltaIn, deltaOut, snapIn, snapOut int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO traffic_user (label, instance_id, bytes_in, bytes_out, snap_in, snap_out)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(label, instance_id) DO UPDATE SET
			bytes_in = bytes_in + excluded.bytes_in,
			bytes_out = bytes_out + excluded.bytes_out,
			snap_in = excluded.snap_in,
			snap_out = excluded.snap_out
	`, label, instanceID, deltaIn, deltaOut, snapIn, snapOut)
	return err
}

// FlushTraffic persists global and per-user traffic deltas in a single transaction.
func (s *TrafficStore) FlushTraffic(ctx context.Context, global model.TrafficSnapshot, users map[string]model.TrafficSnapshot, instanceID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin flush tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE traffic_global SET
			bytes_in = bytes_in + ?, bytes_out = bytes_out + ?,
			snap_in = ?, snap_out = ?
		WHERE id = 1
	`, global.BytesIn, global.BytesOut, global.SnapIn, global.SnapOut); err != nil {
		return fmt.Errorf("flush global: %w", err)
	}

	if err := s.flushUsers(tx, ctx, users, instanceID); err != nil {
		return err
	}

	return tx.Commit()
}

// FlushInstanceTraffic persists per-user traffic for one instance without touching global counters.
func (s *TrafficStore) FlushInstanceTraffic(ctx context.Context, users map[string]model.TrafficSnapshot, instanceID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin instance flush tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.flushUsers(tx, ctx, users, instanceID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *TrafficStore) flushUsers(tx *sql.Tx, ctx context.Context, users map[string]model.TrafficSnapshot, instanceID int64) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO traffic_user (label, instance_id, bytes_in, bytes_out, snap_in, snap_out)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(label, instance_id) DO UPDATE SET
			bytes_in = bytes_in + excluded.bytes_in,
			bytes_out = bytes_out + excluded.bytes_out,
			snap_in = excluded.snap_in,
			snap_out = excluded.snap_out
	`)
	if err != nil {
		return fmt.Errorf("prepare user flush: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for label, snap := range users {
		if _, err := stmt.ExecContext(ctx, label, instanceID, snap.BytesIn, snap.BytesOut, snap.SnapIn, snap.SnapOut); err != nil {
			return fmt.Errorf("flush user %s instance %d: %w", label, instanceID, err)
		}
	}
	return nil
}

// InsertHistoryBatch persists traffic deltas as history records in a single transaction.
func (s *TrafficStore) InsertHistoryBatch(ctx context.Context, ts int64, globalIn, globalOut int64, users map[string][2]int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin history tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO traffic_history (timestamp, bytes_in, bytes_out, label) VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare history insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	if globalIn > 0 || globalOut > 0 {
		if _, err := stmt.ExecContext(ctx, ts, globalIn, globalOut, ""); err != nil {
			return fmt.Errorf("insert global history: %w", err)
		}
	}

	for label, deltas := range users {
		if deltas[0] > 0 || deltas[1] > 0 {
			if _, err := stmt.ExecContext(ctx, ts, deltas[0], deltas[1], label); err != nil {
				return fmt.Errorf("insert user history %s: %w", label, err)
			}
		}
	}

	return tx.Commit()
}

// GetHistory returns traffic history records for the given time range and label.
func (s *TrafficStore) GetHistory(ctx context.Context, start, end int64, label string) ([]model.TrafficHistoryRecord, error) {
	query := `SELECT timestamp, bytes_in, bytes_out FROM traffic_history WHERE timestamp >= ? AND timestamp <= ? AND label = ? ORDER BY timestamp ASC`
	rows, err := s.db.QueryContext(ctx, query, start, end, label)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []model.TrafficHistoryRecord
	for rows.Next() {
		var r model.TrafficHistoryRecord
		if err := rows.Scan(&r.Timestamp, &r.BytesIn, &r.BytesOut); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetAggregatedHistory returns traffic history aggregated by hour or day.
func (s *TrafficStore) GetAggregatedHistory(ctx context.Context, start, end int64, label string, groupSeconds int64) ([]model.TrafficHistoryRecord, error) {
	if groupSeconds <= 0 {
		return nil, fmt.Errorf("groupSeconds must be positive, got %d", groupSeconds)
	}
	query := `SELECT (timestamp / ?) * ? AS ts, SUM(bytes_in), SUM(bytes_out) FROM traffic_history WHERE timestamp >= ? AND timestamp <= ? AND label = ? GROUP BY ts ORDER BY ts ASC`
	rows, err := s.db.QueryContext(ctx, query, groupSeconds, groupSeconds, start, end, label)
	if err != nil {
		return nil, fmt.Errorf("get aggregated history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []model.TrafficHistoryRecord
	for rows.Next() {
		var r model.TrafficHistoryRecord
		if err := rows.Scan(&r.Timestamp, &r.BytesIn, &r.BytesOut); err != nil {
			return nil, fmt.Errorf("scan aggregated history: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// CleanOldHistory deletes traffic history records older than maxAge.
func (s *TrafficStore) CleanOldHistory(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge).Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM traffic_history WHERE timestamp < ?`, cutoff)
	return err
}

// ResetTraffic clears traffic for a specific user.
func (s *TrafficStore) ResetTraffic(ctx context.Context, label string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE traffic_user SET bytes_in = 0, bytes_out = 0, snap_in = 0, snap_out = 0
		WHERE label = ?
	`, label)
	return err
}

// ResetAllTraffic clears traffic for all users.
func (s *TrafficStore) ResetAllTraffic(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE traffic_user SET bytes_in = 0, bytes_out = 0, snap_in = 0, snap_out = 0
	`)
	return err
}
