package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Settings holds all application configuration.
// Maps to the key-value settings table in SQLite.
type Settings struct {
	// Proxy
	ProxyPort                 int    `json:"proxy_port"`
	ProxyMetricsPort          int    `json:"proxy_metrics_port"`
	ProxyDomain               string `json:"proxy_domain"`
	ProxyConcurrency          int    `json:"proxy_concurrency"`
	ProxyCPUs                 string `json:"proxy_cpus"`
	ProxyMemory               string `json:"proxy_memory"`
	CustomIP                  string `json:"custom_ip"`
	FakeCertLen               int    `json:"fake_cert_len"`
	ProxyProtocol             bool   `json:"proxy_protocol"`
	ProxyProtocolTrustedCIDRs string `json:"proxy_protocol_trusted_cidrs"`

	// Ad tag
	AdTag string `json:"ad_tag"`

	// Geo-blocking
	GeoblockMode       string `json:"geoblock_mode"`
	BlocklistCountries string `json:"blocklist_countries"`

	// Traffic masking
	MaskingEnabled   bool   `json:"masking_enabled"`
	MaskingHost      string `json:"masking_host"`
	MaskingPort      int    `json:"masking_port"`
	UnknownSNIAction string `json:"unknown_sni_action"`

	// Telegram
	TelegramEnabled       bool   `json:"telegram_enabled"`
	TelegramBotToken      string `json:"telegram_bot_token"`
	TelegramChatID        string `json:"telegram_chat_id"`
	TelegramInterval      int    `json:"telegram_interval"`
	TelegramAlertsEnabled bool   `json:"telegram_alerts_enabled"`
	TelegramServerLabel   string `json:"telegram_server_label"`

	// Auto-update
	AutoUpdateEnabled bool `json:"auto_update_enabled"`

	// Replication
	ReplicationEnabled         bool   `json:"replication_enabled"`
	ReplicationRole            string `json:"replication_role"`
	ReplicationSyncInterval    int    `json:"replication_sync_interval"`
	ReplicationSSHPort         int    `json:"replication_ssh_port"`
	ReplicationSSHUser         string `json:"replication_ssh_user"`
	ReplicationDeleteExtra     bool   `json:"replication_delete_extra"`
	ReplicationSSHKeyPath      string `json:"replication_ssh_key_path"`
	ReplicationExclude         string `json:"replication_exclude"`
	ReplicationRestartOnChange bool   `json:"replication_restart_on_change"`
	ReplicationLog             string `json:"replication_log"`
	Debug                      bool   `json:"debug"`
}

// DefaultSSHKeyPath returns the default SSH key location.
func DefaultSSHKeyPath() string {
	return filepath.Join(InstallDir, ".ssh/id_ed25519")
}

// DefaultSSHLogPath returns the default replication log location.
func DefaultSSHLogPath() string {
	return filepath.Join(InstallDir, "replication.log")
}

// SSHKeyPath returns current SSH key path or default if empty.
func (s *Settings) SSHKeyPath() string {
	if s.ReplicationSSHKeyPath == "" {
		return DefaultSSHKeyPath()
	}
	return s.ReplicationSSHKeyPath
}

// SSHLogPath returns current replication log path or default if empty.
func (s *Settings) SSHLogPath() string {
	if s.ReplicationLog == "" {
		return DefaultSSHLogPath()
	}
	return s.ReplicationLog
}

// Defaults returns a Settings struct populated with default values.
func DefaultSettings() Settings {
	return Settings{
		ProxyPort:                  443,
		ProxyMetricsPort:           9090,
		ProxyDomain:                "cloudflare.com",
		ProxyConcurrency:           8192,
		FakeCertLen:                2048,
		GeoblockMode:               "blacklist",
		MaskingEnabled:             true,
		MaskingPort:                443,
		UnknownSNIAction:           "mask",
		TelegramInterval:           6,
		TelegramAlertsEnabled:      true,
		TelegramServerLabel:        "PopuGate",
		AutoUpdateEnabled:          true,
		ReplicationRole:            "standalone",
		ReplicationSyncInterval:    60,
		ReplicationSSHPort:         22,
		ReplicationSSHUser:         "root",
		ReplicationDeleteExtra:     true,
		ReplicationSSHKeyPath:      "",
		ReplicationExclude:         "relay_stats,backups,connection.log,.ssh,settings.db,replication",
		ReplicationRestartOnChange: true,
		ReplicationLog:             "",
		Debug:                      false,
	}
}

