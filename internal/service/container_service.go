package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

const stopFlagPath = "/tmp/.popugate_stopped"

// ContainerService manages proxy container lifecycle.
type ContainerService struct {
	docker     *dockerutil.DockerClient
	secrets    *store.SecretStore
	upstreams  *store.UpstreamStore
	instances  *store.InstanceStore
	traffic    *store.TrafficStore
	settings   *store.SettingsStore
	trafficSvc *TrafficService
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

	return nil
}

// Stop stops all proxy instances.
func (s *ContainerService) Stop(ctx context.Context) error {
	// Flush traffic before stopping
	_ = s.flushTraffic(ctx)

	// Write stop flag to prevent auto-recovery from restarting
	_ = os.WriteFile(stopFlagPath, []byte(fmt.Sprintf("%d", time.Now().Unix())), 0644)

	// Stop instances in parallel
	insts, _ := s.instances.List(ctx)
	for _, inst := range insts {
		if inst.Enabled {
			go func(name string) {
				// Use a background context with a timeout to not block the main Stop flow
				stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_ = s.docker.StopInstance(stopCtx, name, 10)
			}(inst.ContainerName())
		}
	}

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
		_ = s.docker.KillSignalInstance(ctx, inst.ContainerName(), "SIGHUP")
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
		if err == nil {
			status.ConnsCurrent = int(metrics.ConnsCurrent)
			status.ConnsTotal = int64(metrics.ConnsTotal)
		}
	}

	// Global traffic (all-time)
	global, _ := s.traffic.GetGlobal(ctx)
	status.TrafficIn = global.BytesIn
	status.TrafficOut = global.BytesOut

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

	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}

		// Generate per-instance config
		instanceSettings := *settings
		instanceSettings.ProxyPort = inst.Port
		instanceSettings.ProxyMetricsPort = inst.MetricsPort

		if err := s.generateConfig(ctx, &instanceSettings, inst.ConfigPath()); err != nil {
			return fmt.Errorf("generate config for instance %d: %w", inst.Port, err)
		}

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
			return fmt.Errorf("start instance %d: %w", inst.Port, err)
		}
	}

	return nil
}

func (s *ContainerService) flushTraffic(ctx context.Context) error {
	if s.trafficSvc != nil {
		return s.trafficSvc.Flush(ctx)
	}
	return nil
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
