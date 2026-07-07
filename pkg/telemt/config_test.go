package telemt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/fussraider/PopuGate/internal/model"
)

// parseRenderedTOML renders cfg and unmarshals the result, failing the test if
// the generated config is not valid TOML. It returns the parsed top-level tables.
func parseRenderedTOML(t *testing.T, cfg *TelemtConfig) map[string]any {
	t.Helper()
	rendered := renderTOML(cfg)
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(rendered), &parsed); err != nil {
		t.Fatalf("renderTOML produced invalid TOML: %v\n---\n%s", err, rendered)
	}
	return parsed
}

// tomlTable returns the named sub-table, failing if it is missing or not a table.
func tomlTable(t *testing.T, parsed map[string]any, name string) map[string]any {
	t.Helper()
	tbl, ok := parsed[name].(map[string]any)
	if !ok {
		t.Fatalf("expected TOML table [%s], got %T", name, parsed[name])
	}
	return tbl
}

func TestBuildConfig_Basic(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
			ProxyDomain:      "example.com",
			MaskingEnabled:   false,
			UnknownSNIAction: "accept",
			ProxyProtocol:    false,
			FakeCertLen:      32,
		},
		Secrets: []SecretEntry{
			{Label: "user1", SecretKey: "aa11bb22cc33dd44ee55ff6677889900", Enabled: true},
			{Label: "user2", SecretKey: "11223344556677889900aabbccddeeff", Enabled: false},
		},
	}

	cfg := BuildConfig(params)
	if cfg == nil {
		t.Fatal("BuildConfig returned nil")
	}

	// Basic checks
	if cfg.Server.Port != 443 {
		t.Errorf("Server.Port = %d, want 443", cfg.Server.Port)
	}
	if cfg.Server.MetricsListen != "127.0.0.1:9091" {
		t.Errorf("Server.MetricsListen = %q, want %q", cfg.Server.MetricsListen, "127.0.0.1:9091")
	}

	// Disabled secrets stay in Users but are flagged in user_enabled=false so the
	// engine cancels their active sessions on reload (ADR-001).
	if len(cfg.Access.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(cfg.Access.Users))
	}
	if _, ok := cfg.Access.Users["user1"]; !ok {
		t.Error("user1 should be in Users")
	}
	if _, ok := cfg.Access.Users["user2"]; !ok {
		t.Error("user2 should be in Users (disabled but present)")
	}
	if v, ok := cfg.Access.UserEnabled["user2"]; !ok || v {
		t.Errorf("user2 should be in user_enabled=false, got ok=%v v=%v", ok, v)
	}
	if _, ok := cfg.Access.UserEnabled["user1"]; ok {
		t.Error("user1 (enabled) must not appear in user_enabled (missing = enabled)")
	}

	// Secure mode should be true when masking is disabled
	if !cfg.General.Modes.Secure {
		t.Error("Secure mode should be true when masking is disabled")
	}
}

func TestBuildConfig_WithMasking(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
			ProxyDomain:      "example.com",
			MaskingEnabled:   true,
			MaskingPort:      8443,
			MaskingHost:      "mask.example.com",
			UnknownSNIAction: "reject",
			FakeCertLen:      64,
		},
	}

	cfg := BuildConfig(params)

	if cfg.Censorship.Mask != true {
		t.Error("Mask should be true")
	}
	if cfg.Censorship.MaskPort != 8443 {
		t.Errorf("MaskPort = %d, want 8443", cfg.Censorship.MaskPort)
	}
	if cfg.Censorship.MaskHost != "mask.example.com" {
		t.Errorf("MaskHost = %q, want %q", cfg.Censorship.MaskHost, "mask.example.com")
	}
	if cfg.General.Modes.Secure {
		t.Error("Secure mode should be false when masking is enabled")
	}
}

