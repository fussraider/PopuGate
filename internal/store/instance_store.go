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

const instanceColumns = `id, port, metrics_port, enabled, label, tls_domain, tls_domains, fake_tls, mask_host, mask_port, tags`

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanInstance(row scanner) (*model.Instance, error) {
	var inst model.Instance
	var enabled, fakeTLS int
	if err := row.Scan(&inst.ID, &inst.Port, &inst.MetricsPort, &enabled, &inst.Label,
		&inst.TLSDomain, &inst.TLSDomains, &fakeTLS, &inst.MaskHost, &inst.MaskPort, &inst.Tags); err != nil {
		return nil, err
	}
	inst.Enabled = intToBool(enabled)
	inst.FakeTLS = intToBool(fakeTLS)
	return &inst, nil
}

// Count returns the total number of instances.
func (s *InstanceStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instances").Scan(&count)
	return count, err
}

// List returns all instances.
func (s *InstanceStore) List(ctx context.Context) ([]model.Instance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+instanceColumns+` FROM instances ORDER BY port
	`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	instances := make([]model.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		instances = append(instances, *inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return instances, nil
}

// GetByID returns a single instance by ID.
func (s *InstanceStore) GetByID(ctx context.Context, id int64) (*model.Instance, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+instanceColumns+` FROM instances WHERE id = ?
	`, id)
	inst, err := scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get instance %d: %w", id, err)
	}
	return inst, nil
}

// GetByPort returns a single instance by port.
func (s *InstanceStore) GetByPort(ctx context.Context, port int) (*model.Instance, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+instanceColumns+` FROM instances WHERE port = ?
	`, port)
	inst, err := scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get instance port %d: %w", port, err)
	}
	return inst, nil
}

// EnsureDefaultInstance checks if instances table is empty and seeds it from settings.
func (s *InstanceStore) EnsureDefaultInstance(ctx context.Context, proxyPort, metricsPort int, proxyDomain, maskingHost string, maskingEnabled bool) error {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instances").Scan(&count)
	if err != nil {
		return fmt.Errorf("count instances: %w", err)
	}

	if count == 0 {
		fakeTLS := 0
		if maskingEnabled {
			fakeTLS = 1
		}
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO instances (port, metrics_port, enabled, label, tls_domain, fake_tls, mask_host, mask_port)
			VALUES (?, ?, 1, 'Default', ?, ?, ?, 443)
		`, proxyPort, metricsPort, proxyDomain, fakeTLS, maskingHost)
		if err != nil {
			return fmt.Errorf("seed default instance: %w", err)
		}
	}
	return nil
}

// Create inserts a new instance.
func (s *InstanceStore) Create(ctx context.Context, inst *model.Instance) error {
	enabled := boolToInt(inst.Enabled)
	fakeTLS := boolToInt(inst.FakeTLS)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO instances (port, metrics_port, enabled, label, tls_domain, tls_domains, fake_tls, mask_host, mask_port, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, inst.Port, inst.MetricsPort, enabled, inst.Label,
		inst.TLSDomain, inst.TLSDomains, fakeTLS, inst.MaskHost, inst.MaskPort, inst.Tags)
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	inst.ID, _ = result.LastInsertId()
	return nil
}

// Update modifies an existing instance.
func (s *InstanceStore) Update(ctx context.Context, inst *model.Instance) error {
	enabled := boolToInt(inst.Enabled)
	fakeTLS := boolToInt(inst.FakeTLS)
	_, err := s.db.ExecContext(ctx, `
		UPDATE instances SET port=?, metrics_port=?, enabled=?, label=?,
		                     tls_domain=?, tls_domains=?, fake_tls=?, mask_host=?, mask_port=?, tags=?
		WHERE id = ?
	`, inst.Port, inst.MetricsPort, enabled, inst.Label,
		inst.TLSDomain, inst.TLSDomains, fakeTLS, inst.MaskHost, inst.MaskPort, inst.Tags,
		inst.ID)
	if err != nil {
		return fmt.Errorf("update instance %d: %w", inst.ID, err)
	}
	return nil
}

// Delete removes an instance by port.
func (s *InstanceStore) Delete(ctx context.Context, port int) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM instances WHERE port = ?", port)
	if err != nil {
		return fmt.Errorf("delete instance port %d: %w", port, err)
	}
	return nil
}

// DeleteByID removes an instance by ID.
func (s *InstanceStore) DeleteByID(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM instances WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete instance %d: %w", id, err)
	}
	return nil
}

// ListEnabled returns all enabled instances.
func (s *InstanceStore) ListEnabled(ctx context.Context) ([]model.Instance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+instanceColumns+` FROM instances WHERE enabled = 1 ORDER BY port
	`)
	if err != nil {
		return nil, fmt.Errorf("list enabled instances: %w", err)
	}
	defer rows.Close()

	instances := make([]model.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		inst.Enabled = true
		instances = append(instances, *inst)
	}
	return instances, rows.Err()
}
