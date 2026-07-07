package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// SecretStore handles secret persistence in SQLite.
type SecretStore struct {
	db *sql.DB
}

// NewSecretStore creates a new SecretStore.
func NewSecretStore(db *sql.DB) *SecretStore {
	return &SecretStore{db: db}
}

// List returns all secrets, optionally with traffic data.
func (s *SecretStore) List(ctx context.Context) ([]model.Secret, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes,
		       s.rate_limit_up_bps, s.rate_limit_down_bps, s.expires_at, s.notes,
		       s.tags, s.archived_at,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN (SELECT label, SUM(bytes_in) AS bytes_in, SUM(bytes_out) AS bytes_out FROM traffic_user GROUP BY label) t ON s.label = t.label
		ORDER BY s.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	secrets := make([]model.Secret, 0)
	for rows.Next() {
		var sec model.Secret
		var enabled int
		if err := rows.Scan(&sec.ID, &sec.Label, &sec.SecretKey, &sec.CreatedAt,
			&enabled, &sec.MaxConns, &sec.MaxIPs, &sec.QuotaBytes,
			&sec.RateLimitUpBps, &sec.RateLimitDownBps,
			&sec.ExpiresAt, &sec.Notes, &sec.Tags, &sec.ArchivedAt,
			&sec.TrafficIn, &sec.TrafficOut); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		sec.Enabled = intToBool(enabled)
		secrets = append(secrets, sec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate secrets: %w", err)
	}
	return secrets, nil
}

// GetByLabel returns a single secret by label.
func (s *SecretStore) GetByLabel(ctx context.Context, label string) (*model.Secret, error) {
	var sec model.Secret
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes,
		       s.rate_limit_up_bps, s.rate_limit_down_bps, s.expires_at, s.notes,
		       s.tags, s.archived_at,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN (SELECT label, SUM(bytes_in) AS bytes_in, SUM(bytes_out) AS bytes_out FROM traffic_user GROUP BY label) t ON s.label = t.label
		WHERE s.label = ?
	`, label).Scan(&sec.ID, &sec.Label, &sec.SecretKey, &sec.CreatedAt,
		&enabled, &sec.MaxConns, &sec.MaxIPs, &sec.QuotaBytes,
		&sec.RateLimitUpBps, &sec.RateLimitDownBps,
		&sec.ExpiresAt, &sec.Notes, &sec.Tags, &sec.ArchivedAt,
		&sec.TrafficIn, &sec.TrafficOut)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get secret %s: %w", label, err)
	}
	sec.Enabled = intToBool(enabled)
	return &sec, nil
}

// Create inserts a new secret.
func (s *SecretStore) Create(ctx context.Context, sec *model.Secret) error {
	enabled := 0
	if sec.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO secrets (label, secret_key, enabled, max_conns, max_ips, quota_bytes, rate_limit_up_bps, rate_limit_down_bps, expires_at, notes, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sec.Label, sec.SecretKey, enabled, sec.MaxConns, sec.MaxIPs, sec.QuotaBytes, sec.RateLimitUpBps, sec.RateLimitDownBps, sec.ExpiresAt, sec.Notes, sec.Tags)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}
	sec.ID, _ = result.LastInsertId()
	return nil
}

// Update modifies an existing secret.
func (s *SecretStore) Update(ctx context.Context, sec *model.Secret) error {
	enabled := 0
	if sec.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE secrets SET secret_key=?, enabled=?, max_conns=?, max_ips=?,
		                   quota_bytes=?, rate_limit_up_bps=?, rate_limit_down_bps=?,
		                   expires_at=?, notes=?, tags=?
		WHERE label = ?
	`, sec.SecretKey, enabled, sec.MaxConns, sec.MaxIPs, sec.QuotaBytes,
		sec.RateLimitUpBps, sec.RateLimitDownBps,
		sec.ExpiresAt, sec.Notes, sec.Tags, sec.Label)
	return err
}

// Delete removes a secret by label.
func (s *SecretStore) Delete(ctx context.Context, label string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM secrets WHERE label = ?", label)
	return err
}

// CountEnabled returns the count of enabled secrets.
func (s *SecretStore) CountEnabled(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets WHERE enabled = 1").Scan(&count)
	return count, err
}