func TestBuildConfig_WithProxyProtocol(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:                 443,
			ProxyMetricsPort:          9091,
			ProxyProtocol:             true,
			ProxyProtocolTrustedCIDRs: "10.0.0.0/8, 172.16.0.0/12",
		},
	}

	cfg := BuildConfig(params)

	if !cfg.Server.ProxyProtocol {
		t.Error("ProxyProtocol should be true")
	}
	if len(cfg.Server.ProxyProtocolTrustedCIDRs) != 2 {
		t.Fatalf("expected 2 CIDRs, got %d", len(cfg.Server.ProxyProtocolTrustedCIDRs))
	}
	if cfg.Server.ProxyProtocolTrustedCIDRs[0] != "10.0.0.0/8" {
		t.Errorf("first CIDR = %q, want %q", cfg.Server.ProxyProtocolTrustedCIDRs[0], "10.0.0.0/8")
	}
}

func TestBuildConfig_WithAdTag(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
			AdTag:            "test_ad_tag_123",
		},
	}

	cfg := BuildConfig(params)
	if cfg.General.AdTag != "test_ad_tag_123" {
		t.Errorf("AdTag = %q, want %q", cfg.General.AdTag, "test_ad_tag_123")
	}
}

func TestBuildConfig_SecretLimits(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
		},
		Secrets: []SecretEntry{
			{
				Label:      "limited",
				SecretKey:  "aa11bb22cc33dd44ee55ff6677889900",
				Enabled:    true,
				MaxConns:   10,
				MaxIPs:     5,
				QuotaBytes: 1073741824,
				ExpiresAt:  "2026-12-31",
			},
		},
	}

	cfg := BuildConfig(params)

	if cfg.Access.UserMaxTCPConns["limited"] != 10 {
		t.Errorf("MaxConns = %d, want 10", cfg.Access.UserMaxTCPConns["limited"])
	}
	if cfg.Access.UserMaxUniqueIPs["limited"] != 5 {
		t.Errorf("MaxIPs = %d, want 5", cfg.Access.UserMaxUniqueIPs["limited"])
	}
	if cfg.Access.UserDataQuota["limited"] != 1073741824 {
		t.Errorf("QuotaBytes = %d, want 1073741824", cfg.Access.UserDataQuota["limited"])
	}
	if cfg.Access.UserExpirations["limited"] != "2026-12-31" {
		t.Errorf("ExpiresAt = %q, want %q", cfg.Access.UserExpirations["limited"], "2026-12-31")
	}
}

func TestBuildConfig_ZeroLimits(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
		},
		Secrets: []SecretEntry{
			{
				Label:     "plain",
				SecretKey: "aa11bb22cc33dd44ee55ff6677889900",
				Enabled:   true,
				MaxConns:  0,
				MaxIPs:    0,
				ExpiresAt: "0",
			},
		},
	}

	cfg := BuildConfig(params)

	if _, ok := cfg.Access.UserMaxTCPConns["plain"]; ok {
		t.Error("MaxConns=0 should not be in UserMaxTCPConns")
	}
	if _, ok := cfg.Access.UserMaxUniqueIPs["plain"]; ok {
		t.Error("MaxIPs=0 should not be in UserMaxUniqueIPs")
	}
	if _, ok := cfg.Access.UserExpirations["plain"]; ok {
		t.Error("ExpiresAt=0 should not be in UserExpirations")
	}
}

