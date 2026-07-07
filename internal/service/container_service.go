package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/netutil"
	"github.com/fussraider/PopuGate/pkg/promutil"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

const stopFlagFile = ".popugate_stopped"

var statusLog = logger.WithScope("status")

// ErrNoMatchingSecrets is returned when an instance has no matching secrets.
var ErrNoMatchingSecrets = errors.New("no matching secrets: add a secret with a matching tag or remove instance tags")

// ContainerService manages proxy container lifecycle.
type ContainerService struct {
	dataDir        string
	docker         *dockerutil.DockerClient
	secrets        *store.SecretStore
	upstreams      *store.UpstreamStore
	instances      *store.InstanceStore
	traffic        *store.TrafficStore
	settings       *store.SettingsStore
	trafficSvc     *TrafficService
	iptables       *netutil.IptablesManager
	notify         atomic.Value
	notifyWithBtns atomic.Value
	client         *http.Client
	frontClient    *http.Client
	mu             sync.RWMutex
	subscribers    map[chan *model.ProxyStatus]struct{}
}

// NewContainerService creates a new ContainerService.
func NewContainerService(
	dataDir string,
	docker *dockerutil.DockerClient,
	secrets *store.SecretStore,
	upstreams *store.UpstreamStore,
	instances *store.InstanceStore,
	traffic *store.TrafficStore,
	settings *store.SettingsStore,
	trafficSvc *TrafficService,
) *ContainerService {
	return &ContainerService{
		dataDir:     dataDir,
		docker:      docker,
		secrets:     secrets,
		upstreams:   upstreams,
		instances:   instances,
		traffic:     traffic,
		settings:    settings,
		trafficSvc:  trafficSvc,
		iptables:    netutil.NewIptablesManager(),
		client:      &http.Client{Timeout: 2 * time.Second},
		frontClient: &http.Client{Timeout: 15 * time.Second},
		subscribers: make(map[chan *model.ProxyStatus]struct{}),
	}
}

// Subscribe returns a channel that receives ProxyStatus updates.
func (s *ContainerService) Subscribe() (chan *model.ProxyStatus, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan *model.ProxyStatus, 1)
	s.subscribers[ch] = struct{}{}

	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subscribers, ch)
		close(ch)
	}
}

func (s *ContainerService) broadcast(status *model.ProxyStatus) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- status:
		default:
			// Buffer full, skip this subscriber
		}
	}
}

func (s *ContainerService) notifyStatusChange() {
	status, err := s.Status(context.Background())
	if err == nil {
		s.broadcast(status)
	}
}

// SetNotify sets the notification callback.
func (s *ContainerService) SetNotify(fn NotifyFunc) { s.notify.Store(fn) }

func (s *ContainerService) SetNotifyWithButtons(fn NotifyWithButtonsFunc) { s.notifyWithBtns.Store(fn) }

func (s *ContainerService) getNotify() NotifyFunc {
	if v := s.notify.Load(); v != nil {
		return v.(NotifyFunc)
	}
	return nil
}

func (s *ContainerService) getNotifyWithBtns() NotifyWithButtonsFunc {
	if v := s.notifyWithBtns.Load(); v != nil {
		return v.(NotifyWithButtonsFunc)
	}
	return nil
}

// Start starts all enabled proxy instances.
func (s *ContainerService) Start(ctx context.Context) error {
	_ = os.Remove(filepath.Join(s.dataDir, stopFlagFile))

	settings, err := s.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	enabledCount, err := s.secrets.CountEnabled(ctx)
	if err != nil {
		return err
	}
	if enabledCount == 0 {
		return fmt.Errorf("no enabled secrets; add at least one secret first")
	}

	if err := s.startInstances(ctx, settings); err != nil {
		return fmt.Errorf("start instances: %w", err)
	}

	s.notifyEngineState(ctx, "🟢 *%s* Proxy engine started")
	return nil
}

// Stop stops all proxy instances.
func (s *ContainerService) Stop(ctx context.Context) error {
	if err := s.flushTraffic(ctx); err != nil {
		statusLog.Warnf("traffic flush before stop: %v", err)
	}

	if err := os.WriteFile(filepath.Join(s.dataDir, stopFlagFile), fmt.Appendf(nil, "%d", time.Now().Unix()), 0644); err != nil {
		statusLog.Warnf("write stop flag: %v", err)
	}

	insts, err := s.instances.List(ctx)
	if err != nil {
		statusLog.Warnf("list instances for stop: %v", err)
	}
	var wg sync.WaitGroup
	for _, inst := range insts {
		if inst.Enabled {
			wg.Add(1)
			go func(inst model.Instance) {
				defer func() {
					if r := recover(); r != nil {
						statusLog.Warnf("goroutine panic (stop instance %s): %v", inst.ContainerName(), r)
					}
				}()
				defer wg.Done()
				stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				activeSwingPort, swingRunning := s.GetActiveSwingPort(stopCtx, &inst)
				activeName := s.GetActiveContainerName(stopCtx, &inst)
				if err := s.docker.StopInstance(stopCtx, activeName, 10); err != nil {
					statusLog.Warnf("stop instance %s: %v", activeName, err)
				}
				if swingRunning {
					_ = s.iptables.RemovePortRedirect(inst.Port, activeSwingPort)
				} else {
					_ = s.iptables.RemovePortRedirect(inst.Port, inst.Port+10000)
				}
				s.cleanupInstanceRuntimeRules(&inst)
			}(inst)
		}
	}
	wg.Wait()

	s.notifyEngineState(ctx, "🔴 *%s* Proxy engine stopped")
	return nil
}

