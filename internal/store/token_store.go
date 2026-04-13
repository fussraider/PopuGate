package store

import (
	"context"
	"database/sql"
	"time"
)

// TokenBlocklistStore handles JWT token revocation.
type TokenBlocklistStore struct {
	db *sql.DB
}

// NewTokenBlocklistStore creates a new TokenBlocklistStore.
func NewTokenBlocklistStore(db *sql.DB) *TokenBlocklistStore {
	return &TokenBlocklistStore{db: db}
}

// Add adds a token JTI to the blocklist.
func (s *TokenBlocklistStore) Add(ctx context.Context, jti string, expiresAt int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO token_blocklist (jti, expires_at) VALUES (?, ?)
	`, jti, expiresAt)
	return err
}

// IsBlocked checks if a token JTI is in the blocklist.
func (s *TokenBlocklistStore) IsBlocked(ctx context.Context, jti string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM token_blocklist WHERE jti = ?
	`, jti).Scan(&count)
	return count > 0, err
}

// Cleanup removes expired entries from the blocklist.
func (s *TokenBlocklistStore) Cleanup(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM token_blocklist WHERE expires_at < ?
	`, time.Now().Unix())
	return err
}
