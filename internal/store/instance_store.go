package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fussraider/PopuGate/internal/model"
)

// InstanceStore handles instance persistence.
type InstanceStore struct {
	db *sql.DB
}

// NewInstanceStore creates a new InstanceStore.
func NewInstanceStore(db *sql.DB) *InstanceStore {
	return &InstanceStore{db: db}
}

// List returns all instances.
func (s *InstanceStore) List(ctx context.Context) ([]model.Instance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, port, metrics_port, enabled, label FROM instances ORDER BY port
	`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	instances := make([]model.Instance, 0)
	for rows.Next() {
		var inst model.Instance
		var enabled int
		if err := rows.Scan(&inst.ID, &inst.Port, &inst.MetricsPort, &enabled, &inst.Label); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		inst.Enabled = enabled == 1
		instances = append(instances, inst)
	}
	return instances, nil
}

// GetByPort returns a single instance by port.
func (s *InstanceStore) GetByPort(ctx context.Context, port int) (*model.Instance, error) {
	var inst model.Instance
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, port, metrics_port, enabled, label FROM instances WHERE port = ?
	`, port).Scan(&inst.ID, &inst.Port, &inst.MetricsPort, &enabled, &inst.Label)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get instance port %d: %w", port, err)
	}
	inst.Enabled = enabled == 1
	return &inst, nil
}

// Create inserts a new instance.
func (s *InstanceStore) Create(ctx context.Context, inst *model.Instance) error {
	enabled := 0
	if inst.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO instances (port, metrics_port, enabled, label) VALUES (?, ?, ?, ?)
	`, inst.Port, inst.MetricsPort, enabled, inst.Label)
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	inst.ID, _ = result.LastInsertId()
	return nil
}

// Delete removes an instance by port.
func (s *InstanceStore) Delete(ctx context.Context, port int) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM instances WHERE port = ?", port)
	return err
}
