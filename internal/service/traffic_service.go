package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/promutil"
)

var (
	trafficLog = logger.WithScope("traffic")
	quotaLog   = logger.WithScope("quota")
)

// TrafficService handles traffic monitoring and persistence.
type TrafficService struct {
	traffic   *store.TrafficStore
	settings  *store.SettingsStore
	instances *store.InstanceStore
	docker    *dockerutil.DockerClient
	secrets   *store.SecretStore
	quota     *store.QuotaAlertStore

	mu         sync.Mutex
	lastLive   *model.LiveMetrics
	lastTime   time.Time
	client     *http.Client
	dockerAddr string // cached dockerenv detection result
}

// NewTrafficService creates a new TrafficService.
func NewTrafficService(traffic *store.TrafficStore, settings *store.SettingsStore, docker *dockerutil.DockerClient, instances *store.InstanceStore) *TrafficService {
	return &TrafficService{
		traffic:   traffic,
		settings:  settings,
		docker:    docker,
		instances: instances,
		client:    &http.Client{Timeout: 2 * time.Second},
		dockerAddr: func() string {
			if _, err := os.Stat("/.dockerenv"); err == nil {
				return "host.docker.internal"
			}
			return "127.0.0.1"
		}(),
	}
}

// SetSecretStore sets the secret store for quota enforcement.
func (s *TrafficService) SetSecretStore(secrets *store.SecretStore, quota *store.QuotaAlertStore) {
	s.secrets = secrets
	s.quota = quota
}

// GetReport returns cumulative global + per-user traffic.
func (s *TrafficService) GetReport(ctx context.Context) (*model.TrafficReport, error) {
	global, err := s.traffic.GetGlobal(ctx)
	if err != nil {
		return nil, err
	}

	users, err := s.traffic.ListUserTraffic(ctx)
	if err != nil {
		return nil, err
	}

	return &model.TrafficReport{
		Global: model.GlobalTraffic{TotalIn: global.BytesIn, TotalOut: global.BytesOut},
		Users:  users,
	}, nil
}

// GetUserTraffic returns traffic for a specific user.
func (s *TrafficService) GetUserTraffic(ctx context.Context, label string) (*model.UserTraffic, error) {
	return s.traffic.GetUserTraffic(ctx, label)
}

// GetLiveMetrics fetches and caches live Prometheus metrics from all instances.
func (s *TrafficService) GetLiveMetrics(ctx context.Context) (*model.LiveMetrics, error) {
	s.mu.Lock()
	// Return cached if fresh (< 2 seconds)
	if s.lastLive != nil && time.Since(s.lastTime) < 2*time.Second {
		cached := s.lastLive
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	instances, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}

	combined := &model.LiveMetrics{
		UserMetrics: make(map[string]*model.UserLiveMetrics),
	}

	foundAny := false

	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}

		url := fmt.Sprintf("http://%s:%d/metrics", s.dockerAddr, inst.MetricsPort)
		resp, err := s.client.Get(url)
		if err != nil {
			trafficLog.Warnf("failed to fetch metrics from %s: %v", url, err)
			continue
		}

		live, err := promutil.FetchAndParse(resp.Body)
		resp.Body.Close()
		if err != nil {
			trafficLog.Warnf("failed to parse metrics from %s: %v", url, err)
			continue
		}

		foundAny = true
		combined.UptimeSeconds = max(combined.UptimeSeconds, live.UptimeSeconds)
		combined.ConnsCurrent += live.ConnsCurrent
		combined.ConnsTotal += live.ConnsTotal
		combined.ConnsBadTotal += live.ConnsBadTotal
		combined.ConnsMECurrent += live.ConnsMECurrent
		combined.ConnsDirectCurrent += live.ConnsDirectCurrent
		combined.UpstreamAttemptTotal += live.UpstreamAttemptTotal
		combined.UpstreamSuccessTotal += live.UpstreamSuccessTotal
		combined.UpstreamFailTotal += live.UpstreamFailTotal
		combined.MEWritersActive += live.MEWritersActive
		combined.MEWritersWarm += live.MEWritersWarm

		for user, metrics := range live.UserMetrics {
			if _, ok := combined.UserMetrics[user]; !ok {
				combined.UserMetrics[user] = &model.UserLiveMetrics{}
			}
			combined.UserMetrics[user].OctetsFromClient += metrics.OctetsFromClient
			combined.UserMetrics[user].OctetsToClient += metrics.OctetsToClient
			combined.UserMetrics[user].Connections += metrics.Connections
			combined.UserMetrics[user].UniqueIPs += metrics.UniqueIPs
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no metrics collected from any active instance")
	}

	// If the engine doesn't expose telemt_connections_current, compute from per-user metrics
	if combined.ConnsCurrent == 0 && len(combined.UserMetrics) > 0 {
		for _, um := range combined.UserMetrics {
			combined.ConnsCurrent += um.Connections
		}
	}

	s.mu.Lock()
	s.lastLive = combined
	s.lastTime = time.Now()
	s.mu.Unlock()

	return combined, nil
}