func TestBuildConfig_Upstreams(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
		},
		Upstreams: []UpstreamEntry{
			{Type: model.UpstreamDirect, Weight: 10, Enabled: true},
			{Type: model.UpstreamSOCKS5, Address: "socks5://proxy:1080", Username: "user", Password: "pass", Weight: 20, Enabled: true},
			{Type: model.UpstreamSOCKS4, Address: "socks4://proxy:1080", Username: "userid", Weight: 5, Enabled: true},
			{Type: model.UpstreamDirect, Weight: 10, Enabled: false},
			{Type: model.UpstreamSOCKS5, Address: "extra:1080", Iface: "eth1", Weight: 15, Enabled: true},
		},
	}

	cfg := BuildConfig(params)

	// 4 enabled upstreams (direct disabled one excluded)
	if len(cfg.Upstreams) != 4 {
		t.Fatalf("expected 4 upstreams, got %d", len(cfg.Upstreams))
	}

	// Check direct has no address
	if cfg.Upstreams[0].Address != "" {
		t.Error("direct upstream should have empty address")
	}

	// Check SOCKS5
	if cfg.Upstreams[1].Username != "user" {
		t.Errorf("SOCKS5 username = %q, want %q", cfg.Upstreams[1].Username, "user")
	}
	if cfg.Upstreams[1].Password != "pass" {
		t.Errorf("SOCKS5 password = %q, want %q", cfg.Upstreams[1].Password, "pass")
	}

	// Check SOCKS4
	if cfg.Upstreams[2].UserID != "userid" {
		t.Errorf("SOCKS4 user_id = %q, want %q", cfg.Upstreams[2].UserID, "userid")
	}

	// Check interface
	if cfg.Upstreams[3].Interface != "eth1" {
		t.Errorf("interface = %q, want %q", cfg.Upstreams[3].Interface, "eth1")
	}
}

func TestBuildConfig_MetricsWhitelist(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
		},
		ExtraMetricsWhitelist: []string{"172.17.0.1", "10.0.0.1"},
	}

	cfg := BuildConfig(params)

	// Should have localhost + extra IPs
	expected := []string{"127.0.0.1", "::1", "172.17.0.1", "10.0.0.1"}
	if len(cfg.Server.MetricsWhitelist) != len(expected) {
		t.Fatalf("expected %d whitelist entries, got %d", len(expected), len(cfg.Server.MetricsWhitelist))
	}
	for i, ip := range expected {
		if cfg.Server.MetricsWhitelist[i] != ip {
			t.Errorf("whitelist[%d] = %q, want %q", i, cfg.Server.MetricsWhitelist[i], ip)
		}
	}
}

func TestBuildConfig_LinksShow(t *testing.T) {
	params := &ConfigParams{
		Settings: &model.Settings{
			ProxyPort:        443,
			ProxyMetricsPort: 9091,
		},
		Secrets: []SecretEntry{
			{Label: "a", Enabled: true},
			{Label: "b", Enabled: false},
			{Label: "c", Enabled: true},
		},
	}

	cfg := BuildConfig(params)

	if len(cfg.General.Links.Show) != 2 {
		t.Fatalf("expected 2 enabled labels, got %d", len(cfg.General.Links.Show))
	}
	// Should contain "a" and "c"
	found := map[string]bool{}
	for _, l := range cfg.General.Links.Show {
		found[l] = true
	}
	if !found["a"] || !found["c"] {
		t.Errorf("expected 'a' and 'c' in Show, got %v", cfg.General.Links.Show)
	}
}