// CountEnabledByLabels returns the count of enabled secrets among the given labels.
func (s *SecretStore) CountEnabledByLabels(ctx context.Context, labels []string) (int, error) {
	if len(labels) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(labels))
	args := make([]any, len(labels))
	for i, l := range labels {
		placeholders[i] = "?"
		args[i] = l
	}
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM secrets WHERE enabled = 1 AND label IN ("+strings.Join(placeholders, ",")+")",
		args...,
	).Scan(&count)
	return count, err
}

// Count returns total number of secrets.
func (s *SecretStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets").Scan(&count)
	return count, err
}

// ListEnabledLabels returns labels of all enabled secrets.
func (s *SecretStore) ListEnabledLabels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT label FROM secrets WHERE enabled = 1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	labels := make([]string, 0)
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate labels: %w", err)
	}
	return labels, nil
}

// RenameLabel changes the label of a secret and its traffic row.
func (s *SecretStore) RenameLabel(ctx context.Context, oldLabel, newLabel string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, "UPDATE secrets SET label = ? WHERE label = ?", newLabel, oldLabel)
	if err != nil {
		return fmt.Errorf("rename secret: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}

	if _, err := tx.ExecContext(ctx, "UPDATE traffic_user SET label = ? WHERE label = ?", newLabel, oldLabel); err != nil {
		return fmt.Errorf("rename traffic_user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE traffic_history SET label = ? WHERE label = ?", newLabel, oldLabel); err != nil {
		return fmt.Errorf("rename traffic_history: %w", err)
	}

	return tx.Commit()
}

// ExtendExpiry sets a new expiry date for a secret, optionally re-enabling it.
func (s *SecretStore) ExtendExpiry(ctx context.Context, label, expiresAt string, reenable bool) error {
	q := "UPDATE secrets SET expires_at = ?"
	args := []any{expiresAt}
	if reenable {
		q += ", enabled = 1"
	}
	q += " WHERE label = ?"
	args = append(args, label)

	result, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("extend expiry: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DisableExpired disables all secrets whose expiry is in the past.
// Returns the number of secrets disabled. Never disables the last enabled secret.
func (s *SecretStore) DisableExpired(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	toDisable, err := findExpiredIDs(ctx, tx)
	if err != nil || len(toDisable) == 0 {
		if err == nil {
			return 0, tx.Commit()
		}
		return 0, err
	}

	var enabledCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets WHERE enabled = 1").Scan(&enabledCount); err != nil {
		return 0, fmt.Errorf("count enabled: %w", err)
	}
	if len(toDisable) >= enabledCount {
		toDisable = toDisable[:len(toDisable)-1]
	}

	disabled := 0
	for _, id := range toDisable {
		res, err := tx.ExecContext(ctx, "UPDATE secrets SET enabled = 0 WHERE id = ? AND enabled = 1", id)
		if err != nil {
			return 0, fmt.Errorf("disable expired %d: %w", id, err)
		}
		n, _ := res.RowsAffected()
		disabled += int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return disabled, nil
}

func findExpiredIDs(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	r, err := tx.QueryContext(ctx, `SELECT id, expires_at FROM secrets WHERE enabled = 1 AND expires_at != '' AND expires_at != '0'`)
	if err != nil {
		return nil, fmt.Errorf("query expired: %w", err)
	}
	defer func() { _ = r.Close() }()

	now := time.Now()
	var toDisable []int64
	for r.Next() {
		var id int64
		var expiresAt string
		if err := r.Scan(&id, &expiresAt); err != nil {
			continue
		}
		t, err := parseExpiry(expiresAt)
		if err != nil {
			continue
		}
		if now.After(t) {
			toDisable = append(toDisable, id)
		}
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("scan expired: %w", err)
	}
	return toDisable, nil
}

// UpdateTags sets the tags string for a secret.
func (s *SecretStore) UpdateTags(ctx context.Context, label, tags string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE secrets SET tags = ? WHERE label = ?", tags, label)
	if err != nil {
		return fmt.Errorf("update tags: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListByTag returns secrets that contain the given tag.
func (s *SecretStore) ListByTag(ctx context.Context, tag string) ([]model.Secret, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes,
		       s.rate_limit_up_bps, s.rate_limit_down_bps, s.expires_at, s.notes,
		       s.tags, s.archived_at,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN (SELECT label, SUM(bytes_in) AS bytes_in, SUM(bytes_out) AS bytes_out FROM traffic_user GROUP BY label) t ON s.label = t.label
		WHERE EXISTS (SELECT 1 FROM json_each(s.tags) WHERE json_each.value = ?)
		ORDER BY s.id
	`, tag)
	if err != nil {
		return nil, fmt.Errorf("list by tag: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSecrets(rows)
}

// Archive sets archived_at on a secret, effectively hiding it.
func (s *SecretStore) Archive(ctx context.Context, label string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE secrets SET archived_at = ? WHERE label = ?", time.Now().Unix(), label)
	if err != nil {
		return fmt.Errorf("archive secret: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Unarchive clears archived_at on a secret.
func (s *SecretStore) Unarchive(ctx context.Context, label string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE secrets SET archived_at = 0 WHERE label = ?", label)
	if err != nil {
		return fmt.Errorf("unarchive secret: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// scanSecrets is a helper to scan secret rows with tags and archived_at.
func scanSecrets(rows *sql.Rows) ([]model.Secret, error) {
	var secrets []model.Secret
	for rows.Next() {
		var sec model.Secret
		var enabled int
		if err := rows.Scan(&sec.ID, &sec.Label, &sec.SecretKey, &sec.CreatedAt,
			&enabled, &sec.MaxConns, &sec.MaxIPs, &sec.QuotaBytes,
			&sec.RateLimitUpBps, &sec.RateLimitDownBps,
			&sec.ExpiresAt, &sec.Notes, &sec.Tags, &sec.ArchivedAt,
			&sec.TrafficIn, &sec.TrafficOut); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		sec.Enabled = intToBool(enabled)
		secrets = append(secrets, sec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate secrets: %w", err)
	}
	return secrets, nil
}

// ListAllTags returns all unique tags across all secrets. extends expiry for multiple labels by setting a new date.
func (s *SecretStore) BulkExtendExpiry(ctx context.Context, labels []string, expiresAt string, reenable bool) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	updated := 0
	for _, label := range labels {
		q := "UPDATE secrets SET expires_at = ?"
		args := []any{expiresAt}
		if reenable {
			q += ", enabled = 1"
		}
		q += " WHERE label = ?"
		args = append(args, label)
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			continue
		}
		n, _ := res.RowsAffected()
		updated += int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
}

// BulkRotateKeys rotates keys for multiple labels.
func (s *SecretStore) BulkRotateKeys(ctx context.Context, labels []string, keys map[string]string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	updated := 0
	for _, label := range labels {
		key, ok := keys[label]
		if !ok {
			continue
		}
		res, err := tx.ExecContext(ctx, "UPDATE secrets SET secret_key = ? WHERE label = ?", key, label)
		if err != nil {
			continue
		}
		n, _ := res.RowsAffected()
		updated += int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
}

// Search returns secrets matching a text query in label or notes.
func (s *SecretStore) Search(ctx context.Context, query string) ([]model.Secret, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes,
		       s.rate_limit_up_bps, s.rate_limit_down_bps, s.expires_at, s.notes,
		       s.tags, s.archived_at,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN (SELECT label, SUM(bytes_in) AS bytes_in, SUM(bytes_out) AS bytes_out FROM traffic_user GROUP BY label) t ON s.label = t.label
		WHERE s.label LIKE '%' || ? || '%' OR s.notes LIKE '%' || ? || '%'
		ORDER BY s.id
	`, query, query)
	if err != nil {
		return nil, fmt.Errorf("search secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSecrets(rows)
}

// Top returns secrets ordered by total traffic descending.
func (s *SecretStore) Top(ctx context.Context, limit int) ([]model.Secret, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.label, s.secret_key, s.created_at, s.enabled,
		       s.max_conns, s.max_ips, s.quota_bytes,
		       s.rate_limit_up_bps, s.rate_limit_down_bps, s.expires_at, s.notes,
		       s.tags, s.archived_at,
		       COALESCE(t.bytes_in, 0), COALESCE(t.bytes_out, 0)
		FROM secrets s
		LEFT JOIN (SELECT label, SUM(bytes_in) AS bytes_in, SUM(bytes_out) AS bytes_out FROM traffic_user GROUP BY label) t ON s.label = t.label
		ORDER BY (COALESCE(t.bytes_in, 0) + COALESCE(t.bytes_out, 0)) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("top secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSecrets(rows)
}

// ListAllTags returns all unique tags across all secrets.
func (s *SecretStore) ListAllTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT tags FROM secrets WHERE tags IS NOT NULL AND tags != '' AND tags != '[]'")
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]bool)
	var tags []string
	for rows.Next() {
		var tagStr string
		if err := rows.Scan(&tagStr); err != nil {
			continue
		}
		var parsed []string
		if err := json.Unmarshal([]byte(tagStr), &parsed); err != nil {
			continue
		}
		for _, t := range parsed {
			t = strings.TrimSpace(t)
			if t != "" && !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}
	return tags, nil
}

// LabelsByTag returns labels of secrets that have the given tag.
func (s *SecretStore) LabelsByTag(ctx context.Context, tag string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT label FROM secrets
		WHERE EXISTS (SELECT 1 FROM json_each(tags) WHERE json_each.value = ?)
		ORDER BY id
	`, tag)
	if err != nil {
		return nil, fmt.Errorf("labels by tag %s: %w", tag, err)
	}
	defer func() { _ = rows.Close() }()

	var labels []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			continue
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate labels by tag %s: %w", tag, err)
	}
	return labels, nil
}

// BulkToggleEnabled enables or disables multiple secrets by labels.
// Never disables the last enabled secret.
func (s *SecretStore) BulkToggleEnabled(ctx context.Context, labels []string, enable bool) (int, error) {
	enabled := 0
	if enable {
		enabled = 1
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if !enable {
		var currentEnabled int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets WHERE enabled = 1").Scan(&currentEnabled); err != nil {
			return 0, fmt.Errorf("count enabled: %w", err)
		}
		var toDisable int
		for _, label := range labels {
			var isCurrentlyEnabled int
			if err := tx.QueryRowContext(ctx, "SELECT enabled FROM secrets WHERE label = ?", label).Scan(&isCurrentlyEnabled); err != nil {
				continue
			}
			if isCurrentlyEnabled == 1 {
				toDisable++
			}
		}
		if toDisable >= currentEnabled {
			return 0, fmt.Errorf("cannot disable: would leave no enabled secrets")
		}
	}

	updated := 0
	for _, label := range labels {
		res, err := tx.ExecContext(ctx, "UPDATE secrets SET enabled = ? WHERE label = ?", enabled, label)
		if err != nil {
			continue
		}
		n, _ := res.RowsAffected()
		updated += int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
}

// BulkSetLimits sets the same limits for multiple secrets by labels.
func (s *SecretStore) BulkSetLimits(ctx context.Context, labels []string, maxConns, maxIPs int, quotaBytes int64, expiresAt string, rateLimitUpBps, rateLimitDownBps int64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	updated := 0
	for _, label := range labels {
		var sec model.Secret
		var enabled int
		if err := tx.QueryRowContext(ctx,
			"SELECT id, label, secret_key, created_at, enabled, max_conns, max_ips, quota_bytes, rate_limit_up_bps, rate_limit_down_bps, expires_at, notes, tags, archived_at FROM secrets WHERE label = ?",
			label,
		).Scan(&sec.ID, &sec.Label, &sec.SecretKey, &sec.CreatedAt, &enabled,
			&sec.MaxConns, &sec.MaxIPs, &sec.QuotaBytes, &sec.RateLimitUpBps, &sec.RateLimitDownBps,
			&sec.ExpiresAt, &sec.Notes, &sec.Tags, &sec.ArchivedAt); err != nil {
			continue
		}
		_ = enabled // scanned as int per convention, not used in UPDATE
		if maxConns >= 0 {
			sec.MaxConns = maxConns
		}
		if maxIPs >= 0 {
			sec.MaxIPs = maxIPs
		}
		if quotaBytes >= 0 {
			sec.QuotaBytes = quotaBytes
		}
		if rateLimitUpBps >= 0 {
			sec.RateLimitUpBps = rateLimitUpBps
		}
		if rateLimitDownBps >= 0 {
			sec.RateLimitDownBps = rateLimitDownBps
		}
		if expiresAt != "" {
			sec.ExpiresAt = expiresAt
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE secrets SET max_conns = ?, max_ips = ?, quota_bytes = ?, rate_limit_up_bps = ?, rate_limit_down_bps = ?, expires_at = ? WHERE label = ?",
			sec.MaxConns, sec.MaxIPs, sec.QuotaBytes, sec.RateLimitUpBps, sec.RateLimitDownBps, sec.ExpiresAt, sec.Label); err == nil {
			updated++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
}