// Validate corrects invalid/missing values to defaults.
func (s *Settings) Validate() {
	if s.ProxyPort < 1 || s.ProxyPort > 65535 {
		s.ProxyPort = 443
	}
	if s.ProxyMetricsPort < 1 || s.ProxyMetricsPort > 65535 {
		s.ProxyMetricsPort = 9090
	}
	if s.ProxyConcurrency < 1 {
		s.ProxyConcurrency = 8192
	}
	if s.FakeCertLen < 512 {
		s.FakeCertLen = 2048
	}
	if s.MaskingPort < 1 || s.MaskingPort > 65535 {
		s.MaskingPort = 443
	}
	if s.UnknownSNIAction != "drop" {
		s.UnknownSNIAction = "mask"
	}
	if s.GeoblockMode != "whitelist" {
		s.GeoblockMode = "blacklist"
	}
	if s.TelegramInterval < 1 {
		s.TelegramInterval = 6
	}
	if s.ReplicationRole != "standalone" && s.ReplicationRole != "master" && s.ReplicationRole != "slave" {
		s.ReplicationRole = "standalone"
	}
	if s.ReplicationSyncInterval < 10 {
		s.ReplicationSyncInterval = 60
	}
	if s.ReplicationSSHPort < 1 || s.ReplicationSSHPort > 65535 {
		s.ReplicationSSHPort = 22
	}
}

// Constants for the application.
const (
	ContainerName    = "popugate"
	DockerImageBase  = "popugate-telemt"
	DefaultTelemtVer = "3.3.39"
	DefaultTelemtRef = "bc69153"
	DefaultTelemtURL = "https://github.com/telemt/telemt.git"
	RegistryImage    = "ghcr.io/fussraider/popugate-telemt"
	GitHubRepo       = "fussraider/PopuGate"
	MaxSecrets       = 1000
	SecretKeyLen     = 32 // hex chars
)

// TelemtVersion returns the telemt version from env or default.
func TelemtVersion() string {
	if v := os.Getenv("TELEMT_VERSION"); v != "" {
		return v
	}
	return DefaultTelemtVer
}

// TelemtCommit returns the telemt commit/ref from env or default.
func TelemtCommit() string {
	if v := os.Getenv("TELEMT_COMMIT"); v != "" {
		return v
	}
	return DefaultTelemtRef
}

// TelemtRepo returns the telemt repository URL from env or default.
func TelemtRepo() string {
	if v := os.Getenv("TELEMT_REPO"); v != "" {
		return v
	}
	return DefaultTelemtURL
}

// Version is overridden at build time via -ldflags "-X main.version=..."
var Version = "dev"

// Commit is the full git SHA, overridden at build time via -ldflags "-X main.commit=..."
var Commit = "unknown"

// VersionURL returns a GitHub URL for the current version: release page for tags,
// commit page for SHAs, or the repo root as fallback.
func VersionURL() string {
	if strings.HasPrefix(Version, "v") {
		return fmt.Sprintf("https://github.com/%s/releases/tag/%s", GitHubRepo, Version)
	}
	if Commit != "unknown" && Commit != "" {
		return fmt.Sprintf("https://github.com/%s/commit/%s", GitHubRepo, Commit)
	}
	return fmt.Sprintf("https://github.com/%s", GitHubRepo)
}

// InstallDir is the base data directory. Overridden at startup from
// the POPUGATE_DATA_DIR env var or the binary's directory.
var InstallDir = "/opt/popugate"

// ConfigDir returns the engine config directory.
func ConfigDir() string { return filepath.Join(InstallDir, "mtproxy") }

// ProxyStatus represents the current state of the proxy.
type ProxyStatus struct {
	Running       bool             `json:"running"`
	Port          int              `json:"port"`
	Uptime        string           `json:"uptime,omitempty"`
	UptimeSeconds int64            `json:"uptime_seconds,omitempty"`
	ContainerID   string           `json:"container_id,omitempty"`
	StartedAt     time.Time        `json:"started_at,omitempty"`
	ConnsCurrent  int              `json:"conns_current,omitempty"`
	ConnsTotal    int64            `json:"conns_total,omitempty"`
	TrafficIn     int64            `json:"traffic_in,omitempty"`
	TrafficOut    int64            `json:"traffic_out,omitempty"`
	Instances     []InstanceStatus `json:"instances,omitempty"`
}

// InstanceStatus for multi-port instances.
type InstanceStatus struct {
	Port    int    `json:"port"`
	Running bool   `json:"running"`
	Label   string `json:"label"`
}