func TestRenderTOML(t *testing.T) {
	cfg := &TelemtConfig{
		General: GeneralConfig{
			FastMode:       true,
			UseMiddleProxy: true,
			LogLevel:       "normal",
			AdTag:          "test_tag",
			Modes:          ModesConfig{Classic: false, Secure: true, TLS: true},
			Links:          LinksConfig{Show: []string{"user1"}},
		},
		Server: ServerConfig{
			Port:             443,
			ListenAddrIPv4:   "0.0.0.0",
			ListenAddrIPv6:   "::",
			MetricsListen:    "127.0.0.1:9091",
			MetricsWhitelist: []string{"127.0.0.1"},
		},
		Timeouts: TimeoutsConfig{
			ClientHandshake: 30,
			TGConnect:       10,
			ClientKeepalive: 15,
			ClientAck:       90,
		},
		Censorship: CensorshipConfig{
			TLSDomain:        "example.com",
			UnknownSNIAction: "accept",
			Mask:             false,
			MaskPort:         8443,
			FakeCertLen:      32,
		},
		Access: AccessConfig{
			ReplayCheckLen:   65536,
			ReplayWindowSecs: 1800,
			Users:            map[string]string{"user1": "aa11bb22cc33dd44ee55ff6677889900"},
		},
	}

	toml := renderTOML(cfg)

	// Check key sections exist
	checks := []string{
		"[general]",
		"[general.modes]",
		"[general.links]",
		"[server]",
		"[timeouts]",
		"[censorship]",
		"[access]",
		"[access.users]",
		`ad_tag = "test_tag"`,
		`user1 = "aa11bb22cc33dd44ee55ff6677889900"`,
		"port = 443",
		`metrics_listen = "127.0.0.1:9091"`,
	}
	for _, check := range checks {
		if !strings.Contains(toml, check) {
			t.Errorf("renderTOML missing: %q", check)
		}
	}

	// Semantic checks: the rendered config must be valid TOML and place
	// tg_connect in [general] (an engine [general] key), never in [timeouts]
	// or a non-existent [telegram] table.
	parsed := parseRenderedTOML(t, cfg)
	if _, ok := parsed["telegram"]; ok {
		t.Error("renderTOML must not emit a [telegram] table")
	}
	general := tomlTable(t, parsed, "general")
	if general["tg_connect"] != int64(10) {
		t.Errorf("general.tg_connect = %v, want 10", general["tg_connect"])
	}
	if timeouts := tomlTable(t, parsed, "timeouts"); timeouts["tg_connect"] != nil {
		t.Errorf("timeouts must not contain tg_connect, got %v", timeouts["tg_connect"])
	}
}

func TestRenderTOML_TelegramURLsInGeneral(t *testing.T) {
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{TGConnect: 10},
		Telegram: TelegramConfig{
			ProxySecretURL:   "https://example.com/getProxySecret",
			ProxyConfigV4URL: "https://example.com/getProxyConfig",
			ProxyConfigV6URL: "https://example.com/getProxyConfigV6",
		},
	}

	parsed := parseRenderedTOML(t, cfg)
	if _, ok := parsed["telegram"]; ok {
		t.Error("telegram URLs must not be rendered under a [telegram] table")
	}
	general := tomlTable(t, parsed, "general")
	want := map[string]string{
		"proxy_secret_url":    "https://example.com/getProxySecret",
		"proxy_config_v4_url": "https://example.com/getProxyConfig",
		"proxy_config_v6_url": "https://example.com/getProxyConfigV6",
	}
	for key, val := range want {
		if general[key] != val {
			t.Errorf("general.%s = %v, want %q", key, general[key], val)
		}
	}
}

func TestRenderTOML_NoTelegramURLsWhenEmpty(t *testing.T) {
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{TGConnect: 10},
		// Telegram left as zero value — no custom URLs configured.
	}

	general := tomlTable(t, parseRenderedTOML(t, cfg), "general")
	for _, key := range []string{"proxy_secret_url", "proxy_config_v4_url", "proxy_config_v6_url"} {
		if _, ok := general[key]; ok {
			t.Errorf("general must not contain %q when no URL is configured", key)
		}
	}
}