// Flush computes deltas from the latest Prometheus snapshot and persists them.
func (s *TrafficService) Flush(ctx context.Context) error {
	live, err := s.GetLiveMetrics(ctx)
	if err != nil {
		return fmt.Errorf("fetch metrics for flush: %w", err)
	}

	// Get previous global snapshot
	globalSnap, err := s.traffic.GetGlobal(ctx)
	if err != nil || globalSnap == nil {
		// First flush or DB error — start from zero
		globalSnap = &model.TrafficSnapshot{}
	}

	// Aggregate global totals from per-user metrics
	var totalFromClient, totalToClient float64
	for _, um := range live.UserMetrics {
		totalFromClient += um.OctetsFromClient
		totalToClient += um.OctetsToClient
	}

	// Compute global deltas from Prometheus counters
	globalDeltaIn := int64(totalFromClient) - globalSnap.SnapIn
	globalDeltaOut := int64(totalToClient) - globalSnap.SnapOut

	// Handle counter reset: if counter reset, treat current reading as delta
	if globalDeltaIn < 0 {
		globalDeltaIn = int64(totalFromClient)
	}
	if globalDeltaOut < 0 {
		globalDeltaOut = int64(totalToClient)
	}

	globalFlush := model.TrafficSnapshot{
		BytesIn:  globalDeltaIn,
		BytesOut: globalDeltaOut,
		SnapIn:   int64(totalFromClient),
		SnapOut:  int64(totalToClient),
	}

	// Per-user deltas — batch fetch all snapshots (M-20)
	userSnapshots, _ := s.traffic.GetAllUserSnapshots(ctx)
	userDeltas := make(map[string]model.TrafficSnapshot)
	for user, um := range live.UserMetrics {
		prev := userSnapshots[user]
		prevSnapIn, prevSnapOut := prev[0], prev[1]

		deltaIn := int64(um.OctetsFromClient) - prevSnapIn
		deltaOut := int64(um.OctetsToClient) - prevSnapOut

		// Handle counter reset
		if deltaIn < 0 {
			deltaIn = int64(um.OctetsFromClient)
		}
		if deltaOut < 0 {
			deltaOut = int64(um.OctetsToClient)
		}

		if deltaIn > 0 || deltaOut > 0 {
			userDeltas[user] = model.TrafficSnapshot{
				BytesIn:  deltaIn,
				BytesOut: deltaOut,
				SnapIn:   int64(um.OctetsFromClient),
				SnapOut:  int64(um.OctetsToClient),
			}
		}
	}

	if err := s.traffic.FlushTraffic(ctx, globalFlush, userDeltas); err != nil {
		return fmt.Errorf("flush traffic: %w", err)
	}

	// Record history (non-critical)
	historyUsers := make(map[string][2]int64, len(userDeltas))
	for label, snap := range userDeltas {
		historyUsers[label] = [2]int64{snap.BytesIn, snap.BytesOut}
	}
	if err := s.traffic.InsertHistoryBatch(ctx, time.Now().Unix(), globalDeltaIn, globalDeltaOut, historyUsers); err != nil {
		trafficLog.Warnf("failed to record traffic history: %v", err)
	}

	trafficLog.Debugf("flush ok: global ↓%d ↑%d, users=%d", globalDeltaIn, globalDeltaOut, len(userDeltas))
	return nil
}

