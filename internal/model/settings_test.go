package model

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()

	if s.ProxyPort != 443 {
		t.Errorf("ProxyPort = %d, want 443", s.ProxyPort)
	}
	if s.ProxyMetricsPort != 9090 {
		t.Errorf("ProxyMetricsPort = %d, want 9090", s.ProxyMetricsPort)
	}
	if s.ProxyDomain != "cloudflare.com" {
		t.Errorf("ProxyDomain = %q, want cloudflare.com", s.ProxyDomain)
	}
	if s.ProxyConcurrency != 8192 {
		t.Errorf("ProxyConcurrency = %d, want 8192", s.ProxyConcurrency)
	}
	if s.FakeCertLen != 2048 {
		t.Errorf("FakeCertLen = %d, want 2048", s.FakeCertLen)
	}
	if s.GeoblockMode != "blacklist" {
		t.Errorf("GeoblockMode = %q, want blacklist", s.GeoblockMode)
	}
	if !s.MaskingEnabled {
		t.Error("MaskingEnabled = false, want true")
	}
	if s.MaskingPort != 443 {
		t.Errorf("MaskingPort = %d, want 443", s.MaskingPort)
	}
	if s.UnknownSNIAction != "mask" {
		t.Errorf("UnknownSNIAction = %q, want mask", s.UnknownSNIAction)
	}
	if s.TelegramInterval != 6 {
		t.Errorf("TelegramInterval = %d, want 6", s.TelegramInterval)
	}
	if !s.TelegramAlertsEnabled {
		t.Error("TelegramAlertsEnabled = false, want true")
	}
	if s.TelegramServerLabel != "PopuGate" {
		t.Errorf("TelegramServerLabel = %q, want PopuGate", s.TelegramServerLabel)
	}
	if !s.AutoUpdateEnabled {
		t.Error("AutoUpdateEnabled = false, want true")
	}
	if s.ReplicationRole != "standalone" {
		t.Errorf("ReplicationRole = %q, want standalone", s.ReplicationRole)
	}
	if s.ReplicationSyncInterval != 60 {
		t.Errorf("ReplicationSyncInterval = %d, want 60", s.ReplicationSyncInterval)
	}
	if s.ReplicationSSHPort != 22 {
		t.Errorf("ReplicationSSHPort = %d, want 22", s.ReplicationSSHPort)
	}
	if s.ReplicationSSHUser != "root" {
		t.Errorf("ReplicationSSHUser = %q, want root", s.ReplicationSSHUser)
	}
	if s.BackupRetentionDays != 7 {
		t.Errorf("BackupRetentionDays = %d, want 7", s.BackupRetentionDays)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(*Settings)
		field    string
		expected interface{}
	}{
		{
			"invalid proxy port",
			func(s *Settings) { s.ProxyPort = 0 },
			"ProxyPort", 443,
		},
		{
			"high proxy port",
			func(s *Settings) { s.ProxyPort = 99999 },
			"ProxyPort", 443,
		},
		{
			"invalid metrics port",
			func(s *Settings) { s.ProxyMetricsPort = -1 },
			"ProxyMetricsPort", 9090,
		},
		{
			"zero concurrency",
			func(s *Settings) { s.ProxyConcurrency = 0 },
			"ProxyConcurrency", 8192,
		},
		{
			"small fake cert len",
			func(s *Settings) { s.FakeCertLen = 100 },
			"FakeCertLen", 2048,
		},
		{
			"invalid masking port",
			func(s *Settings) { s.MaskingPort = 0 },
			"MaskingPort", 443,
		},
		{
			"drop sni action preserved",
			func(s *Settings) { s.UnknownSNIAction = "drop" },
			"UnknownSNIAction", "drop",
		},
		{
			"invalid sni action reset",
			func(s *Settings) { s.UnknownSNIAction = "pass" },
			"UnknownSNIAction", "mask",
		},
		{
			"whitelist mode preserved",
			func(s *Settings) { s.GeoblockMode = "whitelist" },
			"GeoblockMode", "whitelist",
		},
		{
			"invalid geoblock mode reset",
			func(s *Settings) { s.GeoblockMode = "block" },
			"GeoblockMode", "blacklist",
		},
		{
			"zero telegram interval",
			func(s *Settings) { s.TelegramInterval = 0 },
			"TelegramInterval", 6,
		},
		{
			"master role preserved",
			func(s *Settings) { s.ReplicationRole = "master" },
			"ReplicationRole", "master",
		},
		{
			"slave role preserved",
			func(s *Settings) { s.ReplicationRole = "slave" },
			"ReplicationRole", "slave",
		},
		{
			"invalid replication role",
			func(s *Settings) { s.ReplicationRole = "client" },
			"ReplicationRole", "standalone",
		},
		{
			"low sync interval",
			func(s *Settings) { s.ReplicationSyncInterval = 5 },
			"ReplicationSyncInterval", 60,
		},
		{
			"invalid ssh port",
			func(s *Settings) { s.ReplicationSSHPort = 0 },
			"ReplicationSSHPort", 22,
		},
		{
			"zero backup retention",
			func(s *Settings) { s.BackupRetentionDays = 0 },
			"BackupRetentionDays", 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultSettings()
			tt.modify(&s)
			s.Validate()

			var got interface{}
			switch tt.field {
			case "ProxyPort":
				got = s.ProxyPort
			case "ProxyMetricsPort":
				got = s.ProxyMetricsPort
			case "ProxyConcurrency":
				got = s.ProxyConcurrency
			case "FakeCertLen":
				got = s.FakeCertLen
			case "MaskingPort":
				got = s.MaskingPort
			case "UnknownSNIAction":
				got = s.UnknownSNIAction
			case "GeoblockMode":
				got = s.GeoblockMode
			case "TelegramInterval":
				got = s.TelegramInterval
			case "ReplicationRole":
				got = s.ReplicationRole
			case "ReplicationSyncInterval":
				got = s.ReplicationSyncInterval
			case "ReplicationSSHPort":
				got = s.ReplicationSSHPort
			case "BackupRetentionDays":
				got = s.BackupRetentionDays
			}

			if got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.field, got, tt.expected)
			}
		})
	}
}