func TestRenderTOML_UserRateLimits(t *testing.T) {
	access := newEmptyAccess()
	applySecretsToAccess(&access, []SecretEntry{
		{Label: "alice", SecretKey: "aa11bb22cc33dd44ee55ff6677889900", Enabled: true, RateLimitUpBps: 1_000_000, RateLimitDownBps: 5_000_000},
		{Label: "bob", SecretKey: "bb11bb22cc33dd44ee55ff6677889900", Enabled: true},
	})
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{TGConnect: 10},
		Access:   access,
	}

	parsed := parseRenderedTOML(t, cfg)
	accessTbl := tomlTable(t, parsed, "access")
	rl, ok := accessTbl["user_rate_limits"].(map[string]any)
	if !ok {
		t.Fatalf("expected [access.user_rate_limits] table, got %T", accessTbl["user_rate_limits"])
	}
	alice, ok := rl["alice"].(map[string]any)
	if !ok {
		t.Fatalf("expected user_rate_limits.alice table, got %T", rl["alice"])
	}
	if alice["up_bps"] != int64(1_000_000) {
		t.Errorf("alice.up_bps = %v, want 1000000", alice["up_bps"])
	}
	if alice["down_bps"] != int64(5_000_000) {
		t.Errorf("alice.down_bps = %v, want 5000000", alice["down_bps"])
	}
	// bob has no rate limit → must not appear.
	if _, ok := rl["bob"]; ok {
		t.Error("user_rate_limits must not contain bob (no limit set)")
	}
}

func TestRenderTOML_UserEnabled(t *testing.T) {
	access := newEmptyAccess()
	applySecretsToAccess(&access, []SecretEntry{
		{Label: "alice", SecretKey: "aa11bb22cc33dd44ee55ff6677889900", Enabled: true},
		{Label: "bob", SecretKey: "bb11bb22cc33dd44ee55ff6677889900", Enabled: false},
	})
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{TGConnect: 10},
		Access:   access,
	}

	parsed := parseRenderedTOML(t, cfg)
	accessTbl := tomlTable(t, parsed, "access")
	// Both secrets present in [access.users].
	users, _ := accessTbl["users"].(map[string]any)
	if _, ok := users["alice"]; !ok {
		t.Error("alice must be in [access.users]")
	}
	if _, ok := users["bob"]; !ok {
		t.Error("bob (disabled) must still be in [access.users]")
	}
	// user_enabled marks only the disabled one false.
	ue, ok := accessTbl["user_enabled"].(map[string]any)
	if !ok {
		t.Fatalf("expected [access.user_enabled] table, got %T", accessTbl["user_enabled"])
	}
	if ue["bob"] != false {
		t.Errorf("user_enabled.bob = %v, want false", ue["bob"])
	}
	if _, ok := ue["alice"]; ok {
		t.Error("user_enabled must not contain alice (enabled = omitted)")
	}
}

func TestRenderTOML_WithMaskingHost(t *testing.T) {
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{},
		Censorship: CensorshipConfig{
			TLSDomain: "example.com",
			Mask:      true,
			MaskHost:  "mask.example.com",
		},
		Access: AccessConfig{Users: map[string]string{}},
	}

	toml := renderTOML(cfg)
	if !strings.Contains(toml, `mask_host = "mask.example.com"`) {
		t.Error("mask_host should be present when Mask is true and MaskHost is set")
	}
}

func TestRenderTOML_WithProxyProtocolCIDRs(t *testing.T) {
	cfg := &TelemtConfig{
		General: GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server: ServerConfig{
			ProxyProtocol:             true,
			ProxyProtocolTrustedCIDRs: []string{"10.0.0.0/8"},
			MetricsWhitelist:          []string{},
		},
		Timeouts:   TimeoutsConfig{},
		Censorship: CensorshipConfig{},
		Access:     AccessConfig{Users: map[string]string{}},
	}

	toml := renderTOML(cfg)
	if !strings.Contains(toml, "proxy_protocol_trusted_cidrs") {
		t.Error("proxy_protocol_trusted_cidrs should be present")
	}
}

func TestRenderTOML_WithUpstreams(t *testing.T) {
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{},
		Access:   AccessConfig{Users: map[string]string{}},
		Upstreams: []UpstreamConfig{
			{Type: "socks5", Weight: 10, Address: "proxy:1080", Username: "u", Password: "p"},
		},
	}

	toml := renderTOML(cfg)
	if !strings.Contains(toml, "[[upstreams]]") {
		t.Error("upstreams section should be present")
	}
	if !strings.Contains(toml, `type = "socks5"`) {
		t.Error("upstream type should be present")
	}
}

