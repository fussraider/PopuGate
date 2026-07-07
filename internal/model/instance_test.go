package model

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceValidate(t *testing.T) {
	tests := []struct {
		name     string
		instance Instance
		wantErr  bool
	}{
		{
			name: "valid port and metrics port",
			instance: Instance{
				Port:        443,
				MetricsPort: 9090,
				TLSDomain:   "cloudflare.com",
			},
			wantErr: false,
		},
		{
			name: "valid high port",
			instance: Instance{
				Port:        65535,
				MetricsPort: 9091,
				TLSDomain:   "cloudflare.com",
			},
			wantErr: false,
		},
		{
			name: "valid port 1",
			instance: Instance{
				Port:        1,
				MetricsPort: 1,
				TLSDomain:   "cloudflare.com",
			},
			wantErr: false,
		},
		{
			name: "port zero invalid",
			instance: Instance{
				Port:        0,
				MetricsPort: 9090,
			},
			wantErr: true,
		},
		{
			name: "port 65536 invalid",
			instance: Instance{
				Port:        65536,
				MetricsPort: 9090,
			},
			wantErr: true,
		},
		{
			name: "negative port invalid",
			instance: Instance{
				Port:        -1,
				MetricsPort: 9090,
			},
			wantErr: true,
		},
		{
			name: "metrics port zero invalid",
			instance: Instance{
				Port:        443,
				MetricsPort: 0,
			},
			wantErr: true,
		},
		{
			name: "metrics port 65536 invalid",
			instance: Instance{
				Port:        443,
				MetricsPort: 65536,
			},
			wantErr: true,
		},
		{
			name: "both ports invalid",
			instance: Instance{
				Port:        0,
				MetricsPort: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.instance.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInstanceContainerName(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{
		{443, "popugate-telemt-443"},
		{8443, "popugate-telemt-8443"},
		{1, "popugate-telemt-1"},
		{65535, "popugate-telemt-65535"},
		{8090, "popugate-telemt-8090"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("port_%d", tt.port), func(t *testing.T) {
			i := Instance{Port: tt.port}
			got := i.ContainerName()
			if got != tt.want {
				t.Errorf("ContainerName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstanceConfigPath(t *testing.T) {
	// Save and restore InstallDir
	origDir := InstallDir
	InstallDir = "/opt/popugate"
	defer func() { InstallDir = origDir }()

	tests := []struct {
		port int
		want string
	}{
		{443, "/opt/popugate/mtproxy/config-443.toml"},
		{8443, "/opt/popugate/mtproxy/config-8443.toml"},
		{1234, "/opt/popugate/mtproxy/config-1234.toml"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("port_%d", tt.port), func(t *testing.T) {
			i := Instance{Port: tt.port}
			got := i.ConfigPath()
			if got != tt.want {
				t.Errorf("ConfigPath() = %q, want %q", got, tt.want)
			}
			// Verify it contains the expected path structure
			if !strings.Contains(got, "mtproxy/config-") {
				t.Errorf("ConfigPath() = %q should contain 'mtproxy/config-'", got)
			}
			if !strings.Contains(got, fmt.Sprintf("config-%d.toml", tt.port)) {
				t.Errorf("ConfigPath() = %q should contain 'config-%d.toml'", got, tt.port)
			}
		})
	}
}

func TestTagsMatch(t *testing.T) {
	tests := []struct {
		name         string
		instanceTags []string
		secretTags   []string
		want         bool
	}{
		{"both empty", nil, nil, true},
		{"instance no tags, secret has tags", nil, []string{"premium"}, true},
		{"secret no tags, instance has tags", []string{"premium"}, nil, false},
		{"matching tag", []string{"premium"}, []string{"premium"}, true},
		{"no matching tags", []string{"premium"}, []string{"basic"}, false},
		{"partial match", []string{"premium", "vip"}, []string{"vip", "test"}, true},
		{"multiple no match", []string{"premium", "vip"}, []string{"basic", "free"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TagsMatch(tt.instanceTags, tt.secretTags); got != tt.want {
				t.Errorf("TagsMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstanceConfigPathCustomInstallDir(t *testing.T) {
	origDir := InstallDir
	InstallDir = "/custom/data"
	defer func() { InstallDir = origDir }()

	i := Instance{Port: 443}
	got := i.ConfigPath()
	want := "/custom/data/mtproxy/config-443.toml"
	if got != want {
		t.Errorf("ConfigPath() with custom InstallDir = %q, want %q", got, want)
	}
}

func TestInstanceValidate_TCPMSSDefault(t *testing.T) {
	inst := &Instance{Port: 443, MetricsPort: 9091, TLSDomain: "example.com"}
	if err := inst.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.TCPMSS != 88 {
		t.Fatalf("expected default TCPMSS=88, got %d", inst.TCPMSS)
	}
}

func TestInstanceValidate_TCPMSSEnabledValid(t *testing.T) {
	inst := &Instance{Port: 443, MetricsPort: 9091, TLSDomain: "example.com", TCPMSSEnabled: true, TCPMSS: 120}
	if err := inst.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstanceValidate_TCPMSSOutOfRange(t *testing.T) {
	tests := []struct {
		name string
		mss  int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too_low", 50},
		{"too_high", 4097},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instance{Port: 443, MetricsPort: 9091, TLSDomain: "example.com", TCPMSSEnabled: true, TCPMSS: tt.mss}
			// Zero should be replaced by default before the range check
			if tt.mss == 0 {
				if err := inst.Validate(); err != nil {
					t.Fatalf("zero MSS should get default, got error: %v", err)
				}
				return
			}
			if err := inst.Validate(); err == nil {
				t.Errorf("expected error for MSS=%d", tt.mss)
			}
		})
	}
}

func TestInstanceTLSFrontDirPath(t *testing.T) {
	origDir := InstallDir
	InstallDir = "/opt/popugate"
	defer func() { InstallDir = origDir }()

	inst := &Instance{Port: 443}
	expected := filepath.Join(InstallDir, "mtproxy/tlsfront-443")
	if got := inst.TLSFrontDirPath(); got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestInstanceValidate_TLSFrontingRequiresFakeTLS(t *testing.T) {
	inst := &Instance{Port: 443, MetricsPort: 9091, TLSDomain: "example.com", TLSFronting: true, FakeTLS: false}
	if err := inst.Validate(); err == nil {
		t.Fatal("expected error when tls_fronting=true but fake_tls=false")
	}
}

func TestInstanceValidate_TLSFrontingWithFakeTLS(t *testing.T) {
	inst := &Instance{Port: 443, MetricsPort: 9091, TLSDomain: "example.com", TLSFronting: true, FakeTLS: true}
	if err := inst.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetTags_Empty(t *testing.T) {
	inst := Instance{Tags: ""}
	if tags := inst.GetTags(); tags != nil {
		t.Fatalf("expected nil for empty tags, got %v", tags)
	}
}

func TestGetTags_EmptyArray(t *testing.T) {
	inst := Instance{Tags: "[]"}
	if tags := inst.GetTags(); tags != nil {
		t.Fatalf("expected nil for empty array, got %v", tags)
	}
}

func TestGetTags_Valid(t *testing.T) {
	inst := Instance{Tags: `["vip","paid"]`}
	tags := inst.GetTags()
	if len(tags) != 2 || tags[0] != "vip" || tags[1] != "paid" {
		t.Fatalf("expected [vip paid], got %v", tags)
	}
}

func TestGetTags_InvalidJSON(t *testing.T) {
	inst := Instance{Tags: "not-json"}
	tags := inst.GetTags()
	if tags != nil {
		t.Fatalf("expected nil for invalid JSON, got %v", tags)
	}
}

func TestGetMaskHost_Fallback(t *testing.T) {
	inst := Instance{TLSDomain: "cloudflare.com"}
	if h := inst.GetMaskHost(); h != "cloudflare.com" {
		t.Fatalf("expected TLSDomain fallback, got %s", h)
	}
}

func TestGetMaskHost_Custom(t *testing.T) {
	inst := Instance{TLSDomain: "cloudflare.com", MaskHost: "custom.host.com"}
	if h := inst.GetMaskHost(); h != "custom.host.com" {
		t.Fatalf("expected custom mask host, got %s", h)
	}
}
