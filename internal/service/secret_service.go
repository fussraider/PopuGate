package service

import (
	"context"
	"fmt"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/qrutil"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

// SecretService handles secret business logic.
type SecretService struct {
	secrets *store.SecretStore
}

// NewSecretService creates a new SecretService.
func NewSecretService(secrets *store.SecretStore) *SecretService {
	return &SecretService{secrets: secrets}
}

// List returns all secrets with traffic data.
func (s *SecretService) List(ctx context.Context) ([]model.Secret, error) {
	return s.secrets.List(ctx)
}

// Get returns a single secret by label.
func (s *SecretService) Get(ctx context.Context, label string) (*model.Secret, error) {
	return s.secrets.GetByLabel(ctx, label)
}

// Add creates a new secret. If secretKey is empty, generates one automatically.
func (s *SecretService) Add(ctx context.Context, label, secretKey string) (*model.Secret, error) {
	if err := model.ValidateLabel(label); err != nil {
		return nil, err
	}

	// Check max count
	count, err := s.secrets.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count secrets: %w", err)
	}
	if count >= model.MaxSecrets {
		return nil, fmt.Errorf("maximum %d secrets reached", model.MaxSecrets)
	}

	// Check duplicate
	existing, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("secret '%s' already exists", label)
	}

	// Generate or validate key
	if secretKey == "" {
		secretKey, err = telemt.GenerateSecret()
		if err != nil {
			return nil, fmt.Errorf("generate secret: %w", err)
		}
	} else {
		if !telemt.ValidateSecretKey(secretKey) {
			return nil, fmt.Errorf("secret key must be 32 hex characters")
		}
	}

	sec := &model.Secret{
		Label:     label,
		SecretKey: secretKey,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
	}

	if err := s.secrets.Create(ctx, sec); err != nil {
		return nil, fmt.Errorf("create secret: %w", err)
	}

	return sec, nil
}

// Remove deletes a secret. Refuses if it's the last enabled one.
func (s *SecretService) Remove(ctx context.Context, label string, force bool) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}

	if sec.Enabled && !force {
		enabled, err := s.secrets.CountEnabled(ctx)
		if err != nil {
			return err
		}
		if enabled <= 1 {
			return fmt.Errorf("cannot remove the last enabled secret (use force=true to override)")
		}
	}

	return s.secrets.Delete(ctx, label)
}

// Rotate generates a new key for an existing secret, preserving the label and limits.
func (s *SecretService) Rotate(ctx context.Context, label string) (*model.Secret, error) {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return nil, err
	}
	if sec == nil {
		return nil, fmt.Errorf("secret '%s' not found", label)
	}

	newKey, err := telemt.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}

	sec.SecretKey = newKey
	if err := s.secrets.Update(ctx, sec); err != nil {
		return nil, fmt.Errorf("update secret: %w", err)
	}

	return sec, nil
}

// Toggle enables or disables a secret.
func (s *SecretService) Toggle(ctx context.Context, label string, enable bool) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}

	// If disabling, check we're not disabling the last enabled secret
	if !enable {
		enabled, err := s.secrets.CountEnabled(ctx)
		if err != nil {
			return err
		}
		if enabled <= 1 {
			return fmt.Errorf("cannot disable the last enabled secret")
		}
	}

	sec.Enabled = enable
	return s.secrets.Update(ctx, sec)
}

// SetLimits updates per-user limits for a secret.
func (s *SecretService) SetLimits(ctx context.Context, label string, maxConns, maxIPs int, quotaBytes int64, expiresAt string) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}

	const maxLimit = 1_000_000

	if maxConns >= 0 {
		if maxConns > maxLimit {
			return fmt.Errorf("max_conns cannot exceed %d", maxLimit)
		}
		sec.MaxConns = maxConns
	}
	if maxIPs >= 0 {
		if maxIPs > maxLimit {
			return fmt.Errorf("max_ips cannot exceed %d", maxLimit)
		}
		sec.MaxIPs = maxIPs
	}
	if quotaBytes >= 0 {
		sec.QuotaBytes = quotaBytes
	}
	if expiresAt != "" {
		sec.ExpiresAt = expiresAt
	}

	return s.secrets.Update(ctx, sec)
}