func TestBuildConfig_ShadowsocksUpstream(t *testing.T) {
	const ssURL = "ss://2022-blake3-aes-256-gcm:cGFzcw==@127.0.0.1:8388"
	params := &ConfigParams{
		Settings: &model.Settings{ProxyPort: 443, ProxyMetricsPort: 9091},
		Upstreams: []UpstreamEntry{
			{Type: model.UpstreamShadowsocks, URL: ssURL, Weight: 10, Enabled: true},
		},
	}

	cfg := BuildConfig(params)
	if len(cfg.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(cfg.Upstreams))
	}
	uc := cfg.Upstreams[0]
	if uc.Type != "shadowsocks" {
		t.Errorf("type = %q, want shadowsocks", uc.Type)
	}
	if uc.URL != ssURL {
		t.Errorf("url = %q, want %q", uc.URL, ssURL)
	}
	if uc.Address != "" || uc.Username != "" || uc.Password != "" || uc.UserID != "" {
		t.Errorf("shadowsocks upstream must not set address/username/password/user_id, got %+v", uc)
	}
}

func TestRenderTOML_ShadowsocksUpstream(t *testing.T) {
	const ssURL = "ss://2022-blake3-aes-256-gcm:cGFzcw==@127.0.0.1:8388"
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{},
		Access:   AccessConfig{Users: map[string]string{}},
		Upstreams: []UpstreamConfig{
			{Type: "shadowsocks", Weight: 10, URL: ssURL},
		},
	}

	toml := renderTOML(cfg)
	if !strings.Contains(toml, `type = "shadowsocks"`) {
		t.Error("shadowsocks type line missing")
	}
	if !strings.Contains(toml, `url = "`+ssURL+`"`) {
		t.Errorf("url line missing or wrong; got:\n%s", toml)
	}
	// The shadowsocks block must not emit socks-only keys.
	if strings.Contains(toml, "address = ") || strings.Contains(toml, "username = ") || strings.Contains(toml, "password = ") || strings.Contains(toml, "user_id = ") {
		t.Errorf("shadowsocks block must not contain address/username/password/user_id; got:\n%s", toml)
	}
}

func TestRenderTOML_UserLimits(t *testing.T) {
	cfg := &TelemtConfig{
		General:  GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:   ServerConfig{MetricsWhitelist: []string{}},
		Timeouts: TimeoutsConfig{},
		Access: AccessConfig{
			Users:            map[string]string{"user1": "secret"},
			UserMaxTCPConns:  map[string]int{"user1": 5},
			UserMaxUniqueIPs: map[string]int{"user1": 3},
			UserDataQuota:    map[string]int64{"user1": 1073741824},
			UserExpirations:  map[string]string{"user1": "2026-12-31"},
		},
	}

	toml := renderTOML(cfg)
	checks := []string{
		"[access.user_max_tcp_conns]",
		"[access.user_max_unique_ips]",
		"[access.user_data_quota]",
		"[access.user_expirations]",
		"user1 = 5",
		"user1 = 3",
		"user1 = 1073741824",
		`user1 = "2026-12-31"`,
	}
	for _, check := range checks {
		if !strings.Contains(toml, check) {
			t.Errorf("renderTOML missing: %q", check)
		}
	}
}

