package telemt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// TelemtConfig represents the full TOML configuration for the telemt engine.
type TelemtConfig struct {
	General    GeneralConfig    `toml:"general"`
	Server     ServerConfig     `toml:"server"`
	Timeouts   TimeoutsConfig   `toml:"timeouts"`
	Censorship CensorshipConfig `toml:"censorship"`
	Access     AccessConfig     `toml:"access"`
	Upstreams  []UpstreamConfig `toml:"upstreams,omitempty"`
	Telegram   TelegramConfig   `toml:"telegram,omitempty"`
}

type GeneralConfig struct {
	PreferIPv6     bool        `toml:"prefer_ipv6"`
	FastMode       bool        `toml:"fast_mode"`
	UseMiddleProxy bool        `toml:"use_middle_proxy"`
	LogLevel       string      `toml:"log_level"`
	AdTag          string      `toml:"ad_tag,omitempty"`
	Modes          ModesConfig `toml:"modes"`
	Links          LinksConfig `toml:"links"`
}

type ModesConfig struct {
	Classic bool `toml:"classic"`
	Secure  bool `toml:"secure"`
	TLS     bool `toml:"tls"`
}

type LinksConfig struct {
	Show []string `toml:"show"`
}

type ServerConfig struct {
	Port                      int      `toml:"port"`
	ListenAddrIPv4            string   `toml:"listen_addr_ipv4"`
	ListenAddrIPv6            string   `toml:"listen_addr_ipv6"`
	ProxyProtocol             bool     `toml:"proxy_protocol"`
	ProxyProtocolTrustedCIDRs []string `toml:"proxy_protocol_trusted_cidrs,omitempty"`
	MetricsListen             string   `toml:"metrics_listen"`
	MetricsWhitelist          []string `toml:"metrics_whitelist"`
}

type TimeoutsConfig struct {
	ClientHandshake int `toml:"client_handshake"`
	TGConnect       int `toml:"tg_connect"`
	ClientKeepalive int `toml:"client_keepalive"`
	ClientAck       int `toml:"client_ack"`
}

type CensorshipConfig struct {
	TLSDomain         string `toml:"tls_domain"`
	UnknownSNIAction  string `toml:"unknown_sni_action"`
	Mask              bool   `toml:"mask"`
	MaskPort          int    `toml:"mask_port"`
	MaskHost          string `toml:"mask_host,omitempty"`
	MaskRelayMaxBytes int64  `toml:"mask_relay_max_bytes,omitempty"`
	FakeCertLen       int    `toml:"fake_cert_len"`
}

type TelegramConfig struct {
	ProxySecretURL   string `toml:"proxy_secret_url,omitempty"`
	ProxyConfigV4URL string `toml:"proxy_config_v4_url,omitempty"`
	ProxyConfigV6URL string `toml:"proxy_config_v6_url,omitempty"`
}

type AccessConfig struct {
	ReplayCheckLen   int               `toml:"replay_check_len"`
	ReplayWindowSecs int               `toml:"replay_window_secs"`
	IgnoreTimeSkew   bool              `toml:"ignore_time_skew"`
	Users            map[string]string `toml:"users"`
	UserMaxTCPConns  map[string]int    `toml:"user_max_tcp_conns,omitempty"`
	UserMaxUniqueIPs map[string]int    `toml:"user_max_unique_ips,omitempty"`
	UserDataQuota    map[string]int64  `toml:"user_data_quota,omitempty"`
	UserExpirations  map[string]string `toml:"user_expirations,omitempty"`
}

type UpstreamConfig struct {
	Type      string `toml:"type"`
	Weight    int    `toml:"weight"`
	Address   string `toml:"address,omitempty"`
	Username  string `toml:"username,omitempty"`
	Password  string `toml:"password,omitempty"`
	UserID    string `toml:"user_id,omitempty"`
	Interface string `toml:"interface,omitempty"`
}

// ConfigParams holds the parameters needed to generate a telemt config.
type ConfigParams struct {
	Settings              *model.Settings
	Secrets               []SecretEntry
	Upstreams             []UpstreamEntry
	ExtraMetricsWhitelist []string // Additional IPs to allow in metrics_whitelist (e.g. Docker bridge IPs)
}

// SecretEntry represents a secret for TOML generation.
type SecretEntry struct {
	Label      string
	SecretKey  string
	Enabled    bool
	MaxConns   int
	MaxIPs     int
	QuotaBytes int64
	ExpiresAt  string
}

// UpstreamEntry represents an upstream for TOML generation.
type UpstreamEntry struct {
	Type     model.UpstreamType
	Address  string
	Username string
	Password string
	Weight   int
	Iface    string
	Enabled  bool
}

