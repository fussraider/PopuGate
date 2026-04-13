package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
)

// HealthService provides health diagnostics and auto-recovery.
type HealthService struct {
	docker       *dockerutil.DockerClient
	settings     *store.SettingsStore
	containerSvc *ContainerService
}

// NewHealthService creates a new HealthService.
func NewHealthService(docker *dockerutil.DockerClient, settings *store.SettingsStore) *HealthService {
	return &HealthService{docker: docker, settings: settings}
}

// SetContainerSvc sets the container service for auto-recovery.
func (h *HealthService) SetContainerSvc(svc *ContainerService) {
	h.containerSvc = svc
}

// HealthStatus holds the result of a health check.
type HealthStatus struct {
	Docker    string `json:"docker"`
	Container string `json:"container"`
	Port      string `json:"port"`
	Metrics   string `json:"metrics"`
}

// Check runs all health checks.
func (h *HealthService) Check(ctx context.Context) *HealthStatus {
	status := &HealthStatus{}
	settings, _ := h.settings.Load(ctx)

	// Docker check
	if h.docker.IsInstalled(ctx) {
		status.Docker = "installed"
	} else {
		status.Docker = "not installed"
		return status
	}

	// Container check
	running, err := h.docker.IsRunning(ctx)
	if err != nil {
		status.Container = fmt.Sprintf("error: %v", err)
	} else if running {
		status.Container = "running"
	} else {
		status.Container = "stopped"
	}

	// Port check
	if h.isPortListening(settings.ProxyPort) {
		status.Port = "listening"
	} else {
		status.Port = "not listening"
	}

	// Metrics check
	if h.isMetricsResponding(settings.ProxyMetricsPort) {
		status.Metrics = "responding"
	} else {
		status.Metrics = "not responding"
	}

	return status
}

// AutoRecover attempts to start the proxy if it's unexpectedly stopped.
func (h *HealthService) AutoRecover(ctx context.Context) error {
	if h.docker == nil {
		return nil
	}

	running, err := h.docker.IsRunning(ctx)
	if err != nil {
		return err
	}
	if running {
		return nil // already running
	}

	// Check if intentionally stopped (flag set by ContainerService.Stop)
	if _, err := os.Stat("/tmp/.popugate_stopped"); err == nil {
		return nil // intentionally stopped
	}

	// Attempt recovery
	if h.containerSvc == nil {
		return fmt.Errorf("auto-recovery: container service not available")
	}

	log.Printf("[health] proxy not running, attempting auto-recovery...")
	if err := h.containerSvc.Start(ctx); err != nil {
		return fmt.Errorf("auto-recovery failed: %w", err)
	}
	log.Printf("[health] auto-recovery successful")
	return nil
}

func (h *HealthService) isPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (h *HealthService) isMetricsResponding(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
