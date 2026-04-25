package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fussraider/PopuGate/internal/model"
)

// SlaveStore handles replication slave persistence.
type SlaveStore struct {
	db *sql.DB
}

// NewSlaveStore creates a new SlaveStore.
func NewSlaveStore(db *sql.DB) *SlaveStore {
	return &SlaveStore{db: db}
}

// List returns all slaves.
func (s *SlaveStore) List(ctx context.Context) ([]model.Slave, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, host, port, label, enabled, last_sync, status FROM slaves ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list slaves: %w", err)
	}
	defer rows.Close()

	slaves := make([]model.Slave, 0)
	for rows.Next() {
		var sl model.Slave
		var enabled int
		if err := rows.Scan(&sl.ID, &sl.Host, &sl.Port, &sl.Label,
			&enabled, &sl.LastSync, &sl.Status); err != nil {
			return nil, fmt.Errorf("scan slave: %w", err)
		}
		sl.Enabled = intToBool(enabled)
		slaves = append(slaves, sl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate slaves: %w", err)
	}
	return slaves, nil
}

// GetByHost returns a single slave by host.
func (s *SlaveStore) GetByHost(ctx context.Context, host string) (*model.Slave, error) {
	var sl model.Slave
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, host, port, label, enabled, last_sync, status FROM slaves WHERE host = ?
	`, host).Scan(&sl.ID, &sl.Host, &sl.Port, &sl.Label, &enabled, &sl.LastSync, &sl.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get slave %s: %w", host, err)
	}
	sl.Enabled = intToBool(enabled)
	return &sl, nil
}

// Create inserts a new slave.
func (s *SlaveStore) Create(ctx context.Context, sl *model.Slave) error {
	enabled := 0
	if sl.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO slaves (host, port, label, enabled, last_sync, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sl.Host, sl.Port, sl.Label, enabled, sl.LastSync, sl.Status)
	if err != nil {
		return fmt.Errorf("create slave: %w", err)
	}
	sl.ID, _ = result.LastInsertId()
	return nil
}

// Delete removes a slave by host.
func (s *SlaveStore) Delete(ctx context.Context, host string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM slaves WHERE host = ?", host)
	return err
}

// UpdateStatus updates a slave's sync status.
func (s *SlaveStore) UpdateStatus(ctx context.Context, host, status string, lastSync int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE slaves SET status = ?, last_sync = ? WHERE host = ?
	`, status, lastSync, host)
	return err
}
