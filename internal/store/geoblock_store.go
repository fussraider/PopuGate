package store

import (
	"context"
	"database/sql"
	"time"
)

// GeoblockCacheStore handles geo-block CIDR cache tracking.
type GeoblockCacheStore struct {
	db *sql.DB
}

// NewGeoblockCacheStore creates a new GeoblockCacheStore.
func NewGeoblockCacheStore(db *sql.DB) *GeoblockCacheStore {
	return &GeoblockCacheStore{db: db}
}

// GetCache returns cached info for a country.
func (s *GeoblockCacheStore) GetCache(ctx context.Context, code string) (filePath string, downloadedAt int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT file_path, downloaded_at FROM geoblock_cache WHERE country_code = ?
	`, code).Scan(&filePath, &downloadedAt)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return
}

// SetCache updates the cache info for a country.
func (s *GeoblockCacheStore) SetCache(ctx context.Context, code, filePath string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO geoblock_cache (country_code, file_path, downloaded_at)
		VALUES (?, ?, ?)
	`, code, filePath, time.Now().Unix())
	return err
}
