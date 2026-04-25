package store

import (
	"context"
	"database/sql"
	"fmt"

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
		SELECT id, name, type, address, username, password, weight, iface, enabled
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
		if err := rows.Scan(&u.ID, &u.Name, &u.Type, &u.Address, &u.Username,
			&u.Password, &u.Weight, &u.Iface, &enabled); err != nil {
			return nil, fmt.Errorf("scan upstream: %w", err)
		}
		u.Enabled = intToBool(enabled)
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
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, address, username, password, weight, iface, enabled
		FROM upstreams WHERE name = ?
	`, name).Scan(&u.ID, &u.Name, &u.Type, &u.Address, &u.Username,
		&u.Password, &u.Weight, &u.Iface, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream %s: %w", name, err)
	}
	u.Enabled = intToBool(enabled)
	return &u, nil
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

// UpdateEnabled toggles an upstream's enabled state.
func (s *UpstreamStore) UpdateEnabled(ctx context.Context, name string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, "UPDATE upstreams SET enabled = ? WHERE name = ?", v, name)
	return err
}
