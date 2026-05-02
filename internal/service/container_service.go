package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

const stopFlagPath = "/tmp/.popugate_stopped"

var statusLog = logger.WithScope("status")

// ContainerService manages proxy container lifecycle.
type ContainerService struct {
	docker     *dockerutil.DockerClient
	secrets    *store.SecretStore
	upstreams  *store.UpstreamStore
	instances  *store.InstanceStore
	traffic    *store.TrafficStore
	settings   *store.SettingsStore
	trafficSvc *TrafficService
	notify     NotifyFunc
}

// NewContainerService creates a new ContainerService.
func NewContainerService(
	docker *dockerutil.DockerClient,
	secrets *store.SecretStore,
	upstreams *store.UpstreamStore,
	instances *store.InstanceStore,
	traffic *store.TrafficStore,
	settings *store.SettingsStore,
	trafficSvc *TrafficService,
) *ContainerService {
	return &ContainerService{
		docker:     docker,
		secrets:    secrets,
		upstreams:  upstreams,
		instances:  instances,
		traffic:    traffic,
		settings:   settings,
		trafficSvc: trafficSvc,
	}
}

// SetNotify sets the notification callback.
func (s *ContainerService) SetNotify(fn NotifyFunc) { s.notify = fn }

// Start starts all enabled proxy instances.
func (s *ContainerService) Start(ctx context.Context) error {
	// Remove stop flag — allow auto-recovery again
	os.Remove(stopFlagPath)

	settings, err := s.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	// Ensure at least one enabled secret exists
	enabledCount, err := s.secrets.CountEnabled(ctx)
	if err != nil {
		return err
	}
	if enabledCount == 0 {
		return fmt.Errorf("no enabled secrets; add at least one secret first")
	}

	// Start instances
	if err := s.startInstances(ctx, settings); err != nil {
		return fmt.Errorf("start instances: %w", err)
	}

	s.notifyEngineState(ctx, "🟢 *%s* Proxy engine started")
	return nil
}

// Stop stops all proxy instances.
func (s *ContainerService) Stop(ctx context.Context) error {
	// Flush traffic before stopping
	if err := s.flushTraffic(ctx); err != nil {
		statusLog.Warnf("traffic flush before stop: %v", err)
	}

	// Write stop flag to prevent auto-recovery from restarting
	if err := os.WriteFile(stopFlagPath, fmt.Appendf(nil, "%d", time.Now().Unix()), 0644); err != nil {
		statusLog.Warnf("write stop flag: %v", err)
	}

	// Stop instances in parallel, but wait for all to complete
	insts, err := s.instances.List(ctx)
	if err != nil {
		statusLog.Warnf("list instances for stop: %v", err)
	}
	var wg sync.WaitGroup
	for _, inst := range insts {
		if inst.Enabled {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if err := s.docker.StopInstance(stopCtx, name, 10); err != nil {
					statusLog.Warnf("stop instance %s: %v", name, err)
				}
			}(inst.ContainerName())
		}
	}
	wg.Wait()

	s.notifyEngineState(ctx, "🔴 *%s* Proxy engine stopped")
	return nil
}

// Restart stops and starts the proxy.
func (s *ContainerService) Restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	return s.Start(ctx)
}

// Reload regenerates config and sends SIGHUP for hot-reload.
func (s *ContainerService) Reload(ctx context.Context) error {
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}

	// Regenerate primary config
	if err := s.generateConfig(ctx, settings, model.ConfigDir()+"/config.toml"); err != nil {
		return err
	}

	// SIGHUP the primary container
	if err := s.docker.KillSignal(ctx, "SIGHUP"); err != nil {
		return err
	}

	// Regenerate and SIGHUP all running instances
	insts, err := s.instances.List(ctx)
	if err != nil {
		return nil // best effort for instances
	}
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
		if !running {
			continue
		}
		instanceSettings := *settings
		instanceSettings.ProxyPort = inst.Port
		instanceSettings.ProxyMetricsPort = inst.MetricsPort
		if err := s.generateConfig(ctx, &instanceSettings, inst.ConfigPath()); err != nil {
			continue
		}
		if err := s.docker.KillSignalInstance(ctx, inst.ContainerName(), "SIGHUP"); err != nil {
			statusLog.Warnf("SIGHUP instance %s: %v", inst.ContainerName(), err)
		}
	}

	return nil
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

	// Check if any instance is running to set global Running status
	var firstRunningInst *model.Instance
	for i, inst := range insts {
		running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
		status.Instances = append(status.Instances, model.InstanceStatus{
			Port:    inst.Port,
			Running: running,
			Label:   inst.Label,
		})
		if running {
			status.Running = true
			if firstRunningInst == nil {
				firstRunningInst = &insts[i]
			}
		}
	}

	// If at least one instance is running, get more details from the first one
	if firstRunningInst != nil {
		info, err := s.docker.ContainerInspect(ctx, firstRunningInst.ContainerName())
		if err == nil {
			status.ContainerID = info.ID[:12]
			if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
				status.StartedAt = t
				status.UptimeSeconds = int64(time.Since(t).Seconds())
				status.Uptime = formatDuration(time.Since(t))
			}
		}

		// Add metrics if available
		metrics, err := s.trafficSvc.GetLiveMetrics(ctx)
		if err != nil {
			statusLog.Warnf("live metrics unavailable: %v", err)
		} else {
			status.ConnsCurrent = int(metrics.ConnsCurrent)
			status.ConnsTotal = int64(metrics.ConnsTotal)
		}
	}

	// Global traffic (all-time)
	if global, err := s.traffic.GetGlobal(ctx); err == nil && global != nil {
		status.TrafficIn = global.BytesIn
		status.TrafficOut = global.BytesOut
	}

	return status, nil
}