// GetLink returns the proxy link for a secret.
func (s *SecretService) GetLink(ctx context.Context, label, serverIP string, port int, maskingEnabled bool, domain string) (*model.SecretWithLink, error) {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return nil, err
	}
	if sec == nil {
		return nil, fmt.Errorf("secret '%s' not found", label)
	}

	fullSecret := telemt.BuildFakeTLSSecret(sec.SecretKey, domain, maskingEnabled)
	tgLink := telemt.BuildProxyLink(serverIP, port, fullSecret)
	webLink := telemt.BuildWebLink(serverIP, port, fullSecret)

	return &model.SecretWithLink{
		Secret:  *sec,
		TGLink:  tgLink,
		WebLink: webLink,
	}, nil
}

// ResetTraffic resets traffic for a specific user.
func (s *SecretService) ResetTraffic(ctx context.Context, label string) error {
	return s.secrets.ResetTraffic(ctx, label)
}

// ResetAllTraffic resets traffic for all users.
func (s *SecretService) ResetAllTraffic(ctx context.Context) error {
	return s.secrets.ResetAllTraffic(ctx)
}

// UpdateNotes updates the notes/description for a secret.
func (s *SecretService) UpdateNotes(ctx context.Context, label, notes string) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}

	sec.Notes = notes
	return s.secrets.Update(ctx, sec)
}

// GetQRCode generates a QR code PNG for a secret's proxy link.
func (s *SecretService) GetQRCode(ctx context.Context, label, serverIP string, port int, maskingEnabled bool, domain string, size int) ([]byte, error) {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return nil, err
	}
	if sec == nil {
		return nil, fmt.Errorf("secret '%s' not found", label)
	}

	fullSecret := telemt.BuildFakeTLSSecret(sec.SecretKey, domain, maskingEnabled)
	webLink := telemt.BuildWebLink(serverIP, port, fullSecret)

	return qrutil.GeneratePNG(webLink, size)
}

// GetEnabledLabels returns labels of all enabled secrets.
func (s *SecretService) GetEnabledLabels(ctx context.Context) ([]string, error) {
	return s.secrets.ListEnabledLabels(ctx)
}

// Rename changes a secret's label.
func (s *SecretService) Rename(ctx context.Context, oldLabel, newLabel string) error {
	if err := model.ValidateLabel(newLabel); err != nil {
		return fmt.Errorf("invalid new label: %w", err)
	}

	existing, err := s.secrets.GetByLabel(ctx, oldLabel)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("secret '%s' not found", oldLabel)
	}

	dup, err := s.secrets.GetByLabel(ctx, newLabel)
	if err != nil {
		return err
	}
	if dup != nil {
		return fmt.Errorf("secret '%s' already exists", newLabel)
	}

	return s.secrets.RenameLabel(ctx, oldLabel, newLabel)
}

// Extend sets a new expiry for a secret. If days > 0, it sets expiry to now+days
// and optionally re-enables the secret.
func (s *SecretService) Extend(ctx context.Context, label string, days int) error {
	if days <= 0 {
		return fmt.Errorf("days must be positive")
	}

	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}

	expiresAt := time.Now().AddDate(0, 0, days).Format(time.RFC3339)
	reenable := !sec.Enabled
	return s.secrets.ExtendExpiry(ctx, label, expiresAt, reenable)
}

// DisableExpired disables all secrets whose expiry is in the past.
// Returns the count of disabled secrets.
func (s *SecretService) DisableExpired(ctx context.Context) (int, error) {
	return s.secrets.DisableExpired(ctx)
}

// SetTags updates the tags for a secret.
func (s *SecretService) SetTags(ctx context.Context, label, tags string) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}
	return s.secrets.UpdateTags(ctx, label, tags)
}

// Archive marks a secret as archived.
func (s *SecretService) Archive(ctx context.Context, label string) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}
	return s.secrets.Archive(ctx, label)
}

// Unarchive removes the archived status from a secret.
func (s *SecretService) Unarchive(ctx context.Context, label string) error {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return err
	}
	if sec == nil {
		return fmt.Errorf("secret '%s' not found", label)
	}
	return s.secrets.Unarchive(ctx, label)
}

// Clone creates a copy of a secret with a new label and key.
func (s *SecretService) Clone(ctx context.Context, srcLabel, newLabel string) (*model.Secret, error) {
	if err := model.ValidateLabel(newLabel); err != nil {
		return nil, err
	}
	existing, err := s.secrets.GetByLabel(ctx, newLabel)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("secret '%s' already exists", newLabel)
	}

	count, err := s.secrets.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count >= model.MaxSecrets {
		return nil, fmt.Errorf("maximum %d secrets reached", model.MaxSecrets)
	}

	// Verify source exists
	src, err := s.secrets.GetByLabel(ctx, srcLabel)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, fmt.Errorf("secret '%s' not found", srcLabel)
	}

	newKey, err := telemt.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}

	return s.secrets.CloneSecret(ctx, srcLabel, newLabel, newKey)
}