// StopInstance stops a specific instance by ID.
func (s *ContainerService) StopInstance(ctx context.Context, id int64) error {
	if s.docker == nil {
		return fmt.Errorf("docker client is not initialized")
	}
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	activeSwingPort, swingRunning := s.GetActiveSwingPort(stopCtx, inst)
	activeName := s.GetActiveContainerName(stopCtx, inst)
	err = s.docker.StopInstance(stopCtx, activeName, 10)
	if swingRunning {
		_ = s.iptables.RemovePortRedirect(inst.Port, activeSwingPort)
	} else {
		_ = s.iptables.RemovePortRedirect(inst.Port, inst.Port+10000)
	}
	s.cleanupInstanceRuntimeRules(inst)
	return err
}

// RestartInstance stops and starts a specific instance by ID.
func (s *ContainerService) RestartInstance(ctx context.Context, id int64) error {
	if err := s.StopInstance(ctx, id); err != nil {
		statusLog.Warnf("failed to stop instance %d for restart: %v", id, err)
	}
	time.Sleep(1 * time.Second)
	return s.StartInstance(ctx, id)
}

// ReloadInstanceConfig regenerates config for a specific instance and sends SIGHUP for hot-reload.
func (s *ContainerService) ReloadInstanceConfig(ctx context.Context, id int64) error {
	if s.docker == nil {
		return fmt.Errorf("docker client is not initialized")
	}
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}
	if !inst.Enabled {
		return fmt.Errorf("instance %d is disabled", id)
	}
	if err := s.generateInstanceConfig(ctx, settings, inst); err != nil {
		return fmt.Errorf("generate config for instance %d: %w", inst.ID, err)
	}
	activeName := s.GetActiveContainerName(ctx, inst)
	running, _ := s.docker.IsInstanceRunning(ctx, activeName)
	if !running {
		return fmt.Errorf("instance %d container is not running", id)
	}
	return s.docker.KillSignalInstance(ctx, activeName, "SIGHUP")
}

// StartInstance starts a specific instance by ID.
func (s *ContainerService) StartInstance(ctx context.Context, id int64) error {
	if s.docker == nil {
		return fmt.Errorf("docker client is not initialized")
	}
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}

	count, err := s.matchingSecretCount(ctx, inst.GetTags())
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNoMatchingSecrets
	}

	s.cleanStalePortRedirects(ctx, inst.Port)

	tlsFrontDir, err := s.applyInstanceRuntimeRules(ctx, inst)
	if err != nil {
		return err
	}

	if err := s.generateInstanceConfig(ctx, settings, inst); err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	_, err = s.docker.RunInstance(ctx, dockerutil.InstanceRunOptions{
		RunOptions: dockerutil.RunOptions{
			Image:      model.DockerImageBase + ":latest",
			ConfigPath: inst.ConfigPath(),
			CPUs:       settings.ProxyCPUs,
			Memory:     settings.ProxyMemory,
		},
		Name:        inst.ContainerName(),
		Port:        inst.Port,
		TLSFrontDir: tlsFrontDir,
	})
	return err
}

// Restart stops and starts the proxy.
func (s *ContainerService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	return s.Start(ctx)
}

// GetActiveSwingPort finds if a swing container is currently running for the instance, and if so, what port it uses.
func (s *ContainerService) GetActiveSwingPort(ctx context.Context, inst *model.Instance) (int, bool) {
	if s.docker == nil {
		return inst.Port + 10000, false
	}
	running, err := s.docker.ListRunningContainerNames(ctx)
	if err != nil {
		statusLog.Warnf("failed to list running containers: %v", err)
		return inst.Port + 10000, false
	}
	return s.GetActiveSwingPortWithMap(inst, running)
}

// GetActiveSwingPortWithMap finds if a swing container is currently running using a pre-fetched running containers map.
func (s *ContainerService) GetActiveSwingPortWithMap(inst *model.Instance, runningContainers map[string]bool) (int, bool) {
	startPort := inst.Port + 10000
	for i := 0; i < 100; i++ {
		port := startPort + i
		tempName := fmt.Sprintf("popugate-telemt-%d", port)
		if runningContainers[tempName] {
			return port, true
		}
	}
	return startPort, false
}

// ResolveActivePorts determines the active port and metrics port for a given instance.
func (s *ContainerService) ResolveActivePorts(ctx context.Context, inst *model.Instance) (int, int) {
	if s.docker == nil {
		return inst.Port, inst.MetricsPort
	}
	running, err := s.docker.ListRunningContainerNames(ctx)
	if err != nil {
		return inst.Port, inst.MetricsPort
	}
	return s.ResolveActivePortsWithMap(inst, running)
}

// ResolveActivePortsWithMap determines active ports using a pre-fetched running containers map.
func (s *ContainerService) ResolveActivePortsWithMap(inst *model.Instance, runningContainers map[string]bool) (int, int) {
	activePort := inst.Port
	activeMetricsPort := inst.MetricsPort
	if swingPort, running := s.GetActiveSwingPortWithMap(inst, runningContainers); running {
		activePort = swingPort
		activeMetricsPort = swingPort + 100
	}
	return activePort, activeMetricsPort
}

// ListRunningContainerNames returns a set of names of all running Docker containers.
func (s *ContainerService) ListRunningContainerNames(ctx context.Context) (map[string]bool, error) {
	if s.docker == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}
	return s.docker.ListRunningContainerNames(ctx)
}

func (s *ContainerService) cleanStalePortRedirects(ctx context.Context, primaryPort int) {
	startPort := primaryPort + 10000
	for i := 0; i < 100; i++ {
		port := startPort + i
		redirected, err := s.iptables.HasPortRedirect(primaryPort, port)
		if err == nil && redirected {
			statusLog.Infof("Removing stale port redirect for port %d -> %d", primaryPort, port)
			_ = s.iptables.RemovePortRedirect(primaryPort, port)
		}
	}
}

func isPortFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

func (s *ContainerService) findFreeSwingPort(ctx context.Context, primaryPort int) (int, int) {
	startPort := primaryPort + 10000
	for i := 0; i < 100; i++ {
		port := startPort + i
		metricsPort := port + 100 // keep metrics port separated from proxy port

		// Check if port is free on host
		if !isPortFree(port) || !isPortFree(metricsPort) {
			continue
		}

		// Check if any Docker container with this name is already running
		tempName := fmt.Sprintf("popugate-telemt-%d", port)
		running, _ := s.docker.IsInstanceRunning(ctx, tempName)
		if running {
			continue
		}

		return port, metricsPort
	}
	return startPort, primaryPort + 10100 // fallback
}

type swingRoutingState struct {
	targetPort            int
	targetMetricsPort     int
	targetContainerName   string
	drainingPort          int
	drainingMetricsPort   int
	drainingContainerName string
	drainingRunning       bool
	redirectActive        bool
	tempPort              int
}

func (s *ContainerService) resolveSwingRoutingState(ctx context.Context, inst *model.Instance, runningContainers map[string]bool) (*swingRoutingState, error) {
	primaryPort := inst.Port
	primaryMetricsPort := inst.MetricsPort
	primaryName := inst.ContainerName()

	activeSwingPort, swingRunning := s.GetActiveSwingPortWithMap(inst, runningContainers)

	var tempPort, tempMetricsPort int
	if swingRunning {
		tempPort = activeSwingPort
		tempMetricsPort = activeSwingPort + 100
	} else {
		tempPort, tempMetricsPort = s.findFreeSwingPort(ctx, primaryPort)
	}

	tempName := fmt.Sprintf("popugate-telemt-%d", tempPort)

	redirectActive, err := s.iptables.HasPortRedirect(primaryPort, tempPort)
	if err != nil {
		statusLog.Warnf("Failed to check iptables redirect for port %d: %v", primaryPort, err)
		redirectActive = false
	}
	if !redirectActive {
		primaryRunning := runningContainers[primaryName]
		tempRunning := runningContainers[tempName]
		if tempRunning && !primaryRunning {
			redirectActive = true
		}
	}

	state := &swingRoutingState{
		tempPort:       tempPort,
		redirectActive: redirectActive,
	}

	if redirectActive {
		state.targetPort = primaryPort
		state.targetMetricsPort = primaryMetricsPort
		state.targetContainerName = primaryName

		state.drainingPort = tempPort
		state.drainingMetricsPort = tempMetricsPort
		state.drainingContainerName = tempName
	} else {
		state.targetPort = tempPort
		state.targetMetricsPort = tempMetricsPort
		state.targetContainerName = tempName

		state.drainingPort = primaryPort
		state.drainingMetricsPort = primaryMetricsPort
		state.drainingContainerName = primaryName
	}

	state.drainingRunning = runningContainers[state.drainingContainerName]
	if !state.drainingRunning {
		state.targetPort = primaryPort
		state.targetMetricsPort = primaryMetricsPort
		state.targetContainerName = primaryName
		_ = s.iptables.RemovePortRedirect(primaryPort, tempPort)
	}

	return state, nil
}

func (s *ContainerService) setupTargetRuntimeRules(ctx context.Context, settings *model.Settings, inst *model.Instance, targetPort, targetMetricsPort int) (string, error) {
	if inst.TCPMSSEnabled {
		if err := s.iptables.SetTCPMSSRule(targetPort, inst.TCPMSS); err != nil {
			statusLog.Warnf("Failed to set tcpmss rule for target port %d: %v", targetPort, err)
		}
	}

	if err := s.generateInstanceConfigForSwing(ctx, settings, inst, targetPort, targetMetricsPort); err != nil {
		return "", fmt.Errorf("generate swing config: %w", err)
	}

	var tlsFrontDir string
	if inst.TLSFronting && inst.FakeTLS {
		if err := s.downloadFrontingContent(ctx, inst.TLSDomain, inst.TLSFrontDirPath()); err != nil {
			statusLog.Warnf("download fronting content: %v", err)
		} else {
			tlsFrontDir = inst.TLSFrontDirPath()
		}
	}

	return tlsFrontDir, nil
}

func (s *ContainerService) startSwingDraining(inst *model.Instance, state *swingRoutingState) {
	if !state.drainingRunning {
		return
	}

	primaryPort := inst.Port
	if state.redirectActive {
		if err := s.iptables.RemovePortRedirect(primaryPort, state.tempPort); err != nil {
			statusLog.Warnf("Failed to remove port redirect: %v", err)
		}
	} else {
		if err := s.iptables.AddPortRedirect(primaryPort, state.tempPort); err != nil {
			statusLog.Warnf("Failed to add port redirect: %v", err)
		}
	}

	go func(name string, port, metricsPort int) {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		timeout := time.After(3 * time.Minute)

		for {
			select {
			case <-timeout:
				statusLog.Infof("Draining timeout reached for container %s, stopping now", name)
				s.stopAndCleanContainer(name, port)
				return
			case <-ticker.C:
				conns, err := s.getActiveConnections(context.Background(), metricsPort)
				if err != nil {
					statusLog.Warnf("Failed to query metrics for draining container %s: %v", name, err)
					s.stopAndCleanContainer(name, port)
					return
				}
				statusLog.Infof("Draining container %s: %d active connections remaining", name, conns)
				if conns <= 0 {
					statusLog.Infof("Draining complete for container %s, stopping now", name)
					s.stopAndCleanContainer(name, port)
					return
				}
			}
		}
	}(state.drainingContainerName, state.drainingPort, state.drainingMetricsPort)
}

