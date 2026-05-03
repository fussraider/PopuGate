package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fussraider/PopuGate/internal/model"
)

// TemplateStore handles secret template persistence.
type TemplateStore struct {
	db *sql.DB
}

// NewTemplateStore creates a new TemplateStore.
func NewTemplateStore(db *sql.DB) *TemplateStore {
	return &TemplateStore{db: db}
}

// List returns all templates.
func (s *TemplateStore) List(ctx context.Context) ([]model.SecretTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, max_conns, max_ips, quota_bytes, expires_days, notes
		FROM secret_templates ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []model.SecretTemplate
	for rows.Next() {
		var t model.SecretTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.MaxConns, &t.MaxIPs, &t.QuotaBytes, &t.ExpiresDays, &t.Notes); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// GetByName returns a template by name.
func (s *TemplateStore) GetByName(ctx context.Context, name string) (*model.SecretTemplate, error) {
	var t model.SecretTemplate
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, max_conns, max_ips, quota_bytes, expires_days, notes
		FROM secret_templates WHERE name = ?
	`, name).Scan(&t.ID, &t.Name, &t.MaxConns, &t.MaxIPs, &t.QuotaBytes, &t.ExpiresDays, &t.Notes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	return &t, nil
}

// Create inserts a new template.
func (s *TemplateStore) Create(ctx context.Context, t *model.SecretTemplate) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO secret_templates (name, max_conns, max_ips, quota_bytes, expires_days, notes)
		VALUES (?, ?, ?, ?, ?, ?)
	`, t.Name, t.MaxConns, t.MaxIPs, t.QuotaBytes, t.ExpiresDays, t.Notes)
	if err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	t.ID, _ = result.LastInsertId()
	return nil
}

// Delete removes a template by name.
func (s *TemplateStore) Delete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM secret_templates WHERE name = ?", name)
	return err
}
