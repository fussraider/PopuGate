package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// AuditStore handles audit log persistence.
type AuditStore struct {
	db *sql.DB
}

// NewAuditStore creates a new AuditStore.
func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// Insert adds a new audit entry.
func (s *AuditStore) Insert(ctx context.Context, user, action, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_log (timestamp, user, action, detail) VALUES (?, ?, ?, ?)
	`, time.Now().Unix(), user, action, detail)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

// List returns audit entries ordered by timestamp descending, with pagination.
func (s *AuditStore) List(ctx context.Context, limit, offset int) ([]model.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, user, action, detail
		FROM audit_log ORDER BY timestamp DESC LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.User, &e.Action, &e.Detail); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CleanOld removes audit entries older than the given duration.
func (s *AuditStore) CleanOld(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).Unix()
	result, err := s.db.ExecContext(ctx, "DELETE FROM audit_log WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("clean audit: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
