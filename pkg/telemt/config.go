package telemt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/logger"
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
	ClientMSS                 int      `toml:"client_mss,omitempty"`
	ClientMSSBulk             int      `toml:"client_mss_bulk,omitempty"`
	// SYN limiter (netfilter). Empty backend = disabled (not rendered).
	Synlimit         string `toml:"synlimit,omitempty"` // "nftables" | "iptables"
	SynlimitSeconds  int    `toml:"synlimit_seconds,omitempty"`
	SynlimitHitcount int    `toml:"synlimit_hitcount,omitempty"`
	SynlimitBurst    int    `toml:"synlimit_burst,omitempty"`
	// Control-plane API ([server.api]). APIPort 0 = disabled (rendered enabled=false
	// to override the engine's on-by-default 0.0.0.0:9091 listener). Bound to loopback.
	APIPort int `toml:"-"`
}

type TimeoutsConfig struct {
	ClientHandshake int `toml:"client_handshake"`
	TGConnect       int `toml:"tg_connect"`
	ClientKeepalive int `toml:"client_keepalive"`
	ClientAck       int `toml:"client_ack"`
}

type CensorshipConfig struct {
	TLSDomain         string            `toml:"tls_domain"`
	TLSDomains        []string          `toml:"tls_domains,omitempty"`
	UnknownSNIAction  string            `toml:"unknown_sni_action"`
	Mask              bool              `toml:"mask"`
	MaskPort          int               `toml:"mask_port"`
	MaskHost          string            `toml:"mask_host,omitempty"`
	MaskRelayMaxBytes int64             `toml:"mask_relay_max_bytes,omitempty"`
	FakeCertLen       int               `toml:"fake_cert_len"`
	TLSEmulation      bool              `toml:"tls_emulation"`
	TLSFrontDir       string            `toml:"tls_front_dir,omitempty"`
	ExclusiveMask     map[string]string `toml:"exclusive_mask,omitempty"`
}

type TelegramConfig struct {
	ProxySecretURL   string `toml:"proxy_secret_url,omitempty"`
	ProxyConfigV4URL string `toml:"proxy_config_v4_url,omitempty"`
	ProxyConfigV6URL string `toml:"proxy_config_v6_url,omitempty"`
}

type AccessConfig struct {
	ReplayCheckLen   int                  `toml:"replay_check_len"`
	ReplayWindowSecs int                  `toml:"replay_window_secs"`
	IgnoreTimeSkew   bool                 `toml:"ignore_time_skew"`
	Users            map[string]string    `toml:"users"`
	UserMaxTCPConns  map[string]int       `toml:"user_max_tcp_conns,omitempty"`
	UserMaxUniqueIPs map[string]int       `toml:"user_max_unique_ips,omitempty"`
	UserDataQuota    map[string]int64     `toml:"user_data_quota,omitempty"`
	UserExpirations  map[string]string    `toml:"user_expirations,omitempty"`
	UserRateLimits   map[string]RateLimit `toml:"user_rate_limits,omitempty"`
	UserEnabled      map[string]bool      `toml:"user_enabled,omitempty"`
}

// RateLimit holds per-user up/down rate limits in bits per second (0 = unlimited).
type RateLimit struct {
	UpBps   int64
	DownBps int64
}

type UpstreamConfig struct {
	Type         string `toml:"type"`
	Weight       int    `toml:"weight"`
	Address      string `toml:"address,omitempty"`
	Username     string `toml:"username,omitempty"`
	Password     string `toml:"password,omitempty"`
	UserID       string `toml:"user_id,omitempty"`
	URL          string `toml:"url,omitempty"`
	Interface    string `toml:"interface,omitempty"`
	IPv4         *bool  `toml:"ipv4,omitempty"`
	IPv6         *bool  `toml:"ipv6,omitempty"`
	Prefer       int    `toml:"prefer,omitempty"`
	BindToDevice string `toml:"bindtodevice,omitempty"`
}

