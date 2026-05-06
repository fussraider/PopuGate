package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// UpstreamStore handles upstream persistence.
type UpstreamStore struct {
	db *sql.DB
}

// NewUpstreamStore creates a new UpstreamStore.
func NewUpstreamStore(db *sql.DB) *UpstreamStore {
	return &UpstreamStore{db: db}
}

// List returns all upstreams.
func (s *UpstreamStore) List(ctx context.Context) ([]model.Upstream, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, address, username, password, weight, iface, enabled,
		       last_check_at, last_check_ok, latency_ms, last_error, fail_count
		FROM upstreams ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list upstreams: %w", err)
	}
	defer rows.Close()

	upstreams := make([]model.Upstream, 0)
	for rows.Next() {
		var u model.Upstream
		var enabled int
		var lastCheckOK sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Name, &u.Type, &u.Address, &u.Username,
			&u.Password, &u.Weight, &u.Iface, &enabled,
			&u.LastCheckAt, &lastCheckOK, &u.LatencyMs, &u.LastError, &u.FailCount); err != nil {
			return nil, fmt.Errorf("scan upstream: %w", err)
		}
		u.Enabled = intToBool(enabled)
		if lastCheckOK.Valid {
			v := intToBool(int(lastCheckOK.Int64))
			u.LastCheckOK = &v
		}
		upstreams = append(upstreams, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstreams: %w", err)
	}
	return upstreams, nil
}

// GetByName returns a single upstream by name.
func (s *UpstreamStore) GetByName(ctx context.Context, name string) (*model.Upstream, error) {
	var u model.Upstream
	var enabled int
	var lastCheckOK sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, address, username, password, weight, iface, enabled,
		       last_check_at, last_check_ok, latency_ms, last_error, fail_count
		FROM upstreams WHERE name = ?
	`, name).Scan(&u.ID, &u.Name, &u.Type, &u.Address, &u.Username,
		&u.Password, &u.Weight, &u.Iface, &enabled,
		&u.LastCheckAt, &lastCheckOK, &u.LatencyMs, &u.LastError, &u.FailCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream %s: %w", name, err)
	}
	u.Enabled = intToBool(enabled)
	if lastCheckOK.Valid {
		v := intToBool(int(lastCheckOK.Int64))
		u.LastCheckOK = &v
	}
	return &u, nil
}

// ListEnabled returns only enabled upstreams.
func (s *UpstreamStore) ListEnabled(ctx context.Context) ([]model.Upstream, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, address, username, password, weight, iface, enabled,
		       last_check_at, last_check_ok, latency_ms, last_error, fail_count
		FROM upstreams WHERE enabled = 1 ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list enabled upstreams: %w", err)
	}
	defer rows.Close()

	upstreams := make([]model.Upstream, 0)
	for rows.Next() {
		var u model.Upstream
		var enabled int
		var lastCheckOK sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Name, &u.Type, &u.Address, &u.Username,
			&u.Password, &u.Weight, &u.Iface, &enabled,
			&u.LastCheckAt, &lastCheckOK, &u.LatencyMs, &u.LastError, &u.FailCount); err != nil {
			return nil, fmt.Errorf("scan upstream: %w", err)
		}
		u.Enabled = intToBool(enabled)
		if lastCheckOK.Valid {
			v := intToBool(int(lastCheckOK.Int64))
			u.LastCheckOK = &v
		}
		upstreams = append(upstreams, u)
	}
	return upstreams, nil
}

// Create inserts a new upstream.
func (s *UpstreamStore) Create(ctx context.Context, u *model.Upstream) error {
	enabled := 0
	if u.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO upstreams (name, type, address, username, password, weight, iface, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, u.Name, u.Type, u.Address, u.Username, u.Password, u.Weight, u.Iface, enabled)
	if err != nil {
		return fmt.Errorf("create upstream: %w", err)
	}
	u.ID, _ = result.LastInsertId()
	return nil
}

// Delete removes an upstream by name.
func (s *UpstreamStore) Delete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM upstreams WHERE name = ?", name)
	return err
}

// Update modifies an existing upstream identified by name.
func (s *UpstreamStore) Update(ctx context.Context, name string, u *model.Upstream) error {
	enabled := 0
	if u.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE upstreams SET type = ?, address = ?, username = ?, password = ?, weight = ?, iface = ?, enabled = ?
		WHERE name = ?
	`, u.Type, u.Address, u.Username, u.Password, u.Weight, u.Iface, enabled, name)
	if err != nil {
		return fmt.Errorf("update upstream %s: %w", name, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("upstream '%s' not found", name)
	}
	return nil
}

// UpdateHealth updates health status fields for an upstream.
func (s *UpstreamStore) UpdateHealth(ctx context.Context, name string, ok bool, latencyMs int64, errMsg string) error {
	vOK := 0
	if ok {
		vOK = 1
	}

	var query string
	if ok {
		query = `UPDATE upstreams SET last_check_at = ?, last_check_ok = ?, latency_ms = ?, last_error = ?, fail_count = 0 
		         WHERE name = ?`
	} else {
		query = `UPDATE upstreams SET last_check_at = ?, last_check_ok = ?, latency_ms = ?, last_error = ?, fail_count = fail_count + 1 
		         WHERE name = ?`
	}

	_, err := s.db.ExecContext(ctx, query, time.Now().Unix(), vOK, latencyMs, errMsg, name)
	return err
}

// UpdateEnabled toggles an upstream's enabled state.
func (s *UpstreamStore) UpdateEnabled(ctx context.Context, name string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, "UPDATE upstreams SET enabled = ? WHERE name = ?", v, name)
	return err
}

// ClearHealth resets all health-related fields to their default values.
func (s *UpstreamStore) ClearHealth(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE upstreams SET last_check_at = 0, last_check_ok = NULL, latency_ms = 0, last_error = '', fail_count = 0
		WHERE name = ?
	`, name)
	return err
}