func TestSSHKeyPath(t *testing.T) {
	tests := []struct {
		name        string
		customPath  string
		wantDefault bool
	}{
		{"empty uses default", "", true},
		{"custom path used", "/custom/key", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultSettings()
			s.ReplicationSSHKeyPath = tt.customPath
			got := s.SSHKeyPath()
			if tt.wantDefault {
				if got != DefaultSSHKeyPath() {
					t.Errorf("SSHKeyPath() = %q, want default %q", got, DefaultSSHKeyPath())
				}
			} else if got != tt.customPath {
				t.Errorf("SSHKeyPath() = %q, want %q", got, tt.customPath)
			}
		})
	}
}

func TestSSHLogPath(t *testing.T) {
	tests := []struct {
		name        string
		customLog   string
		wantDefault bool
	}{
		{"empty uses default", "", true},
		{"custom path used", "/var/log/repl.log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultSettings()
			s.ReplicationLog = tt.customLog
			got := s.SSHLogPath()
			if tt.wantDefault {
				if got != DefaultSSHLogPath() {
					t.Errorf("SSHLogPath() = %q, want default %q", got, DefaultSSHLogPath())
				}
			} else if got != tt.customLog {
				t.Errorf("SSHLogPath() = %q, want %q", got, tt.customLog)
			}
		})
	}
}

func TestDefaultSSHKeyPath(t *testing.T) {
	got := DefaultSSHKeyPath()
	if !strings.HasSuffix(got, ".ssh/id_ed25519") {
		t.Errorf("DefaultSSHKeyPath() = %q, should end with .ssh/id_ed25519", got)
	}
}

func TestDefaultSSHLogPath(t *testing.T) {
	got := DefaultSSHLogPath()
	if !strings.HasSuffix(got, "replication.log") {
		t.Errorf("DefaultSSHLogPath() = %q, should end with replication.log", got)
	}
}

func TestConfigDir(t *testing.T) {
	got := ConfigDir()
	if !strings.HasSuffix(got, "mtproxy") {
		t.Errorf("ConfigDir() = %q, should end with mtproxy", got)
	}
}

func TestTelemtVersion_Default(t *testing.T) {
	os.Unsetenv("TELEMT_VERSION")
	got := TelemtVersion()
	if got != DefaultTelemtVer {
		t.Errorf("TelemtVersion() = %q, want %q", got, DefaultTelemtVer)
	}
}

func TestTelemtVersion_Env(t *testing.T) {
	os.Setenv("TELEMT_VERSION", "3.4.0")
	defer os.Unsetenv("TELEMT_VERSION")
	got := TelemtVersion()
	if got != "3.4.0" {
		t.Errorf("TelemtVersion() = %q, want 3.4.0", got)
	}
}

func TestTelemtCommit_Default(t *testing.T) {
	os.Unsetenv("TELEMT_COMMIT")
	got := TelemtCommit()
	if got != DefaultTelemtRef {
		t.Errorf("TelemtCommit() = %q, want %q", got, DefaultTelemtRef)
	}
}

func TestTelemtCommit_Env(t *testing.T) {
	os.Setenv("TELEMT_COMMIT", "abc123")
	defer os.Unsetenv("TELEMT_COMMIT")
	got := TelemtCommit()
	if got != "abc123" {
		t.Errorf("TelemtCommit() = %q, want abc123", got)
	}
}

func TestTelemtRepo_Default(t *testing.T) {
	os.Unsetenv("TELEMT_REPO")
	got := TelemtRepo()
	if got != DefaultTelemtURL {
		t.Errorf("TelemtRepo() = %q, want %q", got, DefaultTelemtURL)
	}
}

func TestTelemtRepo_Env(t *testing.T) {
	os.Setenv("TELEMT_REPO", "https://example.com/repo.git")
	defer os.Unsetenv("TELEMT_REPO")
	got := TelemtRepo()
	if got != "https://example.com/repo.git" {
		t.Errorf("TelemtRepo() = %q, want https://example.com/repo.git", got)
	}
}

func TestVersionURL_Tag(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	defer func() { Version = origVersion; Commit = origCommit }()

	Version = "v1.2.3"
	Commit = "abc"
	got := VersionURL()
	if !strings.Contains(got, "releases/tag/v1.2.3") {
		t.Errorf("VersionURL() = %q, should contain releases/tag/v1.2.3", got)
	}
}

func TestVersionURL_Commit(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	defer func() { Version = origVersion; Commit = origCommit }()

	Version = "dev"
	Commit = "abc123def"
	got := VersionURL()
	if !strings.Contains(got, "commit/abc123def") {
		t.Errorf("VersionURL() = %q, should contain commit/abc123def", got)
	}
}

func TestVersionURL_Fallback(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	defer func() { Version = origVersion; Commit = origCommit }()

	Version = "dev"
	Commit = "unknown"
	got := VersionURL()
	if strings.Contains(got, "releases") || strings.Contains(got, "commit") {
		t.Errorf("VersionURL() = %q, should be repo root", got)
	}
	if !strings.Contains(got, "github.com") {
		t.Errorf("VersionURL() = %q, should contain github.com", got)
	}
}
