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
		Secret:   *sec,
		TGLink:   tgLink,
		WebLink:  webLink,
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
