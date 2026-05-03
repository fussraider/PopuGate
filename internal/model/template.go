package model

// SecretTemplate represents a reusable preset for secret limits.
type SecretTemplate struct {
	ID          int64  `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	MaxConns    int    `json:"max_conns" db:"max_conns"`
	MaxIPs      int    `json:"max_ips" db:"max_ips"`
	QuotaBytes  int64  `json:"quota_bytes" db:"quota_bytes"`
	ExpiresDays int    `json:"expires_days" db:"expires_days"`
	Notes       string `json:"notes" db:"notes"`
}
