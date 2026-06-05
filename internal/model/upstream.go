package model

import "fmt"

// UpstreamType defines the proxy upstream type.
type UpstreamType string

const (
	UpstreamDirect UpstreamType = "direct"
	UpstreamSOCKS5 UpstreamType = "socks5"
	UpstreamSOCKS4 UpstreamType = "socks4"
)

// Upstream represents a proxy upstream configuration.
type Upstream struct {
	ID       int64        `json:"id" db:"id"`
	Name     string       `json:"name" db:"name"`
	Type     UpstreamType `json:"type" db:"type"`
	Address  string       `json:"address" db:"address"`
	Username string       `json:"username" db:"username"`
	Password string       `json:"password" db:"password"`
	Weight   int          `json:"weight" db:"weight"`
	Iface    string       `json:"iface" db:"iface"`
	Enabled  bool         `json:"enabled" db:"enabled"`

	// Health fields
	LastCheckAt int64  `json:"last_check_at" db:"last_check_at"` // unix timestamp
	LastCheckOK *bool  `json:"last_check_ok" db:"last_check_ok"` // nil=never tested
	LatencyMs   int64  `json:"latency_ms" db:"latency_ms"`       // last test latency
	LastError   string `json:"last_error" db:"last_error"`       // last error message
	FailCount      int          `json:"fail_count" db:"fail_count"`       // consecutive failures
	AutoDisabled   bool         `json:"auto_disabled" db:"auto_disabled"`
	AutoDisabledAt int64        `json:"auto_disabled_at" db:"auto_disabled_at"`
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
	default:
		return fmt.Errorf("invalid upstream type: %s", u.Type)
	}
	if u.Weight < 1 || u.Weight > 100 {
		return fmt.Errorf("weight must be 1-100")
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
