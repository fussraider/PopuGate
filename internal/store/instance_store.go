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

const instanceColumns = `id, port, metrics_port, enabled, label, tls_domain, tls_domains, fake_tls, mask_host, mask_port, tags, tcp_mss_enabled, tcp_mss, tls_fronting, unknown_sni_action, client_mss_bulk, exclusive_mask, api_port`

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanInstance(row scanner) (*model.Instance, error) {
	var inst model.Instance
	var enabled, fakeTLS, tcpMSSEnabled, tlsFronting int
	if err := row.Scan(&inst.ID, &inst.Port, &inst.MetricsPort, &enabled, &inst.Label,
		&inst.TLSDomain, &inst.TLSDomains, &fakeTLS, &inst.MaskHost, &inst.MaskPort, &inst.Tags,
		&tcpMSSEnabled, &inst.TCPMSS, &tlsFronting, &inst.UnknownSNIAction, &inst.TCPMSSBulk, &inst.ExclusiveMask, &inst.APIPort); err != nil {
		return nil, err
	}
	inst.Enabled = intToBool(enabled)
	inst.FakeTLS = intToBool(fakeTLS)
	inst.TCPMSSEnabled = intToBool(tcpMSSEnabled)
	inst.TLSFronting = intToBool(tlsFronting)
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
	defer func() { _ = rows.Close() }()

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

// Create inserts a new instance.
func (s *InstanceStore) Create(ctx context.Context, inst *model.Instance) error {
	enabled := boolToInt(inst.Enabled)
	fakeTLS := boolToInt(inst.FakeTLS)
	tcpMSSEnabled := boolToInt(inst.TCPMSSEnabled)
	tlsFronting := boolToInt(inst.TLSFronting)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO instances (port, metrics_port, enabled, label, tls_domain, tls_domains, fake_tls, mask_host, mask_port, tags, tcp_mss_enabled, tcp_mss, tls_fronting, unknown_sni_action, client_mss_bulk, exclusive_mask, api_port)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, inst.Port, inst.MetricsPort, enabled, inst.Label,
		inst.TLSDomain, inst.TLSDomains, fakeTLS, inst.MaskHost, inst.MaskPort, inst.Tags,
		tcpMSSEnabled, inst.TCPMSS, tlsFronting, inst.UnknownSNIAction, inst.TCPMSSBulk, inst.ExclusiveMask, inst.APIPort)
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
	tcpMSSEnabled := boolToInt(inst.TCPMSSEnabled)
	tlsFronting := boolToInt(inst.TLSFronting)
	_, err := s.db.ExecContext(ctx, `
		UPDATE instances SET port=?, metrics_port=?, enabled=?, label=?,
		                     tls_domain=?, tls_domains=?, fake_tls=?, mask_host=?, mask_port=?, tags=?,
		                     tcp_mss_enabled=?, tcp_mss=?, tls_fronting=?, unknown_sni_action=?, client_mss_bulk=?, exclusive_mask=?, api_port=?
		WHERE id = ?
	`, inst.Port, inst.MetricsPort, enabled, inst.Label,
		inst.TLSDomain, inst.TLSDomains, fakeTLS, inst.MaskHost, inst.MaskPort, inst.Tags,
		tcpMSSEnabled, inst.TCPMSS, tlsFronting, inst.UnknownSNIAction, inst.TCPMSSBulk, inst.ExclusiveMask, inst.APIPort,
		inst.ID)
	if err != nil {
		return fmt.Errorf("update instance %d: %w", inst.ID, err)
	}
	return nil
}

// BackfillAPIPorts assigns a loopback control-plane API port to any instance that
// has none (api_port = 0), so pre-existing instances gain a hot quota-reset API on
// their next container recreate. Ports are unique across all instances' port/
// metrics_port/api_port and allocated from a high base to avoid the proxy/metrics
// ranges. Idempotent: instances that already have a port are left untouched.
func (s *InstanceStore) BackfillAPIPorts(ctx context.Context) (int, error) {
	const apiPortBase = 19091
	instances, err := s.List(ctx)
	if err != nil {
		return 0, err
	}
	used := make(map[int]bool)
	next := apiPortBase
	for _, inst := range instances {
		used[inst.Port] = true
		used[inst.MetricsPort] = true
		if inst.APIPort != 0 {
			used[inst.APIPort] = true
			if inst.APIPort >= next {
				next = inst.APIPort + 1
			}
		}
	}
	assigned := 0
	for i := range instances {
		if instances[i].APIPort != 0 {
			continue
		}
		for used[next] {
			next++
		}
		used[next] = true
		instances[i].APIPort = next
		if err := s.Update(ctx, &instances[i]); err != nil {
			return assigned, fmt.Errorf("assign api_port to instance %d: %w", instances[i].ID, err)
		}
		assigned++
		next++
	}
	return assigned, nil
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
	defer func() { _ = rows.Close() }()

	instances := make([]model.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		inst.Enabled = true
		instances = append(instances, *inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled instances: %w", err)
	}
	return instances, nil
}
