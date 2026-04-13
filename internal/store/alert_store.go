package store

import (
	"context"
	"database/sql"
)

// QuotaAlertStore handles quota alert deduplication.
type QuotaAlertStore struct {
	db *sql.DB
}

// NewQuotaAlertStore creates a new QuotaAlertStore.
func NewQuotaAlertStore(db *sql.DB) *QuotaAlertStore {
	return &QuotaAlertStore{db: db}
}

// WasAlerted checks if a quota alert was already sent for this label+percent combo.
func (s *QuotaAlertStore) WasAlerted(ctx context.Context, label string, percent int) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM quota_alerts WHERE label = ? AND percent = ?
	`, label, percent).Scan(&count)
	return count > 0, err
}

// MarkAlerted records that a quota alert was sent.
func (s *QuotaAlertStore) MarkAlerted(ctx context.Context, label string, percent int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO quota_alerts (label, percent) VALUES (?, ?)
	`, label, percent)
	return err
}

// ClearForLabel removes quota alerts when traffic is reset.
func (s *QuotaAlertStore) ClearForLabel(ctx context.Context, label string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM quota_alerts WHERE label = ?", label)
	return err
}
