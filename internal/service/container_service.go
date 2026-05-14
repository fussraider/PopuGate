package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
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

// ErrNoMatchingSecrets is returned when an instance has no matching secrets.
var ErrNoMatchingSecrets = errors.New("no matching secrets: add a secret with a matching tag or remove instance tags")

// ContainerService manages proxy container lifecycle.
type ContainerService struct {
	docker         *dockerutil.DockerClient
	secrets        *store.SecretStore
	upstreams      *store.UpstreamStore
	instances      *store.InstanceStore
	traffic        *store.TrafficStore
	settings       *store.SettingsStore
	trafficSvc     *TrafficService
	notify         NotifyFunc
	notifyWithBtns NotifyWithButtonsFunc
	client         *http.Client
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
		client:     &http.Client{Timeout: 2 * time.Second},
	}
}

// SetNotify sets the notification callback.
func (s *ContainerService) SetNotify(fn NotifyFunc) { s.notify = fn }

func (s *ContainerService) SetNotifyWithButtons(fn NotifyWithButtonsFunc) { s.notifyWithBtns = fn }

// Start starts all enabled proxy instances.
func (s *ContainerService) Start(ctx context.Context) error {
	os.Remove(stopFlagPath)

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

	if err := os.WriteFile(stopFlagPath, fmt.Appendf(nil, "%d", time.Now().Unix()), 0644); err != nil {
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

// StopInstance stops a specific instance by ID.
func (s *ContainerService) StopInstance(ctx context.Context, id int64) error {
	inst, err := s.instances.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inst == nil {
		return fmt.Errorf("instance %d not found", id)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.docker.StopInstance(stopCtx, inst.ContainerName(), 10)
}

// StartInstance starts a specific instance by ID.
func (s *ContainerService) StartInstance(ctx context.Context, id int64) error {
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
		Name: inst.ContainerName(),
		Port: inst.Port,
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

// ReloadInstance regenerates config and sends SIGHUP for a specific instance.
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

	if err := s.generateInstanceConfig(ctx, settings, inst); err != nil {
		return err
	}

	return s.docker.KillSignalInstance(ctx, inst.ContainerName(), "SIGHUP")
}

// Reload regenerates config and sends SIGHUP for hot-reload.
func (s *ContainerService) Reload(ctx context.Context) error {
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}

	insts, err := s.instances.List(ctx)
	if err != nil {
		return err
	}
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		if err := s.generateInstanceConfig(ctx, settings, &inst); err != nil {
			statusLog.Warnf("generate config for instance %d: %v", inst.ID, err)
			continue
		}
		running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
		if !running {
			continue
		}
		if err := s.docker.KillSignalInstance(ctx, inst.ContainerName(), "SIGHUP"); err != nil {
			statusLog.Warnf("SIGHUP instance %s: %v", inst.ContainerName(), err)
		}
	}

	return nil
}

func (s *ContainerService) buildSecretCounts(ctx context.Context, insts []model.Instance) map[int64]int {
	dbSecrets, _ := s.secrets.List(ctx)
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
	return counts
}

func buildInstanceStatus(inst model.Instance, running bool, matchingSecrets int) model.InstanceStatus {
	is := model.InstanceStatus{
		ID:                  inst.ID,
		Port:                inst.Port,
		Running:             running,
		Label:               inst.Label,
		TLSDomain:           inst.TLSDomain,
		FakeTLS:             inst.FakeTLS,
		ContainerName:       inst.ContainerName(),
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
	info, err := s.docker.ContainerInspect(ctx, inst.ContainerName())
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

	secretCounts := s.buildSecretCounts(ctx, insts)

	var firstRunningInst *model.Instance
	for i, inst := range insts {
		running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
		status.Instances = append(status.Instances, buildInstanceStatus(inst, running, secretCounts[inst.ID]))
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

	running, err := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
	if err != nil {
		return "error", nil
	}
	if !running {
		return "stopped", nil
	}

	// Check metrics endpoint
	addr := dockerutil.DockerHostAddr()
	resp, err := s.client.Get(fmt.Sprintf("http://%s:%d/metrics", addr, inst.MetricsPort))
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return "unhealthy", nil
	}
	resp.Body.Close()

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
	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
		if !running {
			continue
		}
		count, err := s.matchingSecretCount(ctx, inst.GetTags())
		if err != nil {
			statusLog.Warnf("revalidate: check instance %d: %v", inst.ID, err)
			continue
		}
		if count == 0 {
			statusLog.Infof("stopping instance %d (%s): no matching secrets after secret change", inst.Port, inst.Label)
			stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := s.docker.StopInstance(stopCtx, inst.ContainerName(), 10); err != nil {
				statusLog.Warnf("revalidate: stop instance %s: %v", inst.ContainerName(), err)
			}
			cancel()
			if s.notifyWithBtns != nil {
				s.notifyWithBtns(ctx, "⚠️ *%s* Instance \"%s\" (port %d) stopped: no matching secrets",
					[]KeyboardButton{dashboardButton(ctx, s.settings)}, inst.Label, inst.Port)
			} else if s.notify != nil {
				s.notify(ctx, "⚠️ *%s* Instance \"%s\" (port %d) stopped: no matching secrets", inst.Label, inst.Port)
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
	running, _ := s.docker.IsInstanceRunning(ctx, inst.ContainerName())
	if !running {
		return
	}
	count, err := s.matchingSecretCount(ctx, inst.GetTags())
	if err != nil {
		statusLog.Warnf("revalidate instance %d: %v", id, err)
		return
	}
	if count == 0 {
		statusLog.Infof("stopping instance %d (%s): no matching secrets after instance change", inst.Port, inst.Label)
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := s.docker.StopInstance(stopCtx, inst.ContainerName(), 10); err != nil {
			statusLog.Warnf("revalidate: stop instance %s: %v", inst.ContainerName(), err)
		}
		cancel()
		if s.notifyWithBtns != nil {
			s.notifyWithBtns(ctx, "⚠️ *%s* Instance stopped: no matching secrets",
				[]KeyboardButton{dashboardButton(ctx, s.settings)})
		} else if s.notify != nil {
			s.notify(ctx, "⚠️ *%s* Instance stopped: no matching secrets")
		}
	}
}

func (s *ContainerService) generateInstanceConfig(ctx context.Context, settings *model.Settings, inst *model.Instance) error {
	// Gather secrets, filtering by tag match
	dbSecrets, err := s.secrets.List(ctx)
	if err != nil {
		return err
	}
	instanceTags := inst.GetTags()
	var secretEntries []telemt.SecretEntry
	for _, sec := range dbSecrets {
		if !sec.Enabled {
			continue
		}
		secretTags := sec.GetTags()
		if !model.TagsMatch(instanceTags, secretTags) {
			continue
		}
		secretEntries = append(secretEntries, telemt.SecretEntry{
			Label:      sec.Label,
			SecretKey:  sec.SecretKey,
			Enabled:    true,
			MaxConns:   sec.MaxConns,
			MaxIPs:     sec.MaxIPs,
			QuotaBytes: sec.QuotaBytes,
			ExpiresAt:  sec.ExpiresAt,
		})
	}

	// Gather upstreams (global)
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

	// Generate configs sequentially, skipping instances without matching secrets
	for i := range insts {
		inst := &insts[i]
		if !inst.Enabled {
			continue
		}
		count, err := s.matchingSecretCount(ctx, inst.GetTags())
		if err != nil {
			return fmt.Errorf("check secrets for instance %d: %w", inst.Port, err)
		}
		if count == 0 {
			statusLog.Warnf("skipping instance %d (%s): no matching secrets", inst.Port, inst.Label)
			continue
		}
		if err := s.generateInstanceConfig(ctx, settings, inst); err != nil {
			return fmt.Errorf("generate config for instance %d: %w", inst.Port, err)
		}
	}

	// Start containers in parallel
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for _, inst := range insts {
		if !inst.Enabled {
			continue
		}
		count, _ := s.matchingSecretCount(ctx, inst.GetTags())
		if count == 0 {
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
