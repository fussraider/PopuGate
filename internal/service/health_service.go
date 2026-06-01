package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
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
	client       *http.Client
}

// NewHealthService creates a new HealthService.
func NewHealthService(docker *dockerutil.DockerClient, settings *store.SettingsStore, instances *store.InstanceStore) *HealthService {
	return &HealthService{
		docker:    docker,
		settings:  settings,
		instances: instances,
		client:    &http.Client{Timeout: 2 * time.Second},
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

	var runningContainers map[string]bool
	if h.docker != nil {
		runningContainers, _ = h.docker.ListRunningContainerNames(ctx)
	}
	if runningContainers == nil {
		runningContainers = make(map[string]bool)
	}

	runningCount, listeningCount, metricsCount, enabledTotal := h.checkInstanceCounts(insts, runningContainers)

	status.Container = fmt.Sprintf("%d/%d running", runningCount, enabledTotal)
	status.Port = fmt.Sprintf("%d/%d listening", listeningCount, enabledTotal)
	status.Metrics = fmt.Sprintf("%d/%d responding", metricsCount, enabledTotal)

	if runningCount < enabledTotal || listeningCount < enabledTotal || metricsCount < enabledTotal {
		status.Details = "Some instances are down"
	}

	return status
}

// checkInstanceCounts tallies running, port-listening, and metrics-responding counts across enabled instances.
func (h *HealthService) checkInstanceCounts(insts []model.Instance, runningContainers map[string]bool) (running, listening, metrics, total int) {
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		total++

		activeName := inst.ContainerName()
		if h.containerSvc != nil {
			activeName = h.containerSvc.GetActiveContainerNameWithMap(&inst, runningContainers)
		}

		activePort, activeMetricsPort := inst.Port, inst.MetricsPort
		if h.containerSvc != nil {
			activePort, activeMetricsPort = h.containerSvc.ResolveActivePortsWithMap(&inst, runningContainers)
		} else if activeName != inst.ContainerName() && strings.HasPrefix(activeName, "popugate-telemt-") {
			portStr := strings.TrimPrefix(activeName, "popugate-telemt-")
			if p, err := strconv.Atoi(portStr); err == nil {
				activePort = p
				activeMetricsPort = p + 100
			}
		}

		if runningContainers[activeName] {
			running++
		}
		if h.isPortListening(inst.Port) || h.isPortListening(activePort) {
			listening++
		}
		if h.isMetricsResponding(activeMetricsPort) {
			metrics++
		}
	}
	return
}

// InstanceHealth checks health for a specific instance.
func (h *HealthService) InstanceHealth(ctx context.Context, id int64) (string, error) {
	inst, err := h.instances.GetByID(ctx, id)
	if err != nil {
		return "error", err
	}
	if inst == nil {
		return "not_found", fmt.Errorf("instance %d not found", id)
	}

	activeName := inst.ContainerName()
	if h.containerSvc != nil {
		activeName = h.containerSvc.GetActiveContainerName(ctx, inst)
	}
	running, err := h.docker.IsInstanceRunning(ctx, activeName)
	if err != nil || !running {
		return "stopped", nil
	}

	activeMetricsPort := inst.MetricsPort
	if h.containerSvc != nil {
		_, activeMetricsPort = h.containerSvc.ResolveActivePorts(ctx, inst)
	} else if activeName != inst.ContainerName() && strings.HasPrefix(activeName, "popugate-telemt-") {
		portStr := strings.TrimPrefix(activeName, "popugate-telemt-")
		if p, err := strconv.Atoi(portStr); err == nil {
			activeMetricsPort = p + 100
		}
	}

	if !h.isMetricsResponding(activeMetricsPort) {
		return "unhealthy", nil
	}

	return "healthy", nil
}

// AutoRecover attempts to start instances that are enabled but not running.
func (h *HealthService) AutoRecover(ctx context.Context) error {
	if h.docker == nil {
		return nil
	}

	insts, err := h.instances.List(ctx)
	if err != nil {
		return err
	}

	// Check if intentionally stopped
	if _, err := os.Stat("/tmp/.popugate_stopped"); err == nil {
		return nil
	}

	if h.containerSvc == nil {
		return fmt.Errorf("auto-recovery: container service not available")
	}

	var toRecover []int64
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		activeName := inst.ContainerName()
		if h.containerSvc != nil {
			activeName = h.containerSvc.GetActiveContainerName(ctx, &inst)
		}
		running, err := h.docker.IsInstanceRunning(ctx, activeName)
		if err != nil || !running {
			toRecover = append(toRecover, inst.ID)
		}
	}

	if len(toRecover) == 0 {
		return nil
	}

	recovered := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range toRecover {
		wg.Add(1)
		go func(instID int64) {
			defer func() {
				if r := recover(); r != nil {
					healthLog.Warnf("goroutine panic (recovery instance %d): %v", instID, r)
				}
			}()
			defer wg.Done()
			healthLog.Infof("attempting recovery for instance %d...", instID)
			if err := h.containerSvc.StartInstance(ctx, instID); err != nil {
				healthLog.Warnf("recovery failed for instance %d: %v", instID, err)
			} else {
				mu.Lock()
				recovered++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()

	if recovered > 0 {
		healthLog.Infof("auto-recovery: %d instance(s) recovered", recovered)
	}

	return nil
}

func (h *HealthService) isPortListening(port int) bool {
	addr := dockerutil.DockerHostAddr()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (h *HealthService) isMetricsResponding(port int) bool {
	addr := dockerutil.DockerHostAddr()
	url := fmt.Sprintf("http://%s:%d/metrics", addr, port)
	resp, err := h.client.Get(url)
	if err != nil {
		healthLog.Debugf("metrics check %s: %v", url, err)
		return false
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		healthLog.Debugf("metrics check %s: status %d", url, resp.StatusCode)
	}
	return resp.StatusCode == 200
}
