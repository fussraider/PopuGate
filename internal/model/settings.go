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

	// Middle-Proxy mode (Telegram promoted channels / ad_tag). Default true.
	// Must be false to use shadowsocks upstreams (telemt rejects them in ME mode).
	UseMiddleProxy bool `json:"use_middle_proxy"`

	// Geo-blocking
	GeoblockMode       string `json:"geoblock_mode"`
	BlocklistCountries string `json:"blocklist_countries"`

	// Traffic masking
	MaskingEnabled       bool   `json:"masking_enabled"`
	MaskingHost          string `json:"masking_host"`
	MaskingPort          int    `json:"masking_port"`
	MaskingRelayMaxBytes int64  `json:"masking_relay_max_bytes"`
	UnknownSNIAction     string `json:"unknown_sni_action"`

	// Custom Telegram infrastructure URLs (for restricted regions)
	ProxySecretURL   string `json:"proxy_secret_url"`
	ProxyConfigV4URL string `json:"proxy_config_v4_url"`
	ProxyConfigV6URL string `json:"proxy_config_v6_url"`

	// Telegram
	TelegramEnabled       bool   `json:"telegram_enabled"`
	TelegramBotToken      string `json:"telegram_bot_token"`
	TelegramChatID        string `json:"telegram_chat_id"`
	TelegramInterval      int    `json:"telegram_interval"`
	TelegramAlertsEnabled bool   `json:"telegram_alerts_enabled"`
	TelegramServerLabel   string `json:"telegram_server_label"`
	WebURL                string `json:"web_url"`

	// Auto-update
	AutoUpdateEnabled    bool `json:"auto_update_enabled"`
	SecretAutoRotateDays int  `json:"secret_auto_rotate_days"`

	// Maintenance
	MaintenanceMode bool `json:"maintenance_mode"`

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

	// Backup
	BackupRetentionDays int `json:"backup_retention_days"`

	// telemt engine
	TelemtVersion string `json:"telemt_version"`
	TelemtCommit  string `json:"telemt_commit"`
	TelemtRepo    string `json:"telemt_repo"`

	// SYN limiter (engine netfilter-based SYN flood limiter, opt-in).
	// When enabled, containers are recreated with CAP_NET_ADMIN and the engine
	// installs iptables/nftables rules. Off by default (no capability granted).
	SynlimitEnabled  bool   `json:"synlimit_enabled"`
	SynlimitBackend  string `json:"synlimit_backend"`  // "nftables" | "iptables"
	SynlimitSeconds  int    `json:"synlimit_seconds"`  // window, default 60
	SynlimitHitcount int    `json:"synlimit_hitcount"` // SYNs per window before limiting, default 48
	SynlimitBurst    int    `json:"synlimit_burst"`    // burst allowance, default 1

	// Kernel Network Tuning (TCP BBR & FastOpen)
	SysctlOptimizationsEnabled bool   `json:"sysctl_optimizations_enabled"`
	OriginalQdisc              string `json:"original_qdisc"`
	OriginalCongestionControl  string `json:"original_congestion_control"`
	OriginalFastOpen           string `json:"original_fastopen"`
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
		UseMiddleProxy:             true,
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
		BackupRetentionDays:        7,
		SecretAutoRotateDays:       0,
		MaintenanceMode:            false,
		SysctlOptimizationsEnabled: false,
		SynlimitEnabled:            false,
		SynlimitBackend:            "nftables",
		SynlimitSeconds:            60,
		SynlimitHitcount:           48,
		SynlimitBurst:              1,
	}
}

// Validate corrects invalid/missing values to defaults.
func validPort(port, fallback int) int {
	if port < 1 || port > 65535 {
		return fallback
	}
	return port
}

func validEnum(val string, allowed []string, fallback string) string {
	for _, a := range allowed {
		if val == a {
			return val
		}
	}
	return fallback
}