// ReloadInstance performs a Zero-Downtime Swing Routing update of the instance container.
func (s *ContainerService) ReloadInstance(ctx context.Context, id int64) error {
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}

	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}

	runningContainers, err := s.docker.ListRunningContainerNames(ctx)
	if err != nil {
		runningContainers = make(map[string]bool)
	}

	state, err := s.resolveSwingRoutingState(ctx, inst, runningContainers)
	if err != nil {
		return err
	}

	tlsFrontDir, err := s.setupTargetRuntimeRules(ctx, settings, inst, state.targetPort, state.targetMetricsPort)
	if err != nil {
		return err
	}

	_, err = s.docker.RunInstance(ctx, dockerutil.InstanceRunOptions{
		RunOptions: dockerutil.RunOptions{
			Image:      model.DockerImageBase + ":latest",
			ConfigPath: filepath.Join(model.InstallDir, fmt.Sprintf("mtproxy/config-%d.toml", state.targetPort)),
			CPUs:       settings.ProxyCPUs,
			Memory:     settings.ProxyMemory,
		},
		Name:        state.targetContainerName,
		Port:        state.targetPort,
		TLSFrontDir: tlsFrontDir,
	})
	if err != nil {
		return fmt.Errorf("start swing container: %w", err)
	}

	time.Sleep(2 * time.Second)

	s.startSwingDraining(inst, state)

	return nil
}

// GetActiveContainerName returns the name of the currently running container for the instance,
// checking both the primary container and the temporary swing container.
func (s *ContainerService) GetActiveContainerName(ctx context.Context, inst *model.Instance) string {
	if s.docker == nil {
		return inst.ContainerName()
	}
	running, err := s.docker.ListRunningContainerNames(ctx)
	if err != nil {
		return inst.ContainerName()
	}
	return s.GetActiveContainerNameWithMap(inst, running)
}

// GetActiveContainerNameWithMap returns the name of the currently running container using a pre-fetched running containers map.
func (s *ContainerService) GetActiveContainerNameWithMap(inst *model.Instance, runningContainers map[string]bool) string {
	if swingPort, running := s.GetActiveSwingPortWithMap(inst, runningContainers); running {
		return fmt.Sprintf("popugate-telemt-%d", swingPort)
	}
	return inst.ContainerName()
}

func (s *ContainerService) getActiveConnections(ctx context.Context, metricsPort int) (int, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	live, err := promutil.FetchAndParse(resp.Body)
	if err != nil {
		return 0, err
	}

	return int(live.ConnsCurrent), nil
}

func (s *ContainerService) stopAndCleanContainer(name string, port int) {
	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.docker.StopInstance(stopCtx, name, 10); err != nil {
		statusLog.Warnf("failed to stop draining container %s: %v", name, err)
	}
	if err := s.iptables.RemoveTCPMSSRules(port); err != nil {
		statusLog.Warnf("failed to remove tcpmss rule for port %d: %v", port, err)
	}
	configPath := filepath.Join(model.InstallDir, fmt.Sprintf("mtproxy/config-%d.toml", port))
	_ = os.Remove(configPath)
	statusLog.Infof("Stopped and cleaned up draining container %s", name)
	s.notifyStatusChange()
}

func (s *ContainerService) generateInstanceConfigForSwing(ctx context.Context, settings *model.Settings, inst *model.Instance, port, metricsPort int) error {
	upstreamEntries, err := s.listUpstreamEntries(ctx)
	if err != nil {
		return err
	}

	instCopy := *inst
	instCopy.Port = port
	instCopy.MetricsPort = metricsPort

	return s.generateInstanceConfigWith(ctx, settings, &instCopy, upstreamEntries)
}

// Reload regenerates config and sends SIGHUP for hot-reload.
func (s *ContainerService) Reload(ctx context.Context, reason string) error {
	if s.docker == nil {
		return fmt.Errorf("docker client is not initialized")
	}
	statusLog.Infof("triggering instance reload: reason=%q", reason)
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}

	insts, err := s.instances.List(ctx)
	if err != nil {
		return err
	}

	upstreamEntries, err := s.listUpstreamEntries(ctx)
	if err != nil {
		return err
	}

	dbSecrets, err := s.secrets.List(ctx)
	if err != nil {
		return err
	}

	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		statusLog.Infof("regenerating config for instance %d (%s) due to: %s", inst.ID, inst.Label, reason)
		if err := s.generateInstanceConfigWithCached(ctx, settings, &inst, upstreamEntries, dbSecrets); err != nil {
			statusLog.Warnf("generate config for instance %d: %v", inst.ID, err)
			continue
		}
		running, err := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
		if err != nil {
			statusLog.Warnf("check running state for instance %s: %v", inst.ContainerName(), err)
			continue
		}
		if !running {
			statusLog.Infof("instance %d (%s) is not running, skipping SIGHUP", inst.ID, inst.ContainerName())
			continue
		}
		statusLog.Infof("sending SIGHUP (hot-reload) to instance %d (%s)", inst.ID, inst.ContainerName())
		if err := s.docker.KillSignalInstance(ctx, inst.ContainerName(), "SIGHUP"); err != nil {
			statusLog.Warnf("SIGHUP instance %s: %v", inst.ContainerName(), err)
		}
	}

	return nil
}

