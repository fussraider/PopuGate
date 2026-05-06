package model

import (
	"fmt"
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
			},
			wantErr: false,
		},
		{
			name: "valid high port",
			instance: Instance{
				Port:        65535,
				MetricsPort: 9091,
			},
			wantErr: false,
		},
		{
			name: "valid port 1",
			instance: Instance{
				Port:        1,
				MetricsPort: 1,
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
		{443, "popugate"},
		{8443, "popugate-8443"},
		{1, "popugate-1"},
		{65535, "popugate-65535"},
		{8090, "popugate-8090"},
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