func (s *Settings) Validate() {
	s.ProxyPort = validPort(s.ProxyPort, 443)
	s.ProxyMetricsPort = validPort(s.ProxyMetricsPort, 9090)
	if s.ProxyConcurrency < 1 {
		s.ProxyConcurrency = 8192
	}
	if s.FakeCertLen < 512 {
		s.FakeCertLen = 2048
	}
	s.MaskingPort = validPort(s.MaskingPort, 443)
	s.UnknownSNIAction = validEnum(s.UnknownSNIAction, []string{"drop", "mask"}, "mask")
	s.GeoblockMode = validEnum(s.GeoblockMode, []string{"whitelist", "blacklist"}, "blacklist")
	if s.TelegramInterval < 1 {
		s.TelegramInterval = 6
	}
	s.ReplicationRole = validEnum(s.ReplicationRole, []string{"standalone", "master", "slave"}, "standalone")
	if s.ReplicationSyncInterval < 10 {
		s.ReplicationSyncInterval = 60
	}
	s.ReplicationSSHPort = validPort(s.ReplicationSSHPort, 22)
	if s.BackupRetentionDays < 1 {
		s.BackupRetentionDays = 7
	}
	if s.SecretAutoRotateDays < 0 {
		s.SecretAutoRotateDays = 0
	}
	s.SynlimitBackend = validEnum(s.SynlimitBackend, []string{"nftables", "iptables"}, "nftables")
	if s.SynlimitSeconds < 1 {
		s.SynlimitSeconds = 60
	}
	if s.SynlimitHitcount < 1 {
		s.SynlimitHitcount = 48
	}
	if s.SynlimitBurst < 1 {
		s.SynlimitBurst = 1
	}
}

// Constants for the application.
const (
	ContainerName    = "popugate-telemt"
	DockerImageBase  = "popugate-telemt"
	DefaultTelemtVer = "3.4.22"
	DefaultTelemtRef = "ef1c06a"
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
// Always stored without "v" prefix for consistent comparisons.
var Version = "dev"

// Commit is the full git SHA, overridden at build time via -ldflags "-X main.commit=..."
var Commit = "unknown"

// SetVersion normalizes and sets the version string, stripping any "v" prefix.
func SetVersion(v string) {
	Version = strings.TrimPrefix(v, "v")
	if Version == "" {
		Version = "dev"
	}
}

// VersionTag returns the version with a "v" prefix for display (e.g. "v0.1.2").
// Returns "dev" as-is when no real version is set.
func VersionTag() string {
	if Version == "dev" || Version == "" {
		return Version
	}
	return "v" + Version
}

// VersionURL returns a GitHub URL for the current version: release page for tags,
// commit page for SHAs, or the repo root as fallback.
func VersionURL() string {
	if Version != "dev" && Version != "" {
		return fmt.Sprintf("https://github.com/%s/releases/tag/v%s", GitHubRepo, Version)
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

// TelemtBuildLogPath returns the engine build log file path (one log per
// build run, shared by manual builds and engine updates).
func TelemtBuildLogPath() string { return filepath.Join(InstallDir, "telemt_build.log") }

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

type InstanceStatus struct {
	ID                  int64  `json:"id"`
	Port                int    `json:"port"`
	Running             bool   `json:"running"`
	Label               string `json:"label"`
	TLSDomain           string `json:"tls_domain"`
	FakeTLS             bool   `json:"fake_tls"`
	Status              string `json:"status"` // "healthy", "unhealthy", "stopped"
	ContainerName       string `json:"container_name,omitempty"`
	ActivePort          int    `json:"active_port,omitempty"`
	ActiveMetricsPort   int    `json:"active_metrics_port,omitempty"`
	Draining            bool   `json:"draining"`
	MatchingSecretCount int    `json:"matching_secret_count"`
}

// ProxyLink represents a single proxy link for a specific instance+domain combination.
type ProxyLink struct {
	InstanceLabel string `json:"instance_label"`
	InstancePort  int    `json:"instance_port"`
	Domain        string `json:"domain"`
	TGLink        string `json:"tg_link"`
	WebLink       string `json:"web_link"`
}

// SecretWithLinks extends Secret with multiple proxy links (one per instance×domain).
type SecretWithLinks struct {
	Secret
	Links []ProxyLink `json:"links"`
}