// ReloadZeroDowntime reloads all enabled proxy instances using the Zero-Downtime Swing Routing mechanism.
func (s *ContainerService) ReloadZeroDowntime(ctx context.Context) error {
	insts, err := s.instances.List(ctx)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var errs []error
	var errsMu sync.Mutex

	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		wg.Add(1)
		go func(inst model.Instance) {
			defer func() {
				if r := recover(); r != nil {
					statusLog.Warnf("goroutine panic (swing reload instance %d): %v", inst.ID, r)
				}
				wg.Done()
			}()
			if err := s.ReloadInstance(ctx, inst.ID); err != nil {
				errsMu.Lock()
				errs = append(errs, fmt.Errorf("instance %d: %w", inst.ID, err))
				errsMu.Unlock()
			}
		}(inst)
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("zero-downtime reload failed for some instances: %v", errs)
	}
	return nil
}

func (s *ContainerService) buildSecretCounts(ctx context.Context, insts []model.Instance) (map[int64]int, error) {
	dbSecrets, err := s.secrets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	counts := make(map[int64]int, len(insts))
	for _, inst := range insts {
		instanceTags := inst.GetTags()
		count := 0
		for _, sec := range dbSecrets {
			if sec.Enabled && model.TagsMatch(instanceTags, sec.GetTags()) {
				count++
			}
		}
		counts[inst.ID] = count
	}
	return counts, nil
}

func buildInstanceStatus(inst model.Instance, running bool, containerName string, activePort, activeMetricsPort int, draining bool, matchingSecrets int) model.InstanceStatus {
	is := model.InstanceStatus{
		ID:                  inst.ID,
		Port:                inst.Port,
		Running:             running,
		Label:               inst.Label,
		TLSDomain:           inst.TLSDomain,
		FakeTLS:             inst.FakeTLS,
		ContainerName:       containerName,
		ActivePort:          activePort,
		ActiveMetricsPort:   activeMetricsPort,
		Draining:            draining,
		MatchingSecretCount: matchingSecrets,
	}
	if running {
		is.Status = "healthy"
	} else if inst.Enabled {
		is.Status = "stopped"
	}
	return is
}

func (s *ContainerService) enrichWithRuntimeInfo(ctx context.Context, status *model.ProxyStatus, inst *model.Instance) {
	activeName := s.GetActiveContainerName(ctx, inst)
	info, err := s.docker.ContainerInspect(ctx, activeName)
	if err == nil {
		status.ContainerID = info.ID[:12]
		if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
			status.StartedAt = t
			status.UptimeSeconds = int64(time.Since(t).Seconds())
			status.Uptime = formatDuration(time.Since(t))
		}
	}

	metrics, err := s.trafficSvc.GetLiveMetrics(ctx)
	if err != nil {
		statusLog.Warnf("live metrics unavailable: %v", err)
	} else {
		status.ConnsCurrent = int(metrics.ConnsCurrent)
		status.ConnsTotal = int64(metrics.ConnsTotal)
	}
}

// Status returns the current status of all proxy instances.
func (s *ContainerService) Status(ctx context.Context) (*model.ProxyStatus, error) {
	status := &model.ProxyStatus{
		Running: false,
	}

	insts, _ := s.instances.List(ctx)
	if len(insts) == 0 {
		return status, nil
	}

	secretCounts, _ := s.buildSecretCounts(ctx, insts)
	if secretCounts == nil {
		secretCounts = make(map[int64]int)
	}

	var runningContainers map[string]bool
	if s.docker != nil {
		runningContainers, _ = s.docker.ListRunningContainerNames(ctx)
	}
	if runningContainers == nil {
		runningContainers = make(map[string]bool)
	}

	var firstRunningInst *model.Instance
	for i, inst := range insts {
		activeName := s.GetActiveContainerNameWithMap(&inst, runningContainers)
		running := runningContainers[activeName]

		activePort, activeMetricsPort := s.ResolveActivePortsWithMap(&inst, runningContainers)

		primaryRunning := runningContainers[inst.ContainerName()]
		_, tempRunning := s.GetActiveSwingPortWithMap(&inst, runningContainers)
		draining := primaryRunning && tempRunning

		status.Instances = append(status.Instances, buildInstanceStatus(inst, running, activeName, activePort, activeMetricsPort, draining, secretCounts[inst.ID]))
		if running {
			status.Running = true
			if firstRunningInst == nil {
				firstRunningInst = &insts[i]
			}
		}
	}

	if firstRunningInst != nil {
		s.enrichWithRuntimeInfo(ctx, status, firstRunningInst)
	}

	if global, err := s.traffic.GetGlobal(ctx); err == nil && global != nil {
		status.TrafficIn = global.BytesIn
		status.TrafficOut = global.BytesOut
	}

	return status, nil
}

// InstanceStatus returns detailed status for a specific instance.
func (s *ContainerService) InstanceStatus(ctx context.Context, id int64) (string, error) {
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if inst == nil {
		return "", fmt.Errorf("instance %d not found", id)
	}

	activeName := s.GetActiveContainerName(ctx, inst)
	running, err := s.docker.IsInstanceRunning(ctx, activeName)
	if err != nil {
		return "error", nil
	}
	if !running {
		return "stopped", nil
	}

	_, metricsPort := s.ResolveActivePorts(ctx, inst)

	// Check metrics endpoint
	addr := dockerutil.DockerHostAddr()
	resp, err := s.client.Get(fmt.Sprintf("http://%s:%d/metrics", addr, metricsPort))
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return "unhealthy", nil
	}
	_ = resp.Body.Close()

	return "healthy", nil
}

