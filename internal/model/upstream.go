package model

import (
	"fmt"
	"strings"
)

// isShadowsocksURL reports whether s is a syntactically plausible ss:// URL.
// Full method/cipher validation is delegated to the telemt engine; here we only
// guard against empty or non-ss values reaching the config.
func isShadowsocksURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "ss://") && len(s) > len("ss://")
}

// UpstreamType defines the proxy upstream type.
type UpstreamType string

const (
	UpstreamDirect      UpstreamType = "direct"
	UpstreamSOCKS5      UpstreamType = "socks5"
	UpstreamSOCKS4      UpstreamType = "socks4"
	UpstreamShadowsocks UpstreamType = "shadowsocks"
)

// Upstream represents a proxy upstream configuration.
type Upstream struct {
	ID       int64        `json:"id" db:"id"`
	Name     string       `json:"name" db:"name"`
	Type     UpstreamType `json:"type" db:"type"`
	Address  string       `json:"address" db:"address"`
	Username string       `json:"username" db:"username"`
	Password string       `json:"password" db:"password"`
	URL      string       `json:"url" db:"url"` // shadowsocks ss:// URL (shadowsocks type only)
	Weight   int          `json:"weight" db:"weight"`
	Iface    string       `json:"iface" db:"iface"`
	Enabled  bool         `json:"enabled" db:"enabled"`
	// Dual-stack DC family policy (nil = auto). Both false is invalid.
	IPv4 *bool `json:"ipv4" db:"ipv4"`
	IPv6 *bool `json:"ipv6" db:"ipv6"`
	// Preferred DC family: 0 = auto (inherit), 4, or 6.
	Prefer int `json:"prefer" db:"prefer"`
	// SO_BINDTODEVICE interface pinning (direct upstreams only, Linux).
	BindToDevice string `json:"bindtodevice" db:"bindtodevice"`

	// Health fields
	LastCheckAt    int64  `json:"last_check_at" db:"last_check_at"` // unix timestamp
	LastCheckOK    *bool  `json:"last_check_ok" db:"last_check_ok"` // nil=never tested
	LatencyMs      int64  `json:"latency_ms" db:"latency_ms"`       // last test latency
	LastError      string `json:"last_error" db:"last_error"`       // last error message
	FailCount      int    `json:"fail_count" db:"fail_count"`       // consecutive failures
	AutoDisabled   bool   `json:"auto_disabled" db:"auto_disabled"`
	AutoDisabledAt int64  `json:"auto_disabled_at" db:"auto_disabled_at"`
}

// Validate checks upstream fields.
func (u *Upstream) Validate() error {
	if u.Name == "" || len(u.Name) > 32 {
		return fmt.Errorf("name must be 1-32 characters")
	}
	switch u.Type {
	case UpstreamDirect:
		// No address needed
	case UpstreamSOCKS5, UpstreamSOCKS4:
		if u.Address == "" {
			return fmt.Errorf("address is required for %s upstream", u.Type)
		}
	case UpstreamShadowsocks:
		// Shadowsocks uses an ss:// URL instead of address/credentials.
		if !isShadowsocksURL(u.URL) {
			return fmt.Errorf("a valid ss:// URL is required for shadowsocks upstream")
		}
	default:
		return fmt.Errorf("invalid upstream type: %s", u.Type)
	}
	if u.Weight < 1 || u.Weight > 100 {
		return fmt.Errorf("weight must be 1-100")
	}
	if u.IPv4 != nil && u.IPv6 != nil && !*u.IPv4 && !*u.IPv6 {
		return fmt.Errorf("ipv4 and ipv6 cannot both be false")
	}
	if u.Prefer != 0 && u.Prefer != 4 && u.Prefer != 6 {
		return fmt.Errorf("prefer must be 0 (auto), 4, or 6")
	}
	if u.BindToDevice != "" && u.Type != UpstreamDirect {
		return fmt.Errorf("bindtodevice is only supported for direct upstreams")
	}
	return nil
}

// UpstreamTestResult holds the result of a connectivity test.
type UpstreamTestResult struct {
	OK        bool   `json:"ok"`
	ExitIP    string `json:"exit_ip,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}
