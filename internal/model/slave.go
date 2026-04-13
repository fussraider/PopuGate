package model

import "fmt"

// Slave represents a replication slave server.
type Slave struct {
	ID       int64  `json:"id" db:"id"`
	Host     string `json:"host" db:"host"`
	Port     int    `json:"port" db:"port"`
	Label    string `json:"label" db:"label"`
	Enabled  bool   `json:"enabled" db:"enabled"`
	LastSync int64  `json:"last_sync" db:"last_sync"`
	Status   string `json:"status" db:"status"`
}

// Validate checks slave fields.
func (s *Slave) Validate() error {
	if s.Host == "" {
		return fmt.Errorf("host is required")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be 1-65535")
	}
	if s.Label == "" {
		s.Label = s.Host
	}
	return nil
}

// SlaveTestResult holds the result of an SSH connectivity test.
type SlaveTestResult struct {
	Host         string `json:"host"`
	SSHOK        bool   `json:"ssh_ok"`
	DockerStatus string `json:"docker_status,omitempty"`
	Error        string `json:"error,omitempty"`
}
