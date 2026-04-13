package service

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OSType represents the details of the host OS.
type OSType struct {
	Family  string `json:"family"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

// DetectOS detects the operating system details.
func DetectOS() *OSType {
	res := &OSType{
		Family:  "unknown",
		Version: "unknown",
		Arch:    runtime.GOARCH,
	}

	// Check for macOS
	if runtime.GOOS == "darwin" {
		res.Family = "macos"
		// Simple version check for macOS
		if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			res.Version = strings.TrimSpace(string(out))
		}
		return res
	}

	// Check /etc/os-release for Linux
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		content := strings.ToLower(string(data))
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "id=") {
				res.Family = strings.Trim(strings.TrimPrefix(line, "id="), "\"")
			}
			if strings.HasPrefix(line, "version_id=") {
				res.Version = strings.Trim(strings.TrimPrefix(line, "version_id="), "\"")
			}
		}

		// Map to common families if needed
		switch {
		case containsAny(content, "ubuntu", "debian", "pop", "linuxmint", "kali"):
			if res.Family == "unknown" {
				res.Family = "debian"
			}
		case containsAny(content, "centos", "rhel", "fedora", "rocky", "alma", "oracle"):
			if res.Family == "unknown" {
				res.Family = "rhel"
			}
		case strings.Contains(content, "alpine"):
			if res.Family == "unknown" {
				res.Family = "alpine"
			}
		}
		return res
	}

	// Fallback Linux checks
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		res.Family = "debian"
		if v, err := os.ReadFile("/etc/debian_version"); err == nil {
			res.Version = strings.TrimSpace(string(v))
		}
		return res
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		res.Family = "rhel"
		// Could parse version here, but common distributions use os-release anyway
		return res
	}

	return res
}

// InstallSystemdService creates and enables the popugate systemd service.
func InstallSystemdService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd is not supported on this system (systemctl not found)")
	}

	binaryPath, err := exec.LookPath("popugate")
	if err != nil {
		binaryPath = "/usr/local/bin/popugate"
	}

	dataDir := os.Getenv("POPUGATE_DATA_DIR")
	if dataDir == "" {
		// Attempt to use current working directory or absolute path to it
		if cwd, err := os.Getwd(); err == nil {
			dataDir = cwd
		} else {
			dataDir = "/opt/popugate"
		}
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=PopuGate Telegram Proxy Manager
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
ExecStart=%s serve
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
WorkingDirectory=%s
Environment=POPUGATE_DATA_DIR=%s

[Install]
WantedBy=multi-user.target
`, binaryPath, dataDir, dataDir)

	servicePath := "/etc/systemd/system/popugate.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}

	// Enable the service (best-effort)
	_ = exec.Command("systemctl", "enable", "popugate").Run()

	return nil
}

// UninstallSystemdService removes the systemd service.
func UninstallSystemdService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd is not supported on this system")
	}
	_ = exec.Command("systemctl", "stop", "popugate").Run()
	_ = exec.Command("systemctl", "disable", "popugate").Run()
	os.Remove("/etc/systemd/system/popugate.service")
	_ = exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

// IsSystemdInstalled checks if the popugate service is installed.
func IsSystemdInstalled() bool {
	_, err := os.Stat("/etc/systemd/system/popugate.service")
	return err == nil
}

// SystemdServiceStatus represents the status of the systemd service.
type SystemdServiceStatus struct {
	Supported bool   `json:"supported"`
	Installed bool   `json:"installed"`
	Active    string `json:"active"`
	Enabled   bool   `json:"enabled"`
	PID       string `json:"pid,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
}

// GetServiceStatus queries systemctl for detailed service status.
func GetServiceStatus() *SystemdServiceStatus {
	status := &SystemdServiceStatus{
		Supported: false,
		Installed: false,
		Active:    "Not Installed",
	}

	// Check if systemctl exists
	if _, err := exec.LookPath("systemctl"); err != nil {
		status.Active = "Unsupported (No systemd)"
		return status
	}

	status.Supported = true
	status.Installed = IsSystemdInstalled()

	status.Active = "unknown"

	// Active state
	if out, err := exec.Command("systemctl", "show", "popugate", "--property=ActiveState").Output(); err == nil {
		if v := parseSystemctlProperty(string(out)); v != "" {
			status.Active = v
		}
	}

	// Enabled state
	if out, err := exec.Command("systemctl", "is-enabled", "popugate").Output(); err == nil {
		status.Enabled = strings.TrimSpace(string(out)) == "enabled"
	}

	// Main PID
	if out, err := exec.Command("systemctl", "show", "popugate", "--property=MainPID").Output(); err == nil {
		if v := parseSystemctlProperty(string(out)); v != "" && v != "0" {
			status.PID = v
		}
	}

	// Uptime
	if out, err := exec.Command("systemctl", "show", "popugate", "--property=ActiveEnterTimestamp").Output(); err == nil {
		if v := parseSystemctlProperty(string(out)); v != "" {
			status.Uptime = v
		}
	}

	return status
}

// RestartService restarts the popugate systemd service.
func RestartService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd is not supported on this system")
	}
	if !IsSystemdInstalled() {
		return fmt.Errorf("systemd service not installed")
	}
	return exec.Command("systemctl", "restart", "popugate").Run()
}

// ReloadService sends a reload signal to the systemd service.
func ReloadService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd is not supported on this system")
	}
	if !IsSystemdInstalled() {
		return fmt.Errorf("systemd service not installed")
	}
	return exec.Command("systemctl", "reload", "popugate").Run()
}

// parseSystemctlProperty extracts the value from "KEY=VALUE\n" output.
func parseSystemctlProperty(out string) string {
	parts := strings.SplitN(strings.TrimSpace(out), "=", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
