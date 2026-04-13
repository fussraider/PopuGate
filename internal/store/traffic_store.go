package store

import (
	"context"
	"database/sql"
	"fmt"

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

// ListUserTraffic returns all per-user traffic stats.
func (s *TrafficStore) ListUserTraffic(ctx context.Context) ([]model.UserTraffic, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT label, bytes_in, bytes_out FROM traffic_user ORDER BY label
	`)
	if err != nil {
		return nil, fmt.Errorf("list user traffic: %w", err)
	}
	defer rows.Close()

	stats := make([]model.UserTraffic, 0)
	for rows.Next() {
		var u model.UserTraffic
		if err := rows.Scan(&u.Label, &u.BytesIn, &u.BytesOut); err != nil {
			return nil, fmt.Errorf("scan user traffic: %w", err)
		}
		stats = append(stats, u)
	}
	return stats, nil
}

// GetUserTraffic returns traffic for a specific user.
func (s *TrafficStore) GetUserTraffic(ctx context.Context, label string) (*model.UserTraffic, error) {
	var u model.UserTraffic
	err := s.db.QueryRowContext(ctx, `
		SELECT label, bytes_in, bytes_out FROM traffic_user WHERE label = ?
	`, label).Scan(&u.Label, &u.BytesIn, &u.BytesOut)
	if err == sql.ErrNoRows {
		return &model.UserTraffic{Label: label}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user traffic %s: %w", label, err)
	}
	return &u, nil
}

// GetUserSnapshot returns the raw Prometheus snapshot for a user.
func (s *TrafficStore) GetUserSnapshot(ctx context.Context, label string) (snapIn, snapOut int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT snap_in, snap_out FROM traffic_user WHERE label = ?
	`, label).Scan(&snapIn, &snapOut)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

// UpdateUserTraffic adds deltas to a user's traffic and updates the snapshot.
func (s *TrafficStore) UpdateUserTraffic(ctx context.Context, label string, deltaIn, deltaOut, snapIn, snapOut int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO traffic_user (label, bytes_in, bytes_out, snap_in, snap_out)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(label) DO UPDATE SET
			bytes_in = bytes_in + excluded.bytes_in,
			bytes_out = bytes_out + excluded.bytes_out,
			snap_in = excluded.snap_in,
			snap_out = excluded.snap_out
	`, label, deltaIn, deltaOut, snapIn, snapOut)
	return err
}

// FlushTraffic persists all traffic deltas in a single transaction.
func (s *TrafficStore) FlushTraffic(ctx context.Context, global model.TrafficSnapshot, users map[string]model.TrafficSnapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin flush tx: %w", err)
	}
	defer tx.Rollback()

	// Update global
	if _, err := tx.ExecContext(ctx, `
		UPDATE traffic_global SET
			bytes_in = bytes_in + ?, bytes_out = bytes_out + ?,
			snap_in = ?, snap_out = ?
		WHERE id = 1
	`, global.BytesIn, global.BytesOut, global.SnapIn, global.SnapOut); err != nil {
		return fmt.Errorf("flush global: %w", err)
	}

	// Update per-user
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO traffic_user (label, bytes_in, bytes_out, snap_in, snap_out)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(label) DO UPDATE SET
			bytes_in = bytes_in + excluded.bytes_in,
			bytes_out = bytes_out + excluded.bytes_out,
			snap_in = excluded.snap_in,
			snap_out = excluded.snap_out
	`)
	if err != nil {
		return fmt.Errorf("prepare user flush: %w", err)
	}
	defer stmt.Close()

	for label, snap := range users {
		if _, err := stmt.ExecContext(ctx, label, snap.BytesIn, snap.BytesOut, snap.SnapIn, snap.SnapOut); err != nil {
			return fmt.Errorf("flush user %s: %w", label, err)
		}
	}

	return tx.Commit()
}