// CheckQuotas auto-disables secrets that exceeded their quota and sends warnings at 80%.
func (s *TrafficService) CheckQuotas(ctx context.Context) {
	if s.secrets == nil || s.quota == nil {
		return
	}

	secrets, err := s.secrets.List(ctx)
	if err != nil {
		return
	}

	for _, sec := range secrets {
		if !sec.Enabled || sec.QuotaBytes <= 0 {
			continue
		}

		userTraffic, err := s.traffic.GetUserTraffic(ctx, sec.Label)
		if err != nil {
			continue
		}

		totalBytes := userTraffic.BytesIn + userTraffic.BytesOut
		if totalBytes <= 0 {
			continue
		}

		pct := int(totalBytes * 100 / sec.QuotaBytes)

		if pct >= 100 {
			alerted, _ := s.quota.WasAlerted(ctx, sec.Label, 100)
			if !alerted {
				enabled, _ := s.secrets.CountEnabled(ctx)
				if enabled > 1 {
					sec.Enabled = false
					if err := s.secrets.Update(ctx, &sec); err != nil {
						quotaLog.Warnf("failed to disable secret %s: %v", sec.Label, err)
					} else {
						quotaLog.Warnf("auto-disabled secret %s (quota exceeded: %d%%)", sec.Label, pct)
					}
				} else {
					quotaLog.Warnf("cannot auto-disable %s (last active secret), quota exceeded %d%%", sec.Label, pct)
				}
				if err := s.quota.MarkAlerted(ctx, sec.Label, 100); err != nil {
					quotaLog.Warnf("mark alert for %s: %v", sec.Label, err)
				}
			}
		} else if pct >= 80 {
			alerted, _ := s.quota.WasAlerted(ctx, sec.Label, 80)
			if !alerted {
				quotaLog.Warnf("warning: secret %s at %d%% of quota", sec.Label, pct)
				if err := s.quota.MarkAlerted(ctx, sec.Label, 80); err != nil {
					quotaLog.Warnf("mark alert for %s: %v", sec.Label, err)
				}
			}
		}
	}
}

// GetHistory returns traffic history records for the given time range and label.
func (s *TrafficService) GetHistory(ctx context.Context, start, end int64, label string, aggregate string) ([]model.TrafficHistoryRecord, error) {
	switch aggregate {
	case "hour":
		return s.traffic.GetAggregatedHistory(ctx, start, end, label, 3600)
	case "day":
		return s.traffic.GetAggregatedHistory(ctx, start, end, label, 86400)
	default:
		return s.traffic.GetHistory(ctx, start, end, label)
	}
}

// CheckExpirations checks for secrets nearing or past expiry.
func (s *TrafficService) CheckExpirations(ctx context.Context) {
	if s.secrets == nil {
		return
	}

	secrets, err := s.secrets.List(ctx)
	if err != nil {
		return
	}

	now := time.Now()
	for _, sec := range secrets {
		if !sec.Enabled || sec.ExpiresAt == "" || sec.ExpiresAt == "0" {
			continue
		}

		expTime, err := time.Parse("2006-01-02", sec.ExpiresAt)
		if err != nil {
			continue
		}

		if now.After(expTime) {
			enabled, _ := s.secrets.CountEnabled(ctx)
			if enabled > 1 {
				sec.Enabled = false
				if err := s.secrets.Update(ctx, &sec); err != nil {
					quotaLog.Warnf("failed to disable expired secret %s: %v", sec.Label, err)
				} else {
					quotaLog.Infof("auto-disabled expired secret %s (expired %s)", sec.Label, sec.ExpiresAt)
				}
			}
		}
	}
}
