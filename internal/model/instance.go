package model

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Instance represents a fully independent proxy instance with its own port, domains, and config.
type Instance struct {
	ID          int64  `json:"id" db:"id"`
	Port        int    `json:"port" db:"port"`
	MetricsPort int    `json:"metrics_port" db:"metrics_port"`
	Enabled     bool   `json:"enabled" db:"enabled"`
	Label       string `json:"label" db:"label"`
	// Per-instance proxy configuration
	TLSDomain     string `json:"tls_domain" db:"tls_domain"`           // Primary masking domain (required)
	TLSDomains    string `json:"tls_domains" db:"tls_domains"`         // Additional domains (JSON array)
	FakeTLS       bool   `json:"fake_tls" db:"fake_tls"`               // Enable FakeTLS masking
	MaskHost      string `json:"mask_host" db:"mask_host"`             // Where to proxy non-MTProto traffic
	MaskPort      int    `json:"mask_port" db:"mask_port"`             // Port for mask_host
	Tags          string `json:"tags" db:"tags"`                       // Access tags (JSON array)
	TCPMSSEnabled bool   `json:"tcp_mss_enabled" db:"tcp_mss_enabled"` // Enable TCPMSS clamping
	TCPMSS        int    `json:"tcp_mss" db:"tcp_mss"`                 // MSS value (1-1460, default 88)
	TLSFronting   bool   `json:"tls_fronting" db:"tls_fronting"`       // Enable TLS fronting content serving
}

// Validate checks instance fields.
func (i *Instance) Validate() error {
	if i.Port < 1 || i.Port > 65535 {
		return fmt.Errorf("port must be 1-65535")
	}
	if i.MetricsPort < 1 || i.MetricsPort > 65535 {
		return fmt.Errorf("metrics_port must be 1-65535")
	}
	if i.TLSDomain == "" {
		return fmt.Errorf("tls_domain is required")
	}
	if i.MaskPort == 0 {
		i.MaskPort = 443
	}
	if i.MaskPort < 1 || i.MaskPort > 65535 {
		return fmt.Errorf("mask_port must be 1-65535")
	}
	if i.TCPMSS == 0 {
		i.TCPMSS = 88
	}
	if i.TCPMSSEnabled {
		if i.TCPMSS < 1 || i.TCPMSS > 1460 {
			return fmt.Errorf("tcp_mss must be 1-1460")
		}
	}
	if i.TLSFronting && !i.FakeTLS {
		return fmt.Errorf("tls_fronting requires fake_tls to be enabled")
	}
	return nil
}

// ContainerName returns the Docker container name for this instance.
func (i *Instance) ContainerName() string {
	return fmt.Sprintf("popugate-telemt-%d", i.Port)
}

// ConfigPath returns the TOML config file path for this instance.
func (i *Instance) ConfigPath() string {
	return filepath.Join(InstallDir, fmt.Sprintf("mtproxy/config-%d.toml", i.Port))
}

// GetTLSDomains parses the JSON tls_domains array.
func (i *Instance) GetTLSDomains() []string {
	if i.TLSDomains == "" || i.TLSDomains == "[]" {
		return nil
	}
	var domains []string
	_ = json.Unmarshal([]byte(i.TLSDomains), &domains)
	return domains
}

// AllDomains returns tls_domain + tls_domains combined.
func (i *Instance) AllDomains() []string {
	domains := []string{i.TLSDomain}
	if extra := i.GetTLSDomains(); len(extra) > 0 {
		domains = append(domains, extra...)
	}
	return domains
}

// GetTags parses the JSON tags array.
func (i *Instance) GetTags() []string {
	if i.Tags == "" || i.Tags == "[]" {
		return nil
	}
	var tags []string
	_ = json.Unmarshal([]byte(i.Tags), &tags)
	return tags
}

// GetMaskHost returns the custom mask host or falls back to tls_domain.
func (i *Instance) GetMaskHost() string {
	if i.MaskHost != "" {
		return i.MaskHost
	}
	return i.TLSDomain
}

// TLSFrontDirPath returns the host-side directory for TLS fronting content.
func (i *Instance) TLSFrontDirPath() string {
	return filepath.Join(InstallDir, fmt.Sprintf("mtproxy/tlsfront-%d", i.Port))
}

// TagsMatch checks if an instance is accessible by a secret based on tag matching.
// An instance without tags is accessible to all secrets.
// An instance with tags is only accessible to secrets that share at least one tag.
func TagsMatch(instanceTags, secretTags []string) bool {
	if len(instanceTags) == 0 {
		return true
	}
	for _, it := range instanceTags {
		for _, st := range secretTags {
			if it == st {
				return true
			}
		}
	}
	return false
}
