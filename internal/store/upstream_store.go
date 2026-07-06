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

// upstreamSelectColumns is the canonical column order shared by every full-row
// SELECT; scanUpstream below scans in this exact order.
const upstreamSelectColumns = `id, name, type, address, username, password, url, weight, iface, enabled,
	last_check_at, last_check_ok, latency_ms, last_error, fail_count,
	auto_disabled, auto_disabled_at`

// Write column set shared by Create and CreateMultiple (and upstreamWriteArgs).
const (
	upstreamInsertColumns = "name, type, address, username, password, url, weight, iface, enabled"
	upstreamInsertValues  = "?, ?, ?, ?, ?, ?, ?, ?, ?"
)

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanUpstream reads one full upstream row. The scan order MUST match upstreamSelectColumns.
func scanUpstream(sc rowScanner) (model.Upstream, error) {
	var u model.Upstream
	var enabled, autoDisabled int
	var lastCheckOK sql.NullInt64
	if err := sc.Scan(&u.ID, &u.Name, &u.Type, &u.Address, &u.Username,
		&u.Password, &u.URL, &u.Weight, &u.Iface, &enabled,
		&u.LastCheckAt, &lastCheckOK, &u.LatencyMs, &u.LastError, &u.FailCount,
		&autoDisabled, &u.AutoDisabledAt); err != nil {
		return u, err
	}
	u.Enabled = intToBool(enabled)
	u.AutoDisabled = intToBool(autoDisabled)
	if lastCheckOK.Valid {
		v := intToBool(int(lastCheckOK.Int64))
		u.LastCheckOK = &v
	}
	return u, nil
}

// upstreamWriteArgs returns the INSERT arg list in upstreamInsertColumns order.
func upstreamWriteArgs(u *model.Upstream) []any {
	return []any{u.Name, u.Type, u.Address, u.Username, u.Password, u.URL, u.Weight, u.Iface, boolToInt(u.Enabled)}
}

// queryUpstreams runs a full-row SELECT and scans every result.
func (s *UpstreamStore) queryUpstreams(ctx context.Context, query string, args ...any) ([]model.Upstream, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query upstreams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	upstreams := make([]model.Upstream, 0)
	for rows.Next() {
		u, err := scanUpstream(rows)
		if err != nil {
			return nil, fmt.Errorf("scan upstream: %w", err)
		}
		upstreams = append(upstreams, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstreams: %w", err)
	}
	return upstreams, nil
}

// List returns all upstreams.
func (s *UpstreamStore) List(ctx context.Context) ([]model.Upstream, error) {
	return s.queryUpstreams(ctx, "SELECT "+upstreamSelectColumns+" FROM upstreams ORDER BY id")
}

// GetByName returns a single upstream by name.
func (s *UpstreamStore) GetByName(ctx context.Context, name string) (*model.Upstream, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+upstreamSelectColumns+" FROM upstreams WHERE name = ?", name)
	u, err := scanUpstream(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream %s: %w", name, err)
	}
	return &u, nil
}

// ListEnabled returns only enabled upstreams.
func (s *UpstreamStore) ListEnabled(ctx context.Context) ([]model.Upstream, error) {
	return s.queryUpstreams(ctx, "SELECT "+upstreamSelectColumns+" FROM upstreams WHERE enabled = 1 ORDER BY id")
}

// Create inserts a new upstream.
func (s *UpstreamStore) Create(ctx context.Context, u *model.Upstream) error {
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO upstreams ("+upstreamInsertColumns+") VALUES ("+upstreamInsertValues+")",
		upstreamWriteArgs(u)...)
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
	result, err := s.db.ExecContext(ctx, `
		UPDATE upstreams SET type = ?, address = ?, username = ?, password = ?, url = ?, weight = ?, iface = ?, enabled = ?,
		                     auto_disabled = 0, auto_disabled_at = 0
		WHERE name = ?
	`, u.Type, u.Address, u.Username, u.Password, u.URL, u.Weight, u.Iface, boolToInt(u.Enabled), name)
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
	_, err := s.db.ExecContext(ctx, `
		UPDATE upstreams 
		SET enabled = ?, auto_disabled = 0, auto_disabled_at = 0 
		WHERE name = ?
	`, v, name)
	return err
}

// DisableAutomatically automatically disables an upstream due to health failures.
func (s *UpstreamStore) DisableAutomatically(ctx context.Context, name string, timestamp int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE upstreams 
		SET enabled = 0, auto_disabled = 1, auto_disabled_at = ? 
		WHERE name = ?
	`, timestamp, name)
	return err
}

// EnableAutomatically automatically re-enables a recovered upstream.
func (s *UpstreamStore) EnableAutomatically(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE upstreams 
		SET enabled = 1, auto_disabled = 0, auto_disabled_at = 0, fail_count = 0 
		WHERE name = ?
	`, name)
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

// CreateMultiple inserts multiple upstreams inside a transaction, ignoring duplicates.
// Returns the upstreams that were actually inserted (rows ignored as duplicates
// are excluded), preserving input order.
func (s *UpstreamStore) CreateMultiple(ctx context.Context, upstreams []*model.Upstream) ([]*model.Upstream, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT OR IGNORE INTO upstreams ("+upstreamInsertColumns+") VALUES ("+upstreamInsertValues+")")
	if err != nil {
		return nil, fmt.Errorf("prepare stmt: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	inserted := make([]*model.Upstream, 0, len(upstreams))
	for _, u := range upstreams {
		if err := u.Validate(); err != nil {
			return nil, fmt.Errorf("validate upstream %s: %w", u.Name, err)
		}
		res, err := stmt.ExecContext(ctx, upstreamWriteArgs(u)...)
		if err != nil {
			return nil, fmt.Errorf("exec insert %s: %w", u.Name, err)
		}
		rows, _ := res.RowsAffected()
		if rows > 0 {
			inserted = append(inserted, u)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return inserted, nil
}
