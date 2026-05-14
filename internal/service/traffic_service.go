package service

import (
	"context"
	"fmt"
	"net/http"
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
	dockerAddr string
}

// NewTrafficService creates a new TrafficService.
func NewTrafficService(traffic *store.TrafficStore, settings *store.SettingsStore, docker *dockerutil.DockerClient, instances *store.InstanceStore) *TrafficService {
	return &TrafficService{
		traffic:    traffic,
		settings:   settings,
		docker:     docker,
		instances:  instances,
		client:     &http.Client{Timeout: 2 * time.Second},
		dockerAddr: dockerutil.DockerHostAddr(),
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
	instances, err := s.instances.List(ctx)
	if err != nil {
		return err
	}

	globalSnap, _ := s.traffic.GetGlobal(ctx)
	acc := flushAccumulator{
		histUsers: make(map[string][2]int64),
	}

	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		s.flushInstance(ctx, inst, &acc)
	}

	acc.flushGlobal(ctx, *globalSnap, s.traffic)

	trafficLog.Debugf("flush ok: global ↓%d ↑%d", acc.totalIn, acc.totalOut)
	return nil
}

type flushAccumulator struct {
	totalIn    int64
	totalOut   int64
	histUsers  map[string][2]int64
	accSnapIn  int64
	accSnapOut int64
}

func (a *flushAccumulator) addUserDeltas(deltas map[string]model.TrafficSnapshot) {
	for label, snap := range deltas {
		a.totalIn += snap.BytesIn
		a.totalOut += snap.BytesOut
		existing := a.histUsers[label]
		a.histUsers[label] = [2]int64{existing[0] + snap.BytesIn, existing[1] + snap.BytesOut}
	}
}

func (a *flushAccumulator) flushGlobal(ctx context.Context, globalSnap model.TrafficSnapshot, store *store.TrafficStore) {
	if a.totalIn == 0 && a.totalOut == 0 && a.accSnapIn == 0 && a.accSnapOut == 0 {
		return
	}
	globalDeltaIn := nonNegativeDelta(a.accSnapIn, globalSnap.SnapIn)
	globalDeltaOut := nonNegativeDelta(a.accSnapOut, globalSnap.SnapOut)
	if err := store.UpdateGlobal(ctx, globalDeltaIn, globalDeltaOut, a.accSnapIn, a.accSnapOut); err != nil {
		trafficLog.Warnf("flush global: %v", err)
	}
	if a.totalIn > 0 || a.totalOut > 0 {
		if err := store.InsertHistoryBatch(ctx, time.Now().Unix(), a.totalIn, a.totalOut, a.histUsers); err != nil {
			trafficLog.Warnf("failed to record traffic history: %v", err)
		}
	}
}

func nonNegativeDelta(current, previous int64) int64 {
	d := current - previous
	if d < 0 {
		return current
	}
	return d
}

func (s *TrafficService) flushInstance(ctx context.Context, inst model.Instance, acc *flushAccumulator) {
	url := fmt.Sprintf("http://%s:%d/metrics", s.dockerAddr, inst.MetricsPort)
	resp, err := s.client.Get(url)
	if err != nil {
		return
	}
	live, err := promutil.FetchAndParse(resp.Body)
	resp.Body.Close()
	if err != nil {
		return
	}

	userSnapshots, _ := s.traffic.GetAllUserSnapshots(ctx, inst.ID)
	userDeltas := computeUserDeltas(live.UserMetrics, userSnapshots)

	var instTotalFrom, instTotalTo float64
	for _, um := range live.UserMetrics {
		instTotalFrom += um.OctetsFromClient
		instTotalTo += um.OctetsToClient
	}
	acc.accSnapIn += int64(instTotalFrom)
	acc.accSnapOut += int64(instTotalTo)

	if len(userDeltas) == 0 {
		return
	}
	if err := s.traffic.FlushInstanceTraffic(ctx, userDeltas, inst.ID); err != nil {
		trafficLog.Warnf("flush instance %d: %v", inst.ID, err)
		return
	}
	acc.addUserDeltas(userDeltas)
}

func computeUserDeltas(live map[string]*model.UserLiveMetrics, snapshots map[string][2]int64) map[string]model.TrafficSnapshot {
	deltas := make(map[string]model.TrafficSnapshot)
	for user, um := range live {
		prev := snapshots[user]
		deltaIn := nonNegativeDelta(int64(um.OctetsFromClient), prev[0])
		deltaOut := nonNegativeDelta(int64(um.OctetsToClient), prev[1])
		if deltaIn > 0 || deltaOut > 0 {
			deltas[user] = model.TrafficSnapshot{
				BytesIn:  deltaIn,
				BytesOut: deltaOut,
				SnapIn:   int64(um.OctetsFromClient),
				SnapOut:  int64(um.OctetsToClient),
			}
		}
	}
	return deltas
}

func (s *TrafficService) checkQuotaExceeded(ctx context.Context, sec *model.Secret, pct int) {
	alerted, _ := s.quota.WasAlerted(ctx, sec.Label, 100)
	if alerted {
		return
	}
	enabled, _ := s.secrets.CountEnabled(ctx)
	if enabled > 1 {
		sec.Enabled = false
		if err := s.secrets.Update(ctx, sec); err != nil {
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

func (s *TrafficService) checkQuotaWarning(ctx context.Context, label string, pct int) {
	alerted, _ := s.quota.WasAlerted(ctx, label, 80)
	if alerted {
		return
	}
	quotaLog.Warnf("warning: secret %s at %d%% of quota", label, pct)
	if err := s.quota.MarkAlerted(ctx, label, 80); err != nil {
		quotaLog.Warnf("mark alert for %s: %v", label, err)
	}
}

func (s *TrafficService) checkSecretQuota(ctx context.Context, sec *model.Secret) {
	if !sec.Enabled || sec.QuotaBytes <= 0 {
		return
	}

	userTraffic, err := s.traffic.GetUserTraffic(ctx, sec.Label)
	if err != nil {
		return
	}

	totalBytes := userTraffic.BytesIn + userTraffic.BytesOut
	if totalBytes <= 0 {
		return
	}

	pct := int(totalBytes * 100 / sec.QuotaBytes)

	if pct >= 100 {
		s.checkQuotaExceeded(ctx, sec, pct)
	} else if pct >= 80 {
		s.checkQuotaWarning(ctx, sec.Label, pct)
	}
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
		s.checkSecretQuota(ctx, &sec)
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

// ResetAllQuotas resets traffic counters for all users (monthly quota reset).
func (s *TrafficService) ResetAllQuotas(ctx context.Context) {
	if s.secrets == nil {
		return
	}
	if err := s.secrets.ResetAllTraffic(ctx); err != nil {
		quotaLog.Warnf("failed to reset all quotas: %v", err)
	}
}
