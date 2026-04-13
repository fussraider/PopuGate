package model

import (
	"fmt"
	"regexp"
	"time"
)

var labelRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// Secret represents an MTProto proxy secret key with per-user limits.
type Secret struct {
	ID         int64  `json:"id" db:"id"`
	Label      string `json:"label" db:"label"`
	SecretKey  string `json:"secret_key" db:"secret_key"`
	CreatedAt  int64  `json:"created_at" db:"created_at"`
	Enabled    bool   `json:"enabled" db:"enabled"`
	MaxConns   int    `json:"max_conns" db:"max_conns"`
	MaxIPs     int    `json:"max_ips" db:"max_ips"`
	QuotaBytes int64  `json:"quota_bytes" db:"quota_bytes"`
	ExpiresAt  string `json:"expires_at" db:"expires_at"`
	Notes      string `json:"notes" db:"notes"`

	// Computed fields (not in DB)
	TrafficIn  int64 `json:"traffic_in,omitempty" db:"-"`
	TrafficOut int64 `json:"traffic_out,omitempty" db:"-"`
}

// ValidateLabel checks that a label is valid.
func ValidateLabel(label string) error {
	if !labelRe.MatchString(label) {
		return fmt.Errorf("label must be 1-32 chars, alphanumeric/underscore/hyphen only")
	}
	return nil
}

// IsActive returns true if the secret is enabled and not expired.
func (s *Secret) IsActive() bool {
	if !s.Enabled {
		return false
	}
	if s.ExpiresAt == "" || s.ExpiresAt == "0" {
		return true
	}
	t, err := time.Parse(time.RFC3339, s.ExpiresAt)
	if err != nil {
		// Try date-only format
		t, err = time.Parse("2006-01-02", s.ExpiresAt)
		if err != nil {
			return true // unparseable = treat as no expiry
		}
	}
	return time.Now().Before(t)
}

// IsExpired returns true if the secret has an expiry date in the past.
func (s *Secret) IsExpired() bool {
	if s.ExpiresAt == "" || s.ExpiresAt == "0" {
		return false
	}
	t, err := time.Parse(time.RFC3339, s.ExpiresAt)
	if err != nil {
		t, err = time.Parse("2006-01-02", s.ExpiresAt)
		if err != nil {
			return false
		}
	}
	return time.Now().After(t)
}

// ExpiryWarning returns true if the secret expires within the given duration.
func (s *Secret) ExpiryWarning(within time.Duration) bool {
	if s.ExpiresAt == "" || s.ExpiresAt == "0" {
		return false
	}
	t, err := time.Parse(time.RFC3339, s.ExpiresAt)
	if err != nil {
		t, err = time.Parse("2006-01-02", s.ExpiresAt)
		if err != nil {
			return false
		}
	}
	remaining := time.Until(t)
	return remaining > 0 && remaining <= within
}

// QuotaPercent returns used quota as percentage (0 if no quota set).
func (s *Secret) QuotaPercent() float64 {
	if s.QuotaBytes <= 0 {
		return 0
	}
	total := s.TrafficIn + s.TrafficOut
	return float64(total) / float64(s.QuotaBytes) * 100
}

// QuotaExceeded returns true if traffic exceeds the quota.
func (s *Secret) QuotaExceeded() bool {
	if s.QuotaBytes <= 0 {
		return false
	}
	return s.TrafficIn+s.TrafficOut >= s.QuotaBytes
}

// QuotaWarning returns true if traffic is at or above 80% of quota.
func (s *Secret) QuotaWarning() bool {
	return s.QuotaPercent() >= 80
}

// SecretWithLink extends Secret with proxy link info.
type SecretWithLink struct {
	Secret
	TGLink  string `json:"tg_link,omitempty"`
	WebLink string `json:"web_link,omitempty"`
}