// matchingSecretCount returns the number of enabled secrets that match the given instance tags.
func (s *ContainerService) matchingSecretCount(ctx context.Context, instanceTags []string) (int, error) {
	dbSecrets, err := s.secrets.List(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, sec := range dbSecrets {
		if !sec.Enabled {
			continue
		}
		if model.TagsMatch(instanceTags, sec.GetTags()) {
			count++
		}
	}
	return count, nil
}

// RevalidateAllInstances checks all running instances and stops those without matching secrets.
func (s *ContainerService) RevalidateAllInstances(ctx context.Context) {
	insts, err := s.instances.List(ctx)
	if err != nil {
		statusLog.Warnf("revalidate: list instances: %v", err)
		return
	}
	secretCounts, err := s.buildSecretCounts(ctx, insts)
	if err != nil {
		statusLog.Warnf("revalidate: build secret counts: %v", err)
		return
	}
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		activeSwingPort, swingRunning := s.GetActiveSwingPort(ctx, &inst)
		activeName := s.GetActiveContainerName(ctx, &inst)
		running, _ := s.docker.IsInstanceRunning(ctx, activeName)
		if !running {
			continue
		}
		if secretCounts[inst.ID] == 0 {
			statusLog.Infof("stopping instance %d (%s): no matching secrets after secret change", inst.Port, inst.Label)
			stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := s.docker.StopInstance(stopCtx, activeName, 10); err != nil {
				statusLog.Warnf("revalidate: stop instance %s: %v", activeName, err)
			}
			if swingRunning {
				_ = s.iptables.RemovePortRedirect(inst.Port, activeSwingPort)
			} else {
				_ = s.iptables.RemovePortRedirect(inst.Port, inst.Port+10000)
			}
			cancel()
			if fn := s.getNotifyWithBtns(); fn != nil {
				fn(ctx, "⚠️ *%s* Instance \"%s\" (port %d) stopped: no matching secrets",
					[]KeyboardButton{dashboardButton(ctx, s.settings)}, inst.Label, inst.Port)
			} else if fn := s.getNotify(); fn != nil {
				fn(ctx, "⚠️ *%s* Instance \"%s\" (port %d) stopped: no matching secrets", inst.Label, inst.Port)
			}
		}
	}
}

// RevalidateInstance checks a specific running instance and stops it if no matching secrets.
func (s *ContainerService) RevalidateInstance(ctx context.Context, id int64) {
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil || inst == nil {
		return
	}
	activeSwingPort, swingRunning := s.GetActiveSwingPort(ctx, inst)
	activeName := s.GetActiveContainerName(ctx, inst)
	running, _ := s.docker.IsInstanceRunning(ctx, activeName)
	if !running {
		return
	}
	counts, err := s.buildSecretCounts(ctx, []model.Instance{*inst})
	if err != nil {
		statusLog.Warnf("revalidate instance %d: %v", id, err)
		return
	}
	if counts[inst.ID] == 0 {
		statusLog.Infof("stopping instance %d (%s): no matching secrets after instance change", inst.Port, inst.Label)
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := s.docker.StopInstance(stopCtx, activeName, 10); err != nil {
			statusLog.Warnf("revalidate: stop instance %s: %v", activeName, err)
		}
		if swingRunning {
			_ = s.iptables.RemovePortRedirect(inst.Port, activeSwingPort)
		} else {
			_ = s.iptables.RemovePortRedirect(inst.Port, inst.Port+10000)
		}
		cancel()
		if fn := s.getNotifyWithBtns(); fn != nil {
			fn(ctx, "⚠️ *%s* Instance stopped: no matching secrets",
				[]KeyboardButton{dashboardButton(ctx, s.settings)})
		} else if fn := s.getNotify(); fn != nil {
			fn(ctx, "⚠️ *%s* Instance stopped: no matching secrets")
		}
	}
}

func (s *ContainerService) listUpstreamEntries(ctx context.Context) ([]telemt.UpstreamEntry, error) {
	dbUpstreams, err := s.upstreams.List(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]telemt.UpstreamEntry, 0, len(dbUpstreams))
	for _, u := range dbUpstreams {
		entries = append(entries, telemt.UpstreamEntry{
			Type:     model.UpstreamType(u.Type),
			Address:  u.Address,
			Username: u.Username,
			Password: u.Password,
			URL:      u.URL,
			Weight:   u.Weight,
			Iface:    u.Iface,
			Enabled:  u.Enabled,
		})
	}
	return entries, nil
}

func (s *ContainerService) generateInstanceConfig(ctx context.Context, settings *model.Settings, inst *model.Instance) error {
	upstreamEntries, err := s.listUpstreamEntries(ctx)
	if err != nil {
		return err
	}
	return s.generateInstanceConfigWith(ctx, settings, inst, upstreamEntries)
}

func (s *ContainerService) generateInstanceConfigWith(ctx context.Context, settings *model.Settings, inst *model.Instance, upstreamEntries []telemt.UpstreamEntry) error {
	dbSecrets, err := s.secrets.List(ctx)
	if err != nil {
		return err
	}
	return s.generateInstanceConfigWithCached(ctx, settings, inst, upstreamEntries, dbSecrets)
}

