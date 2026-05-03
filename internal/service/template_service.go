package service

import (
	"context"
	"fmt"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
)

// TemplateService handles secret template business logic.
type TemplateService struct {
	templates *store.TemplateStore
	secrets   *store.SecretStore
}

// NewTemplateService creates a new TemplateService.
func NewTemplateService(templates *store.TemplateStore, secrets *store.SecretStore) *TemplateService {
	return &TemplateService{templates: templates, secrets: secrets}
}

// List returns all templates.
func (s *TemplateService) List(ctx context.Context) ([]model.SecretTemplate, error) {
	return s.templates.List(ctx)
}

// Get returns a template by name.
func (s *TemplateService) Get(ctx context.Context, name string) (*model.SecretTemplate, error) {
	return s.templates.GetByName(ctx, name)
}

// Create adds a new template.
func (s *TemplateService) Create(ctx context.Context, t *model.SecretTemplate) error {
	if t.Name == "" {
		return fmt.Errorf("template name is required")
	}
	existing, err := s.templates.GetByName(ctx, t.Name)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("template '%s' already exists", t.Name)
	}
	return s.templates.Create(ctx, t)
}

// Delete removes a template.
func (s *TemplateService) Delete(ctx context.Context, name string) error {
	existing, err := s.templates.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("template '%s' not found", name)
	}
	return s.templates.Delete(ctx, name)
}

// ApplyToSecret applies a template's limits to a secret.
func (s *TemplateService) ApplyToSecret(ctx context.Context, templateName, secretLabel string) error {
	tmpl, err := s.templates.GetByName(ctx, templateName)
	if err != nil {
		return err
	}
	if tmpl == nil {
		return fmt.Errorf("template '%s' not found", templateName)
	}

	sec, err := s.secrets.GetByLabel(ctx, secretLabel)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", secretLabel)
	}

	sec.MaxConns = tmpl.MaxConns
	sec.MaxIPs = tmpl.MaxIPs
	sec.QuotaBytes = tmpl.QuotaBytes
	if tmpl.ExpiresDays > 0 {
		sec.ExpiresAt = time.Now().AddDate(0, 0, tmpl.ExpiresDays).Format("2006-01-02")
	}
	return s.secrets.Update(ctx, sec)
}