// BulkExtend extends expiry for multiple secrets.
func (s *SecretService) BulkExtend(ctx context.Context, labels []string, days int) (int, error) {
	if days <= 0 {
		return 0, fmt.Errorf("days must be positive")
	}
	expiresAt := time.Now().AddDate(0, 0, days).Format(time.RFC3339)
	return s.secrets.BulkExtendExpiry(ctx, labels, expiresAt, true)
}

// BulkRotate rotates keys for multiple secrets.
func (s *SecretService) BulkRotate(ctx context.Context, labels []string) (int, []string, error) {
	keys := make(map[string]string)
	for _, label := range labels {
		key, err := telemt.GenerateSecret()
		if err != nil {
			return 0, nil, fmt.Errorf("generate key for %s: %w", label, err)
		}
		keys[label] = key
	}
	updated, err := s.secrets.BulkRotateKeys(ctx, labels, keys)
	return updated, labels, err
}

// Search returns secrets matching a text query.
func (s *SecretService) Search(ctx context.Context, query string) ([]model.Secret, error) {
	if query == "" {
		return s.List(ctx)
	}
	return s.secrets.Search(ctx, query)
}

// Top returns secrets ordered by traffic.
func (s *SecretService) Top(ctx context.Context, limit int) ([]model.Secret, error) {
	return s.secrets.Top(ctx, limit)
}

// ExportJSON returns all secrets as a JSON-serializable slice.
func (s *SecretService) ExportJSON(ctx context.Context) ([]model.Secret, error) {
	return s.secrets.List(ctx)
}

// ImportSecrets creates multiple secrets from an import list.
func (s *SecretService) ImportSecrets(ctx context.Context, entries []model.Secret) (int, []string, error) {
	count, err := s.secrets.Count(ctx)
	if err != nil {
		return 0, nil, err
	}
	if count+len(entries) > model.MaxSecrets {
		return 0, nil, fmt.Errorf("import would exceed max %d secrets", model.MaxSecrets)
	}

	var created []string
	imported := 0
	for _, e := range entries {
		if e.Label == "" {
			continue
		}
		if err := model.ValidateLabel(e.Label); err != nil {
			continue
		}
		existing, err := s.secrets.GetByLabel(ctx, e.Label)
		if err != nil {
			continue
		}
		if existing != nil {
			continue
		}
		if e.SecretKey == "" {
			e.SecretKey, _ = telemt.GenerateSecret()
		} else if !telemt.ValidateSecretKey(e.SecretKey) {
			continue
		}
		e.Enabled = true
		e.CreatedAt = time.Now().Unix()
		if err := s.secrets.Create(ctx, &e); err == nil {
			imported++
			created = append(created, e.Label)
		}
	}
	return imported, created, nil
}

// ListByTag returns secrets matching the given tag.
func (s *SecretService) ListByTag(ctx context.Context, tag string) ([]model.Secret, error) {
	return s.secrets.ListByTag(ctx, tag)
}

// ListAllTags returns all unique tags.
func (s *SecretService) ListAllTags(ctx context.Context) ([]string, error) {
	return s.secrets.ListAllTags(ctx)
}

// LabelsByTag resolves a tag to its constituent labels.
func (s *SecretService) LabelsByTag(ctx context.Context, tag string) ([]string, error) {
	if tag == "" {
		return nil, fmt.Errorf("tag must not be empty")
	}
	return s.secrets.LabelsByTag(ctx, tag)
}

// BulkToggle enables or disables multiple secrets.
func (s *SecretService) BulkToggle(ctx context.Context, labels []string, enable bool) (int, error) {
	if !enable {
		enabled, err := s.secrets.CountEnabled(ctx)
		if err != nil {
			return 0, err
		}
		if enabled <= len(labels) {
			return 0, fmt.Errorf("cannot disable all enabled secrets")
		}
	}
	return s.secrets.BulkToggleEnabled(ctx, labels, enable)
}

// BulkSetLimits sets the same limits for multiple secrets.
func (s *SecretService) BulkSetLimits(ctx context.Context, labels []string, maxConns, maxIPs int, quotaBytes int64, expiresAt string) (int, error) {
	return s.secrets.BulkSetLimits(ctx, labels, maxConns, maxIPs, quotaBytes, expiresAt)
}