func TestWriteConfigTOML(t *testing.T) {
	cfg := &TelemtConfig{
		General:    GeneralConfig{Modes: ModesConfig{}, Links: LinksConfig{}},
		Server:     ServerConfig{MetricsWhitelist: []string{}},
		Timeouts:   TimeoutsConfig{},
		Censorship: CensorshipConfig{},
		Access:     AccessConfig{Users: map[string]string{}},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.toml")

	if err := WriteConfigTOML(cfg, path); err != nil {
		t.Fatalf("WriteConfigTOML: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) == 0 {
		t.Error("config file is empty")
	}
	if !strings.Contains(string(data), "[general]") {
		t.Error("config file missing [general]")
	}
}

func TestFormatStringArray(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{nil, "[]"},
		{[]string{}, "[]"},
		{[]string{"a"}, `["a"]`},
		{[]string{"a", "b"}, `["a", "b"]`},
	}

	for _, tt := range tests {
		got := formatStringArray(tt.input)
		if got != tt.want {
			t.Errorf("formatStringArray(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetEnabledLabels(t *testing.T) {
	secrets := []SecretEntry{
		{Label: "a", Enabled: true},
		{Label: "b", Enabled: false},
		{Label: "c", Enabled: true},
	}

	labels := getEnabledLabels(secrets)
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0] != "a" || labels[1] != "c" {
		t.Errorf("labels = %v, want [a c]", labels)
	}

	// Empty
	labels = getEnabledLabels(nil)
	if len(labels) != 0 {
		t.Errorf("expected empty, got %v", labels)
	}
}

func TestParseCIDRList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"10.0.0.0/8, 172.16.0.0/12", []string{"10.0.0.0/8", "172.16.0.0/12"}},
		{"  10.0.0.0/8 , 172.16.0.0/12  ", []string{"10.0.0.0/8", "172.16.0.0/12"}},
		{",,", nil},
		{"", nil},
	}

	for _, tt := range tests {
		got := parseCIDRList(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseCIDRList(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i, v := range got {
			if v != tt.want[i] {
				t.Errorf("parseCIDRList(%q)[%d] = %q, want %q", tt.input, i, v, tt.want[i])
			}
		}
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]string{"c": "3", "a": "1", "b": "2"}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("sortedKeys = %v, want [a b c]", keys)
	}
}

func TestSortedKeysInt(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("sortedKeys[int] = %v, want [a b c]", keys)
	}
}

func TestSortedKeysInt64(t *testing.T) {
	m := map[string]int64{"c": 3, "a": 1, "b": 2}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("sortedKeys[int64] = %v, want [a b c]", keys)
	}
}

func TestSortedKeysStr(t *testing.T) {
	m := map[string]string{"c": "3", "a": "1", "b": "2"}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("sortedKeys[string] = %v, want [a b c]", keys)
	}
}

func TestRenderTOML_SkipsInvalidLabels(t *testing.T) {
	cfg := &TelemtConfig{
		General: GeneralConfig{
			Modes: ModesConfig{Secure: true},
		},
		Server: ServerConfig{Port: 443, MetricsListen: "127.0.0.1:9091"},
		Timeouts: TimeoutsConfig{
			ClientHandshake: 30, TGConnect: 10,
			ClientKeepalive: 15, ClientAck: 90,
		},
		Access: AccessConfig{
			ReplayCheckLen:   65536,
			ReplayWindowSecs: 1800,
			Users: map[string]string{
				"good_user": "aa11bb22cc33dd44ee55ff6677889900",
				"bad\nkey":  "should_be_skipped",
				"=injected": "should_be_skipped",
				"also_good": "11223344556677889900aabbccddeeff",
			},
			UserMaxTCPConns: map[string]int{
				"good_user": 10,
				"bad key":   20,
			},
		},
	}

	toml := renderTOML(cfg)

	if !strings.Contains(toml, `good_user = "aa11bb22cc33dd44ee55ff6677889900"`) {
		t.Error("valid label good_user should appear in TOML")
	}
	if !strings.Contains(toml, `also_good = "11223344556677889900aabbccddeeff"`) {
		t.Error("valid label also_good should appear in TOML")
	}
	if strings.Contains(toml, "bad\\nkey") || strings.Contains(toml, "bad\nkey") {
		t.Error("invalid label with newline should be skipped")
	}
	if strings.Contains(toml, "=injected") {
		t.Error("invalid label with = should be skipped")
	}
	if strings.Contains(toml, "bad key") {
		t.Error("invalid label with space should be skipped in UserMaxTCPConns")
	}
}