func (s *ContainerService) generateInstanceConfigWithCached(ctx context.Context, settings *model.Settings, inst *model.Instance, upstreamEntries []telemt.UpstreamEntry, dbSecrets []model.Secret) error {
	instanceTags := inst.GetTags()
	var secretEntries []telemt.SecretEntry
	for _, sec := range dbSecrets {
		secretTags := sec.GetTags()
		if !model.TagsMatch(instanceTags, secretTags) {
			continue
		}
		// Disabled secrets are still emitted (with Enabled=false) so the engine
		// marks them [access.user_enabled]=false and cancels their active
		// sessions on hot-reload, rather than silently dropping them from config.
		secretEntries = append(secretEntries, telemt.SecretEntry{
			Label:            sec.Label,
			SecretKey:        sec.SecretKey,
			Enabled:          sec.Enabled,
			MaxConns:         sec.MaxConns,
			MaxIPs:           sec.MaxIPs,
			QuotaBytes:       sec.QuotaBytes,
			RateLimitUpBps:   sec.RateLimitUpBps,
			RateLimitDownBps: sec.RateLimitDownBps,
			ExpiresAt:        sec.ExpiresAt,
		})
	}

	// Telegram config
	var telegramCfg telemt.TelegramConfig
	if settings.ProxySecretURL != "" || settings.ProxyConfigV4URL != "" || settings.ProxyConfigV6URL != "" {
		telegramCfg = telemt.TelegramConfig{
			ProxySecretURL:   settings.ProxySecretURL,
			ProxyConfigV4URL: settings.ProxyConfigV4URL,
			ProxyConfigV6URL: settings.ProxyConfigV6URL,
		}
	}

	// Build per-instance config
	cfg := telemt.BuildInstanceConfig(&telemt.InstanceConfigParams{
		Instance:              inst,
		FakeCertLen:           settings.FakeCertLen,
		UseMiddleProxy:        settings.UseMiddleProxy,
		AdTag:                 settings.AdTag,
		ProxyProtocol:         settings.ProxyProtocol,
		ProxyProtocolCIDRs:    parseCIDRListSafe(settings.ProxyProtocolTrustedCIDRs),
		Telegram:              telegramCfg,
		Secrets:               secretEntries,
		Upstreams:             upstreamEntries,
		ExtraMetricsWhitelist: dockerExtraMetricsIPs(),
	})

	return telemt.WriteConfigTOML(cfg, inst.ConfigPath())
}

func (s *ContainerService) startInstances(ctx context.Context, settings *model.Settings) error {
	insts, err := s.instances.List(ctx)
	if err != nil {
		return err
	}

	secretCounts, err := s.buildSecretCounts(ctx, insts)
	if err != nil {
		return fmt.Errorf("count matching secrets: %w", err)
	}
	upstreamEntries, err := s.listUpstreamEntries(ctx)
	if err != nil {
		return fmt.Errorf("list upstreams: %w", err)
	}

	frontingOK, err := s.prepareInstanceConfigs(ctx, insts, secretCounts, upstreamEntries, settings)
	if err != nil {
		return err
	}

	return s.startContainersParallel(ctx, insts, secretCounts, frontingOK, settings)
}

func (s *ContainerService) prepareInstanceConfigs(ctx context.Context, insts []model.Instance, secretCounts map[int64]int, upstreamEntries []telemt.UpstreamEntry, settings *model.Settings) (map[int]bool, error) {
	frontingOK := make(map[int]bool)
	for i := range insts {
		inst := &insts[i]
		if !inst.Enabled || secretCounts[inst.ID] == 0 {
			continue
		}
		if inst.TLSFronting && inst.FakeTLS {
			if err := s.downloadFrontingContent(ctx, inst.TLSDomain, inst.TLSFrontDirPath()); err != nil {
				statusLog.Warnf("download fronting content for port %d: %v", inst.Port, err)
			} else {
				frontingOK[inst.Port] = true
			}
		}
		if err := s.generateInstanceConfigWith(ctx, settings, inst, upstreamEntries); err != nil {
			return nil, fmt.Errorf("generate config for instance %d: %w", inst.Port, err)
		}
	}
	return frontingOK, nil
}

func (s *ContainerService) startContainersParallel(ctx context.Context, insts []model.Instance, secretCounts map[int64]int, frontingOK map[int]bool, settings *model.Settings) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for _, inst := range insts {
		if !inst.Enabled || secretCounts[inst.ID] == 0 {
			continue
		}
		wg.Add(1)
		go func(inst model.Instance) {
			defer func() {
				if r := recover(); r != nil {
					statusLog.Warnf("goroutine panic (start instance %d): %v", inst.Port, r)
				}
			}()
			defer wg.Done()
			s.cleanStalePortRedirects(ctx, inst.Port)

			if err := s.applyTCPMSSRules(&inst); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}

			var tlsFrontDir string
			if inst.TLSFronting && inst.FakeTLS && frontingOK[inst.Port] {
				tlsFrontDir = inst.TLSFrontDirPath()
			}

			_, err := s.docker.RunInstance(ctx, dockerutil.InstanceRunOptions{
				RunOptions: dockerutil.RunOptions{
					Image:      model.DockerImageBase + ":latest",
					ConfigPath: inst.ConfigPath(),
					CPUs:       settings.ProxyCPUs,
					Memory:     settings.ProxyMemory,
				},
				Name:        inst.ContainerName(),
				Port:        inst.Port,
				TLSFrontDir: tlsFrontDir,
			})
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("start instance %d: %w", inst.Port, err))
				mu.Unlock()
			}
		}(inst)
	}
	wg.Wait()

	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("%d instance(s) failed: %s", len(errs), strings.Join(msgs, "; "))
	}
	return nil
}

func (s *ContainerService) flushTraffic(ctx context.Context) error {
	if s.trafficSvc != nil {
		return s.trafficSvc.Flush(ctx)
	}
	return nil
}

