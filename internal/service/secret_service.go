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
	secrets   *store.SecretStore
	instances *store.InstanceStore
	settings  *store.SettingsStore
	traffic   *store.TrafficStore
	engineAPI *EngineAPIClient
}

// NewSecretService creates a new SecretService.
func NewSecretService(secrets *store.SecretStore, instances *store.InstanceStore, settings *store.SettingsStore, traffic *store.TrafficStore) *SecretService {
	return &SecretService{secrets: secrets, instances: instances, settings: settings, traffic: traffic}
}

// SetEngineAPI wires the engine control-plane API client used to propagate quota
// resets to running engines without a container restart.
func (s *SecretService) SetEngineAPI(c *EngineAPIClient) { s.engineAPI = c }

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
func (s *SecretService) SetLimits(ctx context.Context, label string, maxConns, maxIPs int, quotaBytes int64, expiresAt string, rateLimitUpBps, rateLimitDownBps int64) error {
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
	if rateLimitUpBps >= 0 {
		sec.RateLimitUpBps = rateLimitUpBps
	}
	if rateLimitDownBps >= 0 {
		sec.RateLimitDownBps = rateLimitDownBps
	}
	if expiresAt != "" {
		sec.ExpiresAt = expiresAt
	}

	return s.secrets.Update(ctx, sec)
}

// GetLinks returns all proxy links for a secret across all accessible instances × domains.
func (s *SecretService) GetLinks(ctx context.Context, label, serverIP string) (*model.SecretWithLinks, error) {
	sec, err := s.secrets.GetByLabel(ctx, label)
	if err != nil {
		return nil, err
	}
	if sec == nil {
		return nil, fmt.Errorf("secret '%s' not found", label)
	}

	instances, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}

	links := BuildLinksForSecret(sec, instances, serverIP)

	return &model.SecretWithLinks{
		Secret: *sec,
		Links:  links,
	}, nil
}

// GetAllLinks returns proxy links for all enabled secrets.
func (s *SecretService) GetAllLinks(ctx context.Context, serverIP string) ([]model.SecretWithLinks, error) {
	secrets, err := s.secrets.List(ctx)
	if err != nil {
		return nil, err
	}

	instances, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []model.SecretWithLinks
	for _, sec := range secrets {
		if !sec.Enabled {
			continue
		}
		links := BuildLinksForSecret(&sec, instances, serverIP)
		result = append(result, model.SecretWithLinks{
			Secret: sec,
			Links:  links,
		})
	}
	return result, nil
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
	if err := s.traffic.ResetTraffic(ctx, label); err != nil {
		return err
	}
	// Propagate to the running engines so the enforcement counter drops
	// immediately, no container restart. Best-effort.
	if s.engineAPI != nil {
		if insts, err := s.instances.List(ctx); err != nil {
			engineLog.Warnf("engine quota reset: list instances: %v", err)
		} else {
			s.engineAPI.ResetLabel(ctx, insts, label)
		}
	}
	return nil
}

// ResetAllTraffic resets traffic for all users.
func (s *SecretService) ResetAllTraffic(ctx context.Context) error {
	if err := s.traffic.ResetAllTraffic(ctx); err != nil {
		return err
	}
	if s.engineAPI != nil {
		insts, ierr := s.instances.List(ctx)
		secs, serr := s.secrets.List(ctx)
		if ierr != nil || serr != nil {
			engineLog.Warnf("engine quota reset: load state: instances=%v secrets=%v", ierr, serr)
		} else {
			s.engineAPI.ResetAll(ctx, insts, secs)
		}
	}
	return nil
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

	clone := &model.Secret{
		Label:      newLabel,
		SecretKey:  newKey,
		Enabled:    src.Enabled,
		MaxConns:   src.MaxConns,
		MaxIPs:     src.MaxIPs,
		QuotaBytes: src.QuotaBytes,
		ExpiresAt:  src.ExpiresAt,
		Notes:      src.Notes,
		Tags:       src.Tags,
		CreatedAt:  time.Now().Unix(),
	}
	if err := s.secrets.Create(ctx, clone); err != nil {
		return nil, err
	}
	return clone, nil
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

// ImportResult holds the outcome of a bulk import operation.
type ImportResult struct {
	Imported []string // Labels successfully created
	Skipped  []string // Labels skipped (already exist)
	Errors   []string // Entries that failed with reason
}

// ImportSecrets creates multiple secrets from an import list.
// The top-level error is nil for best-effort imports; per-entry details are in ImportResult.
func (s *SecretService) ImportSecrets(ctx context.Context, entries []model.Secret) (*ImportResult, error) {
	count, err := s.secrets.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count+len(entries) > model.MaxSecrets {
		return nil, fmt.Errorf("import would exceed max %d secrets", model.MaxSecrets)
	}

	result := &ImportResult{}
	for _, e := range entries {
		if e.Label == "" {
			result.Errors = append(result.Errors, "entry with empty label skipped")
			continue
		}
		if err := model.ValidateLabel(e.Label); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", e.Label, err.Error()))
			continue
		}
		existing, err := s.secrets.GetByLabel(ctx, e.Label)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: lookup failed: %s", e.Label, err.Error()))
			continue
		}
		if existing != nil {
			result.Skipped = append(result.Skipped, e.Label)
			continue
		}
		if e.SecretKey == "" {
			e.SecretKey, _ = telemt.GenerateSecret()
		} else if !telemt.ValidateSecretKey(e.SecretKey) {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: invalid secret key", e.Label))
			continue
		}
		e.Enabled = true
		e.CreatedAt = time.Now().Unix()
		if err := s.secrets.Create(ctx, &e); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: create failed: %s", e.Label, err.Error()))
			continue
		}
		result.Imported = append(result.Imported, e.Label)
	}
	return result, nil
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
		totalEnabled, err := s.secrets.CountEnabled(ctx)
		if err != nil {
			return 0, err
		}
		toDisable, err := s.secrets.CountEnabledByLabels(ctx, labels)
		if err != nil {
			return 0, err
		}
		if toDisable >= totalEnabled {
			return 0, fmt.Errorf("cannot disable all enabled secrets")
		}
	}
	return s.secrets.BulkToggleEnabled(ctx, labels, enable)
}

// BulkSetLimits sets the same limits for multiple secrets.
func (s *SecretService) BulkSetLimits(ctx context.Context, labels []string, maxConns, maxIPs int, quotaBytes int64, expiresAt string, rateLimitUpBps, rateLimitDownBps int64) (int, error) {
	return s.secrets.BulkSetLimits(ctx, labels, maxConns, maxIPs, quotaBytes, expiresAt, rateLimitUpBps, rateLimitDownBps)
}