// ConfigParams holds the parameters needed to generate a telemt config.
type ConfigParams struct {
	Settings              *model.Settings
	Secrets               []SecretEntry
	Upstreams             []UpstreamEntry
	ExtraMetricsWhitelist []string
}

// InstanceConfigParams holds per-instance parameters for config generation.
type InstanceConfigParams struct {
	Instance              *model.Instance
	DockerImage           string
	FakeCertLen           int
	UseMiddleProxy        bool
	AdTag                 string
	ProxyProtocol         bool
	ProxyProtocolCIDRs    []string
	Telegram              TelegramConfig
	Secrets               []SecretEntry
	Upstreams             []UpstreamEntry
	ExtraMetricsWhitelist []string
	// SYN limiter (opt-in). Empty Backend = disabled.
	SynlimitBackend  string
	SynlimitSeconds  int
	SynlimitHitcount int
	SynlimitBurst    int
}

// SecretEntry represents a secret for TOML generation.
type SecretEntry struct {
	Label            string
	SecretKey        string
	Enabled          bool
	MaxConns         int
	MaxIPs           int
	QuotaBytes       int64
	RateLimitUpBps   int64
	RateLimitDownBps int64
	ExpiresAt        string
}

// UpstreamEntry represents an upstream for TOML generation.
type UpstreamEntry struct {
	Type         model.UpstreamType
	Address      string
	Username     string
	Password     string
	URL          string // shadowsocks ss:// URL (shadowsocks type only)
	Weight       int
	Iface        string
	Enabled      bool
	IPv4         *bool
	IPv6         *bool
	Prefer       int
	BindToDevice string
}

func newEmptyAccess() AccessConfig {
	return AccessConfig{
		ReplayCheckLen:   65536,
		ReplayWindowSecs: 1800,
		IgnoreTimeSkew:   false,
		Users:            make(map[string]string),
		UserMaxTCPConns:  make(map[string]int),
		UserMaxUniqueIPs: make(map[string]int),
		UserDataQuota:    make(map[string]int64),
		UserExpirations:  make(map[string]string),
		UserRateLimits:   make(map[string]RateLimit),
		UserEnabled:      make(map[string]bool),
	}
}

func applySecretsToAccess(access *AccessConfig, secrets []SecretEntry) {
	for _, sec := range secrets {
		access.Users[sec.Label] = sec.SecretKey
		if !sec.Enabled {
			// Keep the user in [access.users] but mark it disabled so the engine
			// cancels active sessions on hot-reload. Skip limits — irrelevant for
			// a blocked user.
			access.UserEnabled[sec.Label] = false
			continue
		}
		if sec.MaxConns > 0 {
			access.UserMaxTCPConns[sec.Label] = sec.MaxConns
		}
		if sec.MaxIPs > 0 {
			access.UserMaxUniqueIPs[sec.Label] = sec.MaxIPs
		}
		if sec.QuotaBytes > 0 {
			access.UserDataQuota[sec.Label] = sec.QuotaBytes
		}
		if sec.RateLimitUpBps > 0 || sec.RateLimitDownBps > 0 {
			access.UserRateLimits[sec.Label] = RateLimit{UpBps: sec.RateLimitUpBps, DownBps: sec.RateLimitDownBps}
		}
		if sec.ExpiresAt != "" && sec.ExpiresAt != "0" {
			access.UserExpirations[sec.Label] = sec.ExpiresAt
		}
	}
}