func (s *ContainerService) notifyEngineState(ctx context.Context, format string) {
	if fn := s.getNotify(); fn == nil {
		return
	} else {
		fn(ctx, format)
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func parseCIDRListSafe(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// dockerExtraMetricsIPs returns additional source IPs to whitelist for the
// telemt metrics endpoint when the backend is running inside a Docker container.
func dockerExtraMetricsIPs() []string {
	if !dockerutil.IsDockerEnv() {
		return nil
	}

	seen := make(map[string]bool)
	var ips []string

	if addrs, err := net.LookupHost("host.docker.internal"); err == nil {
		for _, addr := range addrs {
			if !seen[addr] {
				seen[addr] = true
				ips = append(ips, addr)
			}
		}
	}

	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				ipStr := ipNet.IP.String()
				if !seen[ipStr] {
					seen[ipStr] = true
					ips = append(ips, ipStr)
				}
			}
		}
	}

	return ips
}

func (s *ContainerService) applyInstanceRuntimeRules(ctx context.Context, inst *model.Instance) (string, error) {
	if err := s.applyTCPMSSRules(inst); err != nil {
		return "", fmt.Errorf("apply runtime rules: %w", err)
	}

	var tlsFrontDir string
	if inst.TLSFronting && inst.FakeTLS {
		dir := inst.TLSFrontDirPath()
		if err := s.downloadFrontingContent(ctx, inst.TLSDomain, dir); err != nil {
			statusLog.Warnf("download fronting content for port %d: %v", inst.Port, err)
		} else {
			tlsFrontDir = dir
		}
	}
	return tlsFrontDir, nil
}

func (s *ContainerService) cleanupInstanceRuntimeRules(inst *model.Instance) {
	if inst.TCPMSSEnabled {
		if err := s.iptables.RemoveTCPMSSRules(inst.Port); err != nil {
			statusLog.Warnf("cleanup tcpmss rules for port %d: %v", inst.Port, err)
		}
	}
}

func (s *ContainerService) applyTCPMSSRules(inst *model.Instance) error {
	if !inst.TCPMSSEnabled {
		return nil
	}
	if err := s.iptables.RemoveTCPMSSRules(inst.Port); err != nil {
		statusLog.Warnf("remove old tcpmss rules for port %d: %v", inst.Port, err)
	}
	if err := s.iptables.SetTCPMSSRule(inst.Port, inst.TCPMSS); err != nil {
		return fmt.Errorf("tcpmss port %d mss %d: %w", inst.Port, inst.TCPMSS, err)
	}
	return nil
}

// ReconcileInstanceRules ensures iptables rules match the instance's desired state.
func (s *ContainerService) ReconcileInstanceRules(ctx context.Context, id int64) error {
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get instance %d: %w", id, err)
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}
	running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
	if !running {
		if err := s.iptables.RemoveTCPMSSRules(inst.Port); err != nil {
			return fmt.Errorf("remove tcpmss rules port %d: %w", inst.Port, err)
		}
		return nil
	}
	if inst.TCPMSSEnabled {
		return s.applyTCPMSSRules(inst)
	}
	if err := s.iptables.RemoveTCPMSSRules(inst.Port); err != nil {
		return fmt.Errorf("remove tcpmss rules port %d: %w", inst.Port, err)
	}
	return nil
}

func (s *ContainerService) downloadFrontingContent(ctx context.Context, domain, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create fronting dir: %w", err)
	}

	frontURL := fmt.Sprintf("https://%s/", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, frontURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := s.frontClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", frontURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("fetch %s: HTTP %d", frontURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return err
	}

	tmpPath := filepath.Join(dir, "index.html.tmp")
	if err := os.WriteFile(tmpPath, body, 0644); err != nil {
		return fmt.Errorf("write temp fronting: %w", err)
	}
	return os.Rename(tmpPath, filepath.Join(dir, "index.html"))
}

// RefreshFrontingContent re-downloads TLS fronting content for a specific instance.
func (s *ContainerService) RefreshFrontingContent(ctx context.Context, id int64) error {
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}
	if !inst.TLSFronting || !inst.FakeTLS {
		return fmt.Errorf("TLS fronting is not enabled for instance %d", id)
	}
	return s.downloadFrontingContent(ctx, inst.TLSDomain, inst.TLSFrontDirPath())
}

// EnsureDefaultInstance seeds the instances table with a default instance if empty.
func (s *ContainerService) EnsureDefaultInstance(ctx context.Context, proxyPort, metricsPort int, proxyDomain, maskingHost string, maskingEnabled bool) error {
	count, err := s.instances.Count(ctx)
	if err != nil {
		return fmt.Errorf("count instances: %w", err)
	}
	if count > 0 {
		return nil
	}

	fakeTLS := maskingEnabled
	inst := &model.Instance{
		Port:        proxyPort,
		MetricsPort: metricsPort,
		Enabled:     true,
		Label:       "Default",
		TLSDomain:   proxyDomain,
		FakeTLS:     fakeTLS,
		MaskHost:    maskingHost,
		MaskPort:    443,
		TCPMSS:      88,
	}
	return s.instances.Create(ctx, inst)
}

// RefreshAllFrontingContent refreshes fronting HTML cache for all enabled instances that have TLSFronting turned on.
func (s *ContainerService) RefreshAllFrontingContent(ctx context.Context) error {
	insts, err := s.instances.List(ctx)
	if err != nil {
		return err
	}
	var lastErr error
	for _, inst := range insts {
		if inst.Enabled && inst.TLSFronting && inst.FakeTLS {
			if err := s.downloadFrontingContent(ctx, inst.TLSDomain, inst.TLSFrontDirPath()); err != nil {
				statusLog.Errorf("failed to auto-refresh fronting for instance on port %d: %v", inst.Port, err)
				lastErr = err
			} else {
				statusLog.Infof("successfully refreshed fronting content for instance on port %d", inst.Port)
			}
		}
	}
	return lastErr
}
