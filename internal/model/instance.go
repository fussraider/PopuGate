package model

import (
	"fmt"
	"path/filepath"
)

// Instance represents a multi-port proxy instance.
type Instance struct {
	ID          int64  `json:"id" db:"id"`
	Port        int    `json:"port" db:"port"`
	MetricsPort int    `json:"metrics_port" db:"metrics_port"`
	Enabled     bool   `json:"enabled" db:"enabled"`
	Label       string `json:"label" db:"label"`
}

// Validate checks instance fields.
func (i *Instance) Validate() error {
	if i.Port < 1 || i.Port > 65535 {
		return fmt.Errorf("port must be 1-65535")
	}
	if i.MetricsPort < 1 || i.MetricsPort > 65535 {
		return fmt.Errorf("metrics_port must be 1-65535")
	}
	return nil
}

// ContainerName returns the Docker container name for this instance.
func (i *Instance) ContainerName() string {
	if i.Port == 443 {
		return "popugate"
	}
	return fmt.Sprintf("popugate-%d", i.Port)
}

// ConfigPath returns the TOML config file path for this instance.
func (i *Instance) ConfigPath() string {
	return filepath.Join(InstallDir, fmt.Sprintf("mtproxy/config-%d.toml", i.Port))
}