func buildUpstreamConfig(up UpstreamEntry) UpstreamConfig {
	uc := UpstreamConfig{
		Type:   string(up.Type),
		Weight: up.Weight,
	}
	switch up.Type {
	case model.UpstreamDirect:
		// no address/credentials
	case model.UpstreamShadowsocks:
		uc.URL = up.URL
	case model.UpstreamSOCKS5:
		uc.Address = up.Address
		uc.Username = up.Username
		uc.Password = up.Password
	case model.UpstreamSOCKS4:
		uc.Address = up.Address
		uc.UserID = up.Username
	}
	if up.Iface != "" {
		uc.Interface = up.Iface
	}
	uc.IPv4 = up.IPv4
	uc.IPv6 = up.IPv6
	uc.Prefer = up.Prefer
	if up.Type == model.UpstreamDirect {
		uc.BindToDevice = up.BindToDevice
	}
	return uc
}

func buildUpstreams(entries []UpstreamEntry) []UpstreamConfig {
	var upstreams []UpstreamConfig
	for _, up := range entries {
		if !up.Enabled {
			continue
		}
		upstreams = append(upstreams, buildUpstreamConfig(up))
	}
	return upstreams
}

func buildMetricsWhitelist(extra []string) []string {
	wl := []string{"127.0.0.1", "::1"}
	return append(wl, extra...)
}

func defaultTimeouts() TimeoutsConfig {
	return TimeoutsConfig{
		ClientHandshake: 30,
		TGConnect:       10,
		ClientKeepalive: 15,
		ClientAck:       90,
	}
}

func maskingHost(enabled bool, host string) string {
	if enabled {
		return host
	}
	return ""
}

func telegramHasURLs(tc TelegramConfig) bool {
	return tc.ProxySecretURL != "" || tc.ProxyConfigV4URL != "" || tc.ProxyConfigV6URL != ""
}

func telegramConfigFromURLs(secretURL, v4URL, v6URL string) TelegramConfig {
	if secretURL == "" && v4URL == "" && v6URL == "" {
		return TelegramConfig{}
	}
	return TelegramConfig{
		ProxySecretURL:   secretURL,
		ProxyConfigV4URL: v4URL,
		ProxyConfigV6URL: v6URL,
	}
}

// BuildConfig constructs the telemt TOML configuration (legacy: from global settings).
func BuildConfig(params *ConfigParams) *TelemtConfig {
	s := params.Settings
	metricsWhitelist := buildMetricsWhitelist(params.ExtraMetricsWhitelist)

	cfg := &TelemtConfig{
		General: GeneralConfig{
			PreferIPv6:     false,
			FastMode:       true,
			UseMiddleProxy: s.UseMiddleProxy,
			LogLevel:       "normal",
			AdTag:          s.AdTag,
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
			MetricsListen:    metricsListenAddr(s.ProxyMetricsPort, params.ExtraMetricsWhitelist),
			MetricsWhitelist: metricsWhitelist,
		},
		Timeouts: defaultTimeouts(),
		Censorship: CensorshipConfig{
			TLSDomain:         s.ProxyDomain,
			UnknownSNIAction:  s.UnknownSNIAction,
			Mask:              s.MaskingEnabled,
			MaskPort:          s.MaskingPort,
			MaskHost:          maskingHost(s.MaskingEnabled, s.MaskingHost),
			FakeCertLen:       s.FakeCertLen,
			MaskRelayMaxBytes: s.MaskingRelayMaxBytes,
		},
		Access: newEmptyAccess(),
	}

	cfg.Telegram = telegramConfigFromURLs(s.ProxySecretURL, s.ProxyConfigV4URL, s.ProxyConfigV6URL)

	if s.ProxyProtocol && s.ProxyProtocolTrustedCIDRs != "" {
		cfg.Server.ProxyProtocolTrustedCIDRs = parseCIDRList(s.ProxyProtocolTrustedCIDRs)
	}

	applySecretsToAccess(&cfg.Access, params.Secrets)
	cfg.Upstreams = buildUpstreams(params.Upstreams)

	return cfg
}

func instanceModes(fakeTLS bool) ModesConfig {
	modes := ModesConfig{TLS: fakeTLS, Secure: false}
	if !fakeTLS {
		modes.Classic = true
	}
	return modes
}

