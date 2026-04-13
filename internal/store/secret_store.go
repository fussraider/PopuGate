package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fussraider/PopuGate/internal/model"
)

// SecretStore handles secret persistence in SQLite.
type SecretStore struct {
	db *sql.DB
}

// NewSecretStore creates a new SecretStore.
func NewSecretStore(db *sql.DB) *SecretStore {
	return &SecretStore{db: db}
}

// List returns all secrets, optionally with traffic data.
func (s *SecretStore) List(ctx context.Context) ([]model.Secret, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes, s.expires_at, s.notes,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN traffic_user t ON s.label = t.label
		ORDER BY s.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()

	secrets := make([]model.Secret, 0)
	for rows.Next() {
		var sec model.Secret
		var enabled int
		if err := rows.Scan(&sec.ID, &sec.Label, &sec.SecretKey, &sec.CreatedAt,
			&enabled, &sec.MaxConns, &sec.MaxIPs, &sec.QuotaBytes,
			&sec.ExpiresAt, &sec.Notes, &sec.TrafficIn, &sec.TrafficOut); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		sec.Enabled = enabled == 1
		secrets = append(secrets, sec)
	}
	return secrets, nil
}

// GetByLabel returns a single secret by label.
func (s *SecretStore) GetByLabel(ctx context.Context, label string) (*model.Secret, error) {
	var sec model.Secret
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes, s.expires_at, s.notes,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN traffic_user t ON s.label = t.label
		WHERE s.label = ?
	`, label).Scan(&sec.ID, &sec.Label, &sec.SecretKey, &sec.CreatedAt,
		&enabled, &sec.MaxConns, &sec.MaxIPs, &sec.QuotaBytes,
		&sec.ExpiresAt, &sec.Notes, &sec.TrafficIn, &sec.TrafficOut)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get secret %s: %w", label, err)
	}
	sec.Enabled = enabled == 1
	return &sec, nil
}

// Create inserts a new secret.
func (s *SecretStore) Create(ctx context.Context, sec *model.Secret) error {
	enabled := 0
	if sec.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO secrets (label, secret_key, enabled, max_conns, max_ips, quota_bytes, expires_at, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sec.Label, sec.SecretKey, enabled, sec.MaxConns, sec.MaxIPs, sec.QuotaBytes, sec.ExpiresAt, sec.Notes)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}
	sec.ID, _ = result.LastInsertId()
	return nil
}

// Update modifies an existing secret.
func (s *SecretStore) Update(ctx context.Context, sec *model.Secret) error {
	enabled := 0
	if sec.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE secrets SET secret_key=?, enabled=?, max_conns=?, max_ips=?,
		                   quota_bytes=?, expires_at=?, notes=?
		WHERE label = ?
	`, sec.SecretKey, enabled, sec.MaxConns, sec.MaxIPs, sec.QuotaBytes,
		sec.ExpiresAt, sec.Notes, sec.Label)
	return err
}

// Delete removes a secret by label.
func (s *SecretStore) Delete(ctx context.Context, label string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM secrets WHERE label = ?", label)
	return err
}

// CountEnabled returns the count of enabled secrets.
func (s *SecretStore) CountEnabled(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets WHERE enabled = 1").Scan(&count)
	return count, err
}

// Count returns total number of secrets.
func (s *SecretStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets").Scan(&count)
	return count, err
}

// ListEnabledLabels returns labels of all enabled secrets.
func (s *SecretStore) ListEnabledLabels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT label FROM secrets WHERE enabled = 1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := make([]string, 0)
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, nil
}

// UpdateTraffic increments the traffic counter for a user.
func (s *SecretStore) UpdateTraffic(ctx context.Context, label string, deltaIn, deltaOut int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO traffic_user (label, bytes_in, bytes_out) VALUES (?, ?, ?)
		ON CONFLICT(label) DO UPDATE SET
			bytes_in = bytes_in + excluded.bytes_in,
			bytes_out = bytes_out + excluded.bytes_out
	`, label, deltaIn, deltaOut)
	return err
}

// ResetTraffic clears traffic for a specific user.
func (s *SecretStore) ResetTraffic(ctx context.Context, label string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE traffic_user SET bytes_in = 0, bytes_out = 0, snap_in = 0, snap_out = 0
		WHERE label = ?
	`, label)
	return err
}

// ResetAllTraffic clears traffic for all users.
func (s *SecretStore) ResetAllTraffic(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE traffic_user SET bytes_in = 0, bytes_out = 0, snap_in = 0, snap_out = 0
	`)
	return err
}