// BuildConfig constructs the telemt TOML configuration.
func BuildConfig(params *ConfigParams) *TelemtConfig {
	s := params.Settings

	metricsWhitelist := []string{"127.0.0.1", "::1"}
	metricsWhitelist = append(metricsWhitelist, params.ExtraMetricsWhitelist...)

	cfg := &TelemtConfig{
		General: GeneralConfig{
			PreferIPv6:     false,
			FastMode:       true,
			UseMiddleProxy: true,
			LogLevel:       "normal",
			Modes: ModesConfig{
				Classic: false,
				Secure:  !s.MaskingEnabled,
				TLS:     true,
			},
			Links: LinksConfig{
				Show: getEnabledLabels(params.Secrets),
			},
		},
		Server: ServerConfig{
			Port:             s.ProxyPort,
			ListenAddrIPv4:   "0.0.0.0",
			ListenAddrIPv6:   "::",
			ProxyProtocol:    s.ProxyProtocol,
			MetricsListen:    fmt.Sprintf("127.0.0.1:%d", s.ProxyMetricsPort),
			MetricsWhitelist: metricsWhitelist,
		},
		Timeouts: TimeoutsConfig{
			ClientHandshake: 30,
			TGConnect:       10,
			ClientKeepalive: 15,
			ClientAck:       90,
		},
		Censorship: CensorshipConfig{
			TLSDomain:         s.ProxyDomain,
			UnknownSNIAction:  s.UnknownSNIAction,
			Mask:              s.MaskingEnabled,
			MaskPort:          s.MaskingPort,
			FakeCertLen:       s.FakeCertLen,
			MaskRelayMaxBytes: s.MaskingRelayMaxBytes,
		},
		Access: AccessConfig{
			ReplayCheckLen:   65536,
			ReplayWindowSecs: 1800,
			IgnoreTimeSkew:   false,
			Users:            make(map[string]string),
		},
	}

	// Ad tag
	if s.AdTag != "" {
		cfg.General.AdTag = s.AdTag
	}

	// Masking host
	if s.MaskingEnabled && s.MaskingHost != "" {
		cfg.Censorship.MaskHost = s.MaskingHost
	}

	// Telegram custom URLs (for restricted regions)
	if s.ProxySecretURL != "" || s.ProxyConfigV4URL != "" || s.ProxyConfigV6URL != "" {
		cfg.Telegram = TelegramConfig{
			ProxySecretURL:   s.ProxySecretURL,
			ProxyConfigV4URL: s.ProxyConfigV4URL,
			ProxyConfigV6URL: s.ProxyConfigV6URL,
		}
	}

	// Proxy protocol trusted CIDRs
	if s.ProxyProtocol && s.ProxyProtocolTrustedCIDRs != "" {
		cfg.Server.ProxyProtocolTrustedCIDRs = parseCIDRList(s.ProxyProtocolTrustedCIDRs)
	}

	// Secrets
	cfg.Access.Users = make(map[string]string)
	cfg.Access.UserMaxTCPConns = make(map[string]int)
	cfg.Access.UserMaxUniqueIPs = make(map[string]int)
	cfg.Access.UserDataQuota = make(map[string]int64)
	cfg.Access.UserExpirations = make(map[string]string)

	for _, sec := range params.Secrets {
		if !sec.Enabled {
			continue
		}
		cfg.Access.Users[sec.Label] = sec.SecretKey

		if sec.MaxConns > 0 {
			cfg.Access.UserMaxTCPConns[sec.Label] = sec.MaxConns
		}
		if sec.MaxIPs > 0 {
			cfg.Access.UserMaxUniqueIPs[sec.Label] = sec.MaxIPs
		}
		if sec.QuotaBytes > 0 {
			cfg.Access.UserDataQuota[sec.Label] = sec.QuotaBytes
		}
		if sec.ExpiresAt != "" && sec.ExpiresAt != "0" {
			cfg.Access.UserExpirations[sec.Label] = sec.ExpiresAt
		}
	}

	// Upstreams
	for _, up := range params.Upstreams {
		if !up.Enabled {
			continue
		}
		uc := UpstreamConfig{
			Type:   string(up.Type),
			Weight: up.Weight,
		}
		if up.Type != model.UpstreamDirect {
			uc.Address = up.Address
			if up.Type == model.UpstreamSOCKS5 {
				uc.Username = up.Username
				uc.Password = up.Password
			} else if up.Type == model.UpstreamSOCKS4 {
				uc.UserID = up.Username
			}
		}
		if up.Iface != "" {
			uc.Interface = up.Iface
		}
		cfg.Upstreams = append(cfg.Upstreams, uc)
	}

	return cfg
}

