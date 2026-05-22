package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/fmtutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

// lastAlertTime tracks when the last alert was sent for each resource type.
var lastAlertTime = make(map[string]time.Time)
var alertMu sync.Mutex

// CheckResources checks system resource usage and alerts if thresholds are exceeded.
func CheckResources(ctx context.Context, notify NotifyFunc) error {
	res := GetResources()
	if res == nil {
		return fmt.Errorf("failed to get system resources")
	}
	checkResourcesWithStats(ctx, res, notify)
	return nil
}

// checkResourcesWithStats applies threshold checks against the provided stats.
// Extracted for unit-testability without requiring real system calls.
func checkResourcesWithStats(ctx context.Context, res *model.SystemResources, notify NotifyFunc) {
	alertMu.Lock()
	defer alertMu.Unlock()

	now := time.Now()
	const alertCooldown = 30 * time.Minute

	// Cleanup entries older than 24 hours to prevent unbounded map growth.
	for k, t := range lastAlertTime {
		if now.Sub(t) > 24*time.Hour {
			delete(lastAlertTime, k)
		}
	}

	// Memory > 95%
	if res.MemoryTotal > 0 {
		memPct := float64(res.MemoryUsed) / float64(res.MemoryTotal) * 100
		if memPct > 95 && now.Sub(lastAlertTime["memory"]) > alertCooldown {
			notify(ctx, "🚨 *%s* High Memory Usage: %.1f%% (%s/%s)", memPct, fmtutil.FormatBytes(res.MemoryUsed), fmtutil.FormatBytes(res.MemoryTotal))
			lastAlertTime["memory"] = now
		}
	}

	// Disk > 90%
	if res.DiskTotal > 0 {
		diskPct := float64(res.DiskUsed) / float64(res.DiskTotal) * 100
		if diskPct > 90 && now.Sub(lastAlertTime["disk"]) > alertCooldown {
			notify(ctx, "🚨 *%s* High Disk Usage: %.1f%% (%s/%s)", diskPct, fmtutil.FormatBytes(res.DiskUsed), fmtutil.FormatBytes(res.DiskTotal))
			lastAlertTime["disk"] = now
		}
	}
}

// OSType represents the details of the host OS.
type OSType struct {
	Family  string `json:"family"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

func newOSType() *OSType {
	return &OSType{
		Family:  "unknown",
		Version: "unknown",
		Arch:    runtime.GOARCH,
	}
}

func detectMacOS() *OSType {
	res := newOSType()
	res.Family = "macos"
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		res.Version = strings.TrimSpace(string(out))
	}
	return res
}

func parseOSRelease(data []byte) *OSType {
	res := newOSType()
	content := strings.ToLower(string(data))
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "id=") {
			res.Family = strings.Trim(strings.TrimPrefix(line, "id="), "\"")
		}
		if strings.HasPrefix(line, "version_id=") {
			res.Version = strings.Trim(strings.TrimPrefix(line, "version_id="), "\"")
		}
	}
	if res.Family == "unknown" {
		res.Family = mapOSFamily(content)
	}
	return res
}

func mapOSFamily(content string) string {
	switch {
	case containsAny(content, "ubuntu", "debian", "pop", "linuxmint", "kali"):
		return "debian"
	case containsAny(content, "centos", "rhel", "fedora", "rocky", "alma", "oracle"):
		return "rhel"
	case strings.Contains(content, "alpine"):
		return "alpine"
	}
	return "unknown"
}

func detectLinuxFallback() *OSType {
	if data, err := os.ReadFile("/etc/debian_version"); err == nil {
		res := newOSType()
		res.Family = "debian"
		res.Version = strings.TrimSpace(string(data))
		return res
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		res := newOSType()
		res.Family = "rhel"
		return res
	}
	return newOSType()
}

// DetectOS detects the operating system details.
func DetectOS() *OSType {
	if runtime.GOOS == "darwin" {
		return detectMacOS()
	}

	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		return parseOSRelease(data)
	}

	return detectLinuxFallback()
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
	if err := exec.Command("systemctl", "enable", "popugate").Run(); err != nil {
		logger.WithScope("system").Warnf("enable service: %v", err)
	}

	return nil
}

// UninstallSystemdService removes the systemd service.
func UninstallSystemdService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd is not supported on this system")
	}
	if err := exec.Command("systemctl", "stop", "popugate").Run(); err != nil {
		logger.WithScope("system").Warnf("stop service: %v", err)
	}
	if err := exec.Command("systemctl", "disable", "popugate").Run(); err != nil {
		logger.WithScope("system").Warnf("disable service: %v", err)
	}
	_ = os.Remove("/etc/systemd/system/popugate.service")
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		logger.WithScope("system").Warnf("daemon-reload: %v", err)
	}
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