func instanceCensorship(inst *model.Instance, fakeCertLen int) CensorshipConfig {
	if !inst.FakeTLS {
		return CensorshipConfig{}
	}
	sniAction := inst.UnknownSNIAction
	if sniAction == "" {
		sniAction = "mask"
	}
	cfg := CensorshipConfig{
		TLSDomain:        inst.TLSDomain,
		TLSDomains:       inst.GetTLSDomains(),
		UnknownSNIAction: sniAction,
		ExclusiveMask:    inst.GetExclusiveMask(),
		Mask:             true,
		MaskHost:         inst.GetMaskHost(),
		MaskPort:         inst.MaskPort,
		FakeCertLen:      fakeCertLen,
		TLSEmulation:     true,
	}
	if inst.TLSFronting {
		cfg.TLSFrontDir = "/tlsfront"
	}
	return cfg
}

// BuildInstanceConfig constructs a telemt TOML config for a specific instance.
func BuildInstanceConfig(params *InstanceConfigParams) *TelemtConfig {
	inst := params.Instance
	metricsWhitelist := buildMetricsWhitelist(params.ExtraMetricsWhitelist)

	cfg := &TelemtConfig{
		General: GeneralConfig{
			PreferIPv6:     false,
			FastMode:       true,
			UseMiddleProxy: params.UseMiddleProxy,
			LogLevel:       "normal",
			AdTag:          params.AdTag,
			Modes:          instanceModes(inst.FakeTLS),
			Links: LinksConfig{
				Show: getEnabledLabels(params.Secrets),
			},
		},
		Server: ServerConfig{
			Port:             inst.Port,
			ListenAddrIPv4:   "0.0.0.0",
			ListenAddrIPv6:   "::",
			ProxyProtocol:    params.ProxyProtocol,
			MetricsListen:    metricsListenAddr(inst.MetricsPort, params.ExtraMetricsWhitelist),
			MetricsWhitelist: metricsWhitelist,
		},
		Timeouts:   defaultTimeouts(),
		Censorship: instanceCensorship(inst, params.FakeCertLen),
		Access:     newEmptyAccess(),
	}

	if inst.TCPMSSEnabled && inst.TCPMSS > 0 {
		cfg.Server.ClientMSS = inst.TCPMSS
		cfg.Server.ClientMSSBulk = inst.TCPMSSBulk
	}

	if params.SynlimitBackend != "" {
		cfg.Server.Synlimit = params.SynlimitBackend
		cfg.Server.SynlimitSeconds = params.SynlimitSeconds
		cfg.Server.SynlimitHitcount = params.SynlimitHitcount
		cfg.Server.SynlimitBurst = params.SynlimitBurst
	}

	cfg.Server.APIPort = inst.APIPort

	if params.ProxyProtocol && len(params.ProxyProtocolCIDRs) > 0 {
		cfg.Server.ProxyProtocolTrustedCIDRs = params.ProxyProtocolCIDRs
	}

	if telegramHasURLs(params.Telegram) {
		cfg.Telegram = params.Telegram
	}

	applySecretsToAccess(&cfg.Access, params.Secrets)
	cfg.Upstreams = buildUpstreams(params.Upstreams)

	return cfg
}

