package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var healthLog = logger.WithScope("health")

// HealthService provides health diagnostics and auto-recovery.
type HealthService struct {
	docker       *dockerutil.DockerClient
	settings     *store.SettingsStore
	instances    *store.InstanceStore
	containerSvc *ContainerService
}

// NewHealthService creates a new HealthService.
func NewHealthService(docker *dockerutil.DockerClient, settings *store.SettingsStore, instances *store.InstanceStore) *HealthService {
	return &HealthService{
		docker:    docker,
		settings:  settings,
		instances: instances,
	}
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
	Details   string `json:"details,omitempty"`
}

// Check runs all health checks.
func (h *HealthService) Check(ctx context.Context) *HealthStatus {
	status := &HealthStatus{}

	// Docker check
	if h.docker.IsInstalled(ctx) {
		status.Docker = "installed"
	} else {
		status.Docker = "not installed"
		return status
	}

	insts, _ := h.instances.List(ctx)
	if len(insts) == 0 {
		status.Container = "no instances"
		return status
	}

	runningCount := 0
	listeningCount := 0
	metricsCount := 0

	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		r, _ := h.docker.IsInstanceRunning(ctx, inst.ContainerName())
		if r {
			runningCount++
		}
		if h.isPortListening(inst.Port) {
			listeningCount++
		}
		if h.isMetricsResponding(inst.MetricsPort) {
			metricsCount++
		}
	}

	enabledTotal := 0
	for _, inst := range insts {
		if inst.Enabled {
			enabledTotal++
		}
	}

	status.Container = fmt.Sprintf("%d/%d running", runningCount, enabledTotal)
	status.Port = fmt.Sprintf("%d/%d listening", listeningCount, enabledTotal)
	status.Metrics = fmt.Sprintf("%d/%d responding", metricsCount, enabledTotal)

	if runningCount < enabledTotal || listeningCount < enabledTotal || metricsCount < enabledTotal {
		status.Details = "Some instances are down"
	}

	return status
}

// AutoRecover attempts to start the proxy if it's unexpectedly stopped.
func (h *HealthService) AutoRecover(ctx context.Context) error {
	if h.docker == nil {
		return nil
	}

	insts, err := h.instances.List(ctx)
	if err != nil {
		return err
	}

	allRunning := true
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		running, err := h.docker.IsInstanceRunning(ctx, inst.ContainerName())
		if err != nil || !running {
			allRunning = false
			break
		}
	}

	if allRunning {
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

	healthLog.Infof("some instances not running, attempting auto-recovery...")
	if err := h.containerSvc.Start(ctx); err != nil {
		return fmt.Errorf("auto-recovery failed: %w", err)
	}
	healthLog.Infof("auto-recovery successful")
	return nil
}

func (h *HealthService) isPortListening(port int) bool {
	// Try host.docker.internal first if in Docker, fallback to 127.0.0.1
	addr := "127.0.0.1"
	if _, err := os.Stat("/.dockerenv"); err == nil {
		addr = "host.docker.internal"
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (h *HealthService) isMetricsResponding(port int) bool {
	// Try host.docker.internal first if in Docker, fallback to 127.0.0.1
	addr := "127.0.0.1"
	if _, err := os.Stat("/.dockerenv"); err == nil {
		addr = "host.docker.internal"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s:%d/metrics", addr, port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
