package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// List returns audit entries ordered by timestamp descending, with pagination and optional filters.
func (s *AuditStore) List(ctx context.Context, limit, offset int, filter *model.AuditFilter) ([]model.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := `SELECT id, timestamp, user, action, detail FROM audit_log WHERE 1=1`
	var args []interface{}

	if filter != nil {
		if len(filter.Users) > 0 {
			var placeholders []string
			for _, u := range filter.Users {
				if u != "" {
					placeholders = append(placeholders, "?")
					args = append(args, u)
				}
			}
			if len(placeholders) > 0 {
				query += fmt.Sprintf(" AND user IN (%s)", strings.Join(placeholders, ","))
			}
		}

		if len(filter.Actions) > 0 {
			var placeholders []string
			for _, a := range filter.Actions {
				if a != "" {
					placeholders = append(placeholders, "?")
					args = append(args, a)
				}
			}
			if len(placeholders) > 0 {
				query += fmt.Sprintf(" AND action IN (%s)", strings.Join(placeholders, ","))
			}
		}

		if filter.From > 0 {
			query += " AND timestamp >= ?"
			args = append(args, filter.From)
		}

		if filter.To > 0 {
			query += " AND timestamp <= ?"
			args = append(args, filter.To)
		}
	}

	query += " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

// GetFilters returns all unique users and actions from the audit log.
func (s *AuditStore) GetFilters(ctx context.Context) ([]string, []string, error) {
	userRows, err := s.db.QueryContext(ctx, "SELECT DISTINCT user FROM audit_log ORDER BY user ASC")
	if err != nil {
		return nil, nil, fmt.Errorf("get distinct users: %w", err)
	}
	defer func() { _ = userRows.Close() }()

	var users []string
	for userRows.Next() {
		var u string
		if err := userRows.Scan(&u); err == nil && u != "" {
			users = append(users, u)
		}
	}

	actionRows, err := s.db.QueryContext(ctx, "SELECT DISTINCT action FROM audit_log ORDER BY action ASC")
	if err != nil {
		return nil, nil, fmt.Errorf("get distinct actions: %w", err)
	}
	defer func() { _ = actionRows.Close() }()

	var actions []string
	for actionRows.Next() {
		var a string
		if err := actionRows.Scan(&a); err == nil && a != "" {
			actions = append(actions, a)
		}
	}

	return users, actions, nil
}