// WriteConfigTOML writes the TOML configuration to a file atomically.
func WriteConfigTOML(cfg *TelemtConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	content := renderTOML(cfg)

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}

// renderTOML produces a TOML string matching the format the bash script generated.
func renderTOML(cfg *TelemtConfig) string {
	var b strings.Builder

	b.WriteString("# PopuGate — telemt configuration\n")
	b.WriteString(fmt.Sprintf("# Generated: %s\n\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))

	// [general]
	b.WriteString("[general]\n")
	b.WriteString(fmt.Sprintf("prefer_ipv6 = %v\n", cfg.General.PreferIPv6))
	b.WriteString(fmt.Sprintf("fast_mode = %v\n", cfg.General.FastMode))
	b.WriteString(fmt.Sprintf("use_middle_proxy = %v\n", cfg.General.UseMiddleProxy))
	b.WriteString(fmt.Sprintf("log_level = %q\n", cfg.General.LogLevel))
	if cfg.General.AdTag != "" {
		b.WriteString(fmt.Sprintf("ad_tag = %q\n", cfg.General.AdTag))
	} else {
		b.WriteString("# ad_tag = \"\"  # Get from @MTProxyBot\n")
	}
	b.WriteString("\n")

	// [general.modes]
	b.WriteString("[general.modes]\n")
	b.WriteString(fmt.Sprintf("classic = %v\n", cfg.General.Modes.Classic))
	b.WriteString(fmt.Sprintf("secure = %v\n", cfg.General.Modes.Secure))
	b.WriteString(fmt.Sprintf("tls = %v\n", cfg.General.Modes.TLS))
	b.WriteString("\n")

	// [general.links]
	b.WriteString("[general.links]\n")
	b.WriteString(fmt.Sprintf("show = %s\n", formatStringArray(cfg.General.Links.Show)))
	b.WriteString("# public_host = \"\"\n")
	b.WriteString(fmt.Sprintf("# public_port = %d\n", cfg.Server.Port))
	b.WriteString("\n")

	// [server]
	b.WriteString("[server]\n")
	b.WriteString(fmt.Sprintf("port = %d\n", cfg.Server.Port))
	b.WriteString(fmt.Sprintf("listen_addr_ipv4 = %q\n", cfg.Server.ListenAddrIPv4))
	b.WriteString(fmt.Sprintf("listen_addr_ipv6 = %q\n", cfg.Server.ListenAddrIPv6))
	b.WriteString(fmt.Sprintf("proxy_protocol = %v\n", cfg.Server.ProxyProtocol))
	if cfg.Server.ProxyProtocol && len(cfg.Server.ProxyProtocolTrustedCIDRs) > 0 {
		b.WriteString(fmt.Sprintf("proxy_protocol_trusted_cidrs = %s\n", formatStringArray(cfg.Server.ProxyProtocolTrustedCIDRs)))
	}
	b.WriteString(fmt.Sprintf("metrics_listen = %q\n", cfg.Server.MetricsListen))
	b.WriteString(fmt.Sprintf("metrics_whitelist = %s\n", formatStringArray(cfg.Server.MetricsWhitelist)))
	b.WriteString("\n")

	// [timeouts]
	b.WriteString("[timeouts]\n")
	b.WriteString(fmt.Sprintf("client_handshake = %d\n", cfg.Timeouts.ClientHandshake))
	b.WriteString(fmt.Sprintf("tg_connect = %d\n", cfg.Timeouts.TGConnect))
	b.WriteString(fmt.Sprintf("client_keepalive = %d\n", cfg.Timeouts.ClientKeepalive))
	b.WriteString(fmt.Sprintf("client_ack = %d\n", cfg.Timeouts.ClientAck))
	b.WriteString("\n")

	// [censorship]
	b.WriteString("[censorship]\n")
	b.WriteString(fmt.Sprintf("tls_domain = %q\n", cfg.Censorship.TLSDomain))
	b.WriteString(fmt.Sprintf("unknown_sni_action = %q\n", cfg.Censorship.UnknownSNIAction))
	b.WriteString(fmt.Sprintf("mask = %v\n", cfg.Censorship.Mask))
	b.WriteString(fmt.Sprintf("mask_port = %d\n", cfg.Censorship.MaskPort))
	if cfg.Censorship.Mask && cfg.Censorship.MaskHost != "" {
		b.WriteString(fmt.Sprintf("mask_host = %q\n", cfg.Censorship.MaskHost))
	}
	b.WriteString(fmt.Sprintf("fake_cert_len = %d\n", cfg.Censorship.FakeCertLen))
	if cfg.Censorship.MaskRelayMaxBytes > 0 {
		b.WriteString(fmt.Sprintf("mask_relay_max_bytes = %d\n", cfg.Censorship.MaskRelayMaxBytes))
	}
	b.WriteString("# Note: geo-blocking is enforced at the host firewall level (iptables/nftables),\n")
	b.WriteString("# not via telemt config.\n\n")

	// [access]
	b.WriteString("[access]\n")
	b.WriteString(fmt.Sprintf("replay_check_len = %d\n", cfg.Access.ReplayCheckLen))
	b.WriteString(fmt.Sprintf("replay_window_secs = %d\n", cfg.Access.ReplayWindowSecs))
	b.WriteString(fmt.Sprintf("ignore_time_skew = %v\n\n", cfg.Access.IgnoreTimeSkew))

	// [access.users]
	b.WriteString("[access.users]\n")
	for _, label := range sortedKeys(cfg.Access.Users) {
		b.WriteString(fmt.Sprintf("%s = %q\n", label, cfg.Access.Users[label]))
	}
	b.WriteString("\n")

	// Per-user limits
	if len(cfg.Access.UserMaxTCPConns) > 0 {
		b.WriteString("[access.user_max_tcp_conns]\n")
		for _, label := range sortedKeysInt(cfg.Access.UserMaxTCPConns) {
			b.WriteString(fmt.Sprintf("%s = %d\n", label, cfg.Access.UserMaxTCPConns[label]))
		}
		b.WriteString("\n")
	}

	if len(cfg.Access.UserMaxUniqueIPs) > 0 {
		b.WriteString("[access.user_max_unique_ips]\n")
		for _, label := range sortedKeysInt(cfg.Access.UserMaxUniqueIPs) {
			b.WriteString(fmt.Sprintf("%s = %d\n", label, cfg.Access.UserMaxUniqueIPs[label]))
		}
		b.WriteString("\n")
	}

	if len(cfg.Access.UserDataQuota) > 0 {
		b.WriteString("[access.user_data_quota]\n")
		for _, label := range sortedKeysInt64(cfg.Access.UserDataQuota) {
			b.WriteString(fmt.Sprintf("%s = %d\n", label, cfg.Access.UserDataQuota[label]))
		}
		b.WriteString("\n")
	}

	if len(cfg.Access.UserExpirations) > 0 {
		b.WriteString("[access.user_expirations]\n")
		for _, label := range sortedKeysStr(cfg.Access.UserExpirations) {
			b.WriteString(fmt.Sprintf("%s = %q\n", label, cfg.Access.UserExpirations[label]))
		}
		b.WriteString("\n")
	}

	// Upstreams
	for _, up := range cfg.Upstreams {
		b.WriteString("[[upstreams]]\n")
		b.WriteString(fmt.Sprintf("type = %q\n", up.Type))
		b.WriteString(fmt.Sprintf("weight = %d\n", up.Weight))
		if up.Address != "" {
			b.WriteString(fmt.Sprintf("address = %q\n", up.Address))
		}
		if up.Username != "" {
			b.WriteString(fmt.Sprintf("username = %q\n", up.Username))
		}
		if up.Password != "" {
			b.WriteString(fmt.Sprintf("password = %q\n", up.Password))
		}
		if up.UserID != "" {
			b.WriteString(fmt.Sprintf("user_id = %q\n", up.UserID))
		}
		if up.Interface != "" {
			b.WriteString(fmt.Sprintf("interface = %q\n", up.Interface))
		}
		b.WriteString("\n")
	}

	// [telegram] — custom URLs for restricted regions
	if cfg.Telegram.ProxySecretURL != "" || cfg.Telegram.ProxyConfigV4URL != "" || cfg.Telegram.ProxyConfigV6URL != "" {
		b.WriteString("[telegram]\n")
		if cfg.Telegram.ProxySecretURL != "" {
			b.WriteString(fmt.Sprintf("proxy_secret_url = %q\n", cfg.Telegram.ProxySecretURL))
		}
		if cfg.Telegram.ProxyConfigV4URL != "" {
			b.WriteString(fmt.Sprintf("proxy_config_v4_url = %q\n", cfg.Telegram.ProxyConfigV4URL))
		}
		if cfg.Telegram.ProxyConfigV6URL != "" {
			b.WriteString(fmt.Sprintf("proxy_config_v6_url = %q\n", cfg.Telegram.ProxyConfigV6URL))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	parts := make([]string, len(arr))
	for i, s := range arr {
		parts[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func getEnabledLabels(secrets []SecretEntry) []string {
	var labels []string
	for _, s := range secrets {
		if s.Enabled {
			labels = append(labels, s.Label)
		}
	}
	return labels
}

func parseCIDRList(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysInt(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysInt64(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysStr(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