// metricsListenAddr returns "0.0.0.0:port" when running inside Docker
// (so the host can reach metrics via host.docker.internal), otherwise
// "127.0.0.1:port".
func metricsListenAddr(port int, extraWhitelist []string) string {
	if len(extraWhitelist) > 0 {
		return fmt.Sprintf("0.0.0.0:%d", port)
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
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
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}

func renderTOML(cfg *TelemtConfig) string {
	var b strings.Builder

	b.WriteString("# PopuGate — telemt configuration\n")
	fmt.Fprintf(&b, "# Generated: %s\n\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))

	renderGeneralSection(&b, cfg)
	renderModesSection(&b, cfg)
	renderLinksSection(&b, cfg)
	renderServerSection(&b, cfg)
	renderTimeoutsSection(&b, cfg)
	renderCensorshipSection(&b, cfg)
	renderAccessSection(&b, cfg)
	renderUpstreamsSection(&b, cfg)

	return b.String()
}

func renderGeneralSection(b *strings.Builder, cfg *TelemtConfig) {
	b.WriteString("[general]\n")
	fmt.Fprintf(b, "prefer_ipv6 = %v\n", cfg.General.PreferIPv6)
	fmt.Fprintf(b, "fast_mode = %v\n", cfg.General.FastMode)
	fmt.Fprintf(b, "use_middle_proxy = %v\n", cfg.General.UseMiddleProxy)
	fmt.Fprintf(b, "log_level = %q\n", cfg.General.LogLevel)
	if cfg.General.AdTag != "" {
		fmt.Fprintf(b, "ad_tag = %q\n", cfg.General.AdTag)
	} else {
		b.WriteString("# ad_tag = \"\"  # Get from @MTProxyBot\n")
	}
	// tg_connect is a [general] key in the telemt engine (not [timeouts]).
	fmt.Fprintf(b, "tg_connect = %d\n", cfg.Timeouts.TGConnect)
	// Custom Telegram infrastructure URLs are [general] keys (engine >= 3.4.4).
	if cfg.Telegram.ProxySecretURL != "" {
		fmt.Fprintf(b, "proxy_secret_url = %q\n", cfg.Telegram.ProxySecretURL)
	}
	if cfg.Telegram.ProxyConfigV4URL != "" {
		fmt.Fprintf(b, "proxy_config_v4_url = %q\n", cfg.Telegram.ProxyConfigV4URL)
	}
	if cfg.Telegram.ProxyConfigV6URL != "" {
		fmt.Fprintf(b, "proxy_config_v6_url = %q\n", cfg.Telegram.ProxyConfigV6URL)
	}
	b.WriteString("\n")
}

func renderModesSection(b *strings.Builder, cfg *TelemtConfig) {
	b.WriteString("[general.modes]\n")
	fmt.Fprintf(b, "classic = %v\n", cfg.General.Modes.Classic)
	fmt.Fprintf(b, "secure = %v\n", cfg.General.Modes.Secure)
	fmt.Fprintf(b, "tls = %v\n", cfg.General.Modes.TLS)
	b.WriteString("\n")
}

func renderLinksSection(b *strings.Builder, cfg *TelemtConfig) {
	b.WriteString("[general.links]\n")
	fmt.Fprintf(b, "show = %s\n", formatStringArray(cfg.General.Links.Show))
	b.WriteString("# public_host = \"\"\n")
	fmt.Fprintf(b, "# public_port = %d\n", cfg.Server.Port)
	b.WriteString("\n")
}

func renderServerSection(b *strings.Builder, cfg *TelemtConfig) {
	b.WriteString("[server]\n")
	fmt.Fprintf(b, "port = %d\n", cfg.Server.Port)
	fmt.Fprintf(b, "listen_addr_ipv4 = %q\n", cfg.Server.ListenAddrIPv4)
	fmt.Fprintf(b, "listen_addr_ipv6 = %q\n", cfg.Server.ListenAddrIPv6)
	fmt.Fprintf(b, "proxy_protocol = %v\n", cfg.Server.ProxyProtocol)
	if cfg.Server.ProxyProtocol && len(cfg.Server.ProxyProtocolTrustedCIDRs) > 0 {
		fmt.Fprintf(b, "proxy_protocol_trusted_cidrs = %s\n", formatStringArray(cfg.Server.ProxyProtocolTrustedCIDRs))
	}
	fmt.Fprintf(b, "metrics_listen = %q\n", cfg.Server.MetricsListen)
	fmt.Fprintf(b, "metrics_whitelist = %s\n", formatStringArray(cfg.Server.MetricsWhitelist))
	// Client-facing TCP MSS shaping (anti-DPI). client_mss_bulk raises the MSS
	// after the handshake to cut pps. Restart-required (listener rebind).
	// The engine types these as strings (Option<String>, preset-or-int grammar),
	// so they MUST be quoted — a bare integer is rejected as a config type error.
	if cfg.Server.ClientMSS > 0 {
		fmt.Fprintf(b, "client_mss = \"%d\"\n", cfg.Server.ClientMSS)
		if cfg.Server.ClientMSSBulk > 0 {
			fmt.Fprintf(b, "client_mss_bulk = \"%d\"\n", cfg.Server.ClientMSSBulk)
		}
	}
	b.WriteString("\n")

	// Control-plane API: bind to loopback for hot per-user quota reset. When no
	// port is assigned, explicitly disable it (the engine enables it by default
	// on 0.0.0.0:9091, which we do not want exposed).
	b.WriteString("[server.api]\n")
	if cfg.Server.APIPort > 0 {
		b.WriteString("enabled = true\n")
		fmt.Fprintf(b, "listen = %q\n", fmt.Sprintf("127.0.0.1:%d", cfg.Server.APIPort))
	} else {
		b.WriteString("enabled = false\n")
	}
	b.WriteString("\n")

	renderServerListeners(b, cfg)
}

// renderServerListeners emits explicit [[server.listeners]] entries. The engine's
// netfilter SYN limiter is a PER-LISTENER setting (ListenerConfig), not a [server]
// key — placed on [server] it is silently ignored. So we only emit listeners when
// the limiter is enabled, and then we must fully replicate the engine's own
// legacy-to-listener migration (one listener per configured bind address on the
// primary port), because providing any explicit listener disables that migration.
// client_mss stays on [server]; a listener with client_mss unset inherits it.
func renderServerListeners(b *strings.Builder, cfg *TelemtConfig) {
	if cfg.Server.Synlimit == "" {
		return
	}
	addrs := make([]string, 0, 2)
	if cfg.Server.ListenAddrIPv4 != "" {
		addrs = append(addrs, cfg.Server.ListenAddrIPv4)
	}
	if cfg.Server.ListenAddrIPv6 != "" {
		addrs = append(addrs, cfg.Server.ListenAddrIPv6)
	}
	for _, ip := range addrs {
		b.WriteString("[[server.listeners]]\n")
		fmt.Fprintf(b, "ip = %q\n", ip)
		fmt.Fprintf(b, "port = %d\n", cfg.Server.Port)
		fmt.Fprintf(b, "synlimit = %q\n", cfg.Server.Synlimit)
		if cfg.Server.SynlimitSeconds > 0 {
			fmt.Fprintf(b, "synlimit_seconds = %d\n", cfg.Server.SynlimitSeconds)
		}
		if cfg.Server.SynlimitHitcount > 0 {
			fmt.Fprintf(b, "synlimit_hitcount = %d\n", cfg.Server.SynlimitHitcount)
		}
		if cfg.Server.SynlimitBurst > 0 {
			fmt.Fprintf(b, "synlimit_burst = %d\n", cfg.Server.SynlimitBurst)
		}
		b.WriteString("\n")
	}
}

func renderTimeoutsSection(b *strings.Builder, cfg *TelemtConfig) {
	b.WriteString("[timeouts]\n")
	fmt.Fprintf(b, "client_handshake = %d\n", cfg.Timeouts.ClientHandshake)
	fmt.Fprintf(b, "client_keepalive = %d\n", cfg.Timeouts.ClientKeepalive)
	fmt.Fprintf(b, "client_ack = %d\n", cfg.Timeouts.ClientAck)
	b.WriteString("\n")
}

func renderCensorshipSection(b *strings.Builder, cfg *TelemtConfig) {
	if cfg.Censorship.TLSDomain == "" {
		return
	}
	b.WriteString("[censorship]\n")
	fmt.Fprintf(b, "tls_domain = %q\n", cfg.Censorship.TLSDomain)
	if len(cfg.Censorship.TLSDomains) > 0 {
		fmt.Fprintf(b, "tls_domains = %s\n", formatStringArray(cfg.Censorship.TLSDomains))
	}
	fmt.Fprintf(b, "unknown_sni_action = %q\n", cfg.Censorship.UnknownSNIAction)
	fmt.Fprintf(b, "mask = %v\n", cfg.Censorship.Mask)
	fmt.Fprintf(b, "mask_port = %d\n", cfg.Censorship.MaskPort)
	if cfg.Censorship.Mask && cfg.Censorship.MaskHost != "" {
		fmt.Fprintf(b, "mask_host = %q\n", cfg.Censorship.MaskHost)
	}
	fmt.Fprintf(b, "fake_cert_len = %d\n", cfg.Censorship.FakeCertLen)
	if cfg.Censorship.MaskRelayMaxBytes > 0 {
		fmt.Fprintf(b, "mask_relay_max_bytes = %d\n", cfg.Censorship.MaskRelayMaxBytes)
	}
	if cfg.Censorship.TLSEmulation {
		b.WriteString("tls_emulation = true\n")
	}
	if cfg.Censorship.TLSFrontDir != "" {
		fmt.Fprintf(b, "tls_front_dir = %q\n", cfg.Censorship.TLSFrontDir)
	}
	b.WriteString("# Note: geo-blocking is enforced at the host firewall level (iptables/nftables),\n")
	b.WriteString("# not via telemt config.\n\n")

	// Per-SNI mask targets for unauthenticated fallback traffic. Must follow all
	// flat [censorship] keys (TOML sub-table rule).
	if len(cfg.Censorship.ExclusiveMask) > 0 {
		b.WriteString("[censorship.exclusive_mask]\n")
		for _, sni := range sortedKeys(cfg.Censorship.ExclusiveMask) {
			fmt.Fprintf(b, "%q = %q\n", sni, cfg.Censorship.ExclusiveMask[sni])
		}
		b.WriteString("\n")
	}
}

// tomlKeyRe validates that a string is safe to use as a bare TOML key.
var tomlKeyRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// tomlLog is a scoped logger for TOML config generation warnings.
var tomlLog = logger.WithScope("telemt-config")

func renderAccessSection(b *strings.Builder, cfg *TelemtConfig) {
	b.WriteString("[access]\n")
	fmt.Fprintf(b, "replay_check_len = %d\n", cfg.Access.ReplayCheckLen)
	fmt.Fprintf(b, "replay_window_secs = %d\n", cfg.Access.ReplayWindowSecs)
	fmt.Fprintf(b, "ignore_time_skew = %v\n\n", cfg.Access.IgnoreTimeSkew)

	b.WriteString("[access.users]\n")
	for _, label := range sortedKeys(cfg.Access.Users) {
		if !tomlKeyRe.MatchString(label) {
			tomlLog.Warnf("skipping secret %q: label not valid as TOML key (must match [a-zA-Z0-9_-]{1,32})", label)
			continue
		}
		fmt.Fprintf(b, "%s = %q\n", label, cfg.Access.Users[label])
	}
	b.WriteString("\n")

	if len(cfg.Access.UserMaxTCPConns) > 0 {
		b.WriteString("[access.user_max_tcp_conns]\n")
		for _, label := range sortedKeys(cfg.Access.UserMaxTCPConns) {
			if !tomlKeyRe.MatchString(label) {
				continue
			}
			fmt.Fprintf(b, "%s = %d\n", label, cfg.Access.UserMaxTCPConns[label])
		}
		b.WriteString("\n")
	}

	if len(cfg.Access.UserMaxUniqueIPs) > 0 {
		b.WriteString("[access.user_max_unique_ips]\n")
		for _, label := range sortedKeys(cfg.Access.UserMaxUniqueIPs) {
			if !tomlKeyRe.MatchString(label) {
				continue
			}
			fmt.Fprintf(b, "%s = %d\n", label, cfg.Access.UserMaxUniqueIPs[label])
		}
		b.WriteString("\n")
	}

	if len(cfg.Access.UserDataQuota) > 0 {
		b.WriteString("[access.user_data_quota]\n")
		for _, label := range sortedKeys(cfg.Access.UserDataQuota) {
			if !tomlKeyRe.MatchString(label) {
				continue
			}
			fmt.Fprintf(b, "%s = %d\n", label, cfg.Access.UserDataQuota[label])
		}
		b.WriteString("\n")
	}

	if len(cfg.Access.UserExpirations) > 0 {
		b.WriteString("[access.user_expirations]\n")
		for _, label := range sortedKeys(cfg.Access.UserExpirations) {
			if !tomlKeyRe.MatchString(label) {
				continue
			}
			fmt.Fprintf(b, "%s = %q\n", label, cfg.Access.UserExpirations[label])
		}
		b.WriteString("\n")
	}

	// Per-user rate limits render as nested tables [access.user_rate_limits.<label>]
	// with up_bps/down_bps (bits per second); hot-reloadable in the engine.
	for _, label := range sortedKeys(cfg.Access.UserRateLimits) {
		if !tomlKeyRe.MatchString(label) {
			continue
		}
		rl := cfg.Access.UserRateLimits[label]
		fmt.Fprintf(b, "[access.user_rate_limits.%s]\n", label)
		fmt.Fprintf(b, "up_bps = %d\n", rl.UpBps)
		fmt.Fprintf(b, "down_bps = %d\n", rl.DownBps)
		b.WriteString("\n")
	}

	// Disabled secrets stay in [access.users] but are marked disabled here so the
	// engine cancels their active sessions on hot-reload (missing = enabled).
	if len(cfg.Access.UserEnabled) > 0 {
		b.WriteString("[access.user_enabled]\n")
		for _, label := range sortedKeys(cfg.Access.UserEnabled) {
			if !tomlKeyRe.MatchString(label) {
				continue
			}
			fmt.Fprintf(b, "%s = %v\n", label, cfg.Access.UserEnabled[label])
		}
		b.WriteString("\n")
	}
}

func renderUpstreamsSection(b *strings.Builder, cfg *TelemtConfig) {
	for _, up := range cfg.Upstreams {
		b.WriteString("[[upstreams]]\n")
		fmt.Fprintf(b, "type = %q\n", up.Type)
		fmt.Fprintf(b, "weight = %d\n", up.Weight)
		if up.URL != "" {
			fmt.Fprintf(b, "url = %q\n", up.URL)
		}
		if up.Address != "" {
			fmt.Fprintf(b, "address = %q\n", up.Address)
		}
		if up.Username != "" {
			fmt.Fprintf(b, "username = %q\n", up.Username)
		}
		if up.Password != "" {
			fmt.Fprintf(b, "password = %q\n", up.Password)
		}
		if up.UserID != "" {
			fmt.Fprintf(b, "user_id = %q\n", up.UserID)
		}
		if up.Interface != "" {
			fmt.Fprintf(b, "interface = %q\n", up.Interface)
		}
		if up.IPv4 != nil {
			fmt.Fprintf(b, "ipv4 = %v\n", *up.IPv4)
		}
		if up.IPv6 != nil {
			fmt.Fprintf(b, "ipv6 = %v\n", *up.IPv6)
		}
		if up.Prefer == 4 || up.Prefer == 6 {
			fmt.Fprintf(b, "prefer = %d\n", up.Prefer)
		}
		if up.BindToDevice != "" {
			fmt.Fprintf(b, "bindtodevice = %q\n", up.BindToDevice)
		}
		b.WriteString("\n")
	}
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

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