func (s *ContainerService) generateConfig(ctx context.Context, settings *model.Settings, configPath string) error {
	// Gather secrets
	dbSecrets, err := s.secrets.List(ctx)
	if err != nil {
		return err
	}
	var secretEntries []telemt.SecretEntry
	for _, sec := range dbSecrets {
		secretEntries = append(secretEntries, telemt.SecretEntry{
			Label:      sec.Label,
			SecretKey:  sec.SecretKey,
			Enabled:    sec.Enabled,
			MaxConns:   sec.MaxConns,
			MaxIPs:     sec.MaxIPs,
			QuotaBytes: sec.QuotaBytes,
			ExpiresAt:  sec.ExpiresAt,
		})
	}

	// Gather upstreams
	dbUpstreams, err := s.upstreams.List(ctx)
	if err != nil {
		return err
	}
	var upstreamEntries []telemt.UpstreamEntry
	for _, u := range dbUpstreams {
		upstreamEntries = append(upstreamEntries, telemt.UpstreamEntry{
			Type:     model.UpstreamType(u.Type),
			Address:  u.Address,
			Username: u.Username,
			Password: u.Password,
			Weight:   u.Weight,
			Iface:    u.Iface,
			Enabled:  u.Enabled,
		})
	}

	// Build and write config
	cfg := telemt.BuildConfig(&telemt.ConfigParams{
		Settings:              settings,
		Secrets:               secretEntries,
		Upstreams:             upstreamEntries,
		ExtraMetricsWhitelist: dockerExtraMetricsIPs(),
	})

	return telemt.WriteConfigTOML(cfg, configPath)
}

func (s *ContainerService) startInstances(ctx context.Context, settings *model.Settings) error {
	insts, err := s.instances.List(ctx)
	if err != nil {
		return err
	}

	// Generate configs sequentially first (safe: writes to different files)
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		instanceSettings := *settings
		instanceSettings.ProxyPort = inst.Port
		instanceSettings.ProxyMetricsPort = inst.MetricsPort

		if err := s.generateConfig(ctx, &instanceSettings, inst.ConfigPath()); err != nil {
			return fmt.Errorf("generate config for instance %d: %w", inst.Port, err)
		}
	}

	// Start containers in parallel
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		first error
	)

	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}

		wg.Add(1)
		go func(inst model.Instance) {
			defer wg.Done()

			_, err := s.docker.RunInstance(ctx, dockerutil.InstanceRunOptions{
				RunOptions: dockerutil.RunOptions{
					Image:      model.DockerImageBase + ":latest",
					ConfigPath: inst.ConfigPath(),
					CPUs:       settings.ProxyCPUs,
					Memory:     settings.ProxyMemory,
				},
				Name: inst.ContainerName(),
				Port: inst.Port,
			})
			if err != nil {
				mu.Lock()
				if first == nil {
					first = fmt.Errorf("start instance %d: %w", inst.Port, err)
				}
				mu.Unlock()
			}
		}(inst)
	}
	wg.Wait()

	return first
}

func (s *ContainerService) flushTraffic(ctx context.Context) error {
	if s.trafficSvc != nil {
		return s.trafficSvc.Flush(ctx)
	}
	return nil
}

func (s *ContainerService) notifyEngineState(ctx context.Context, format string) {
	if s.notify == nil {
		return
	}
	s.notify(ctx, format)
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

// dockerExtraMetricsIPs returns additional source IPs to whitelist for the
// telemt metrics endpoint when the backend is running inside a Docker container.
// Telemt runs with --network host, so it sees the container's bridge IP as the
// connection source — which must be explicitly allowed in metrics_whitelist.
func dockerExtraMetricsIPs() []string {
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var ips []string

	// Resolve host.docker.internal (the host gateway IP the backend connects to)
	if addrs, err := net.LookupHost("host.docker.internal"); err == nil {
		for _, addr := range addrs {
			if !seen[addr] {
				seen[addr] = true
				ips = append(ips, addr)
			}
		}
	}

	// Get container's own non-loopback IPs (the source IPs telemt will see)
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
