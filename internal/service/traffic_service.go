package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/promutil"
)

// TrafficService handles traffic monitoring and persistence.
type TrafficService struct {
	traffic  *store.TrafficStore
	settings *store.SettingsStore
	docker   *dockerutil.DockerClient
	secrets  *store.SecretStore
	quota    *store.QuotaAlertStore

	mu       sync.Mutex
	lastLive *model.LiveMetrics
	lastTime time.Time
}

// NewTrafficService creates a new TrafficService.
func NewTrafficService(traffic *store.TrafficStore, settings *store.SettingsStore, docker *dockerutil.DockerClient) *TrafficService {
	return &TrafficService{
		traffic:  traffic,
		settings: settings,
		docker:   docker,
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

// GetLiveMetrics fetches and caches live Prometheus metrics.
func (s *TrafficService) GetLiveMetrics(ctx context.Context) (*model.LiveMetrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Return cached if fresh (< 2 seconds)
	if s.lastLive != nil && time.Since(s.lastTime) < 2*time.Second {
		return s.lastLive, nil
	}

	settings, err := s.settings.Load(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", settings.ProxyMetricsPort)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	live, err := promutil.FetchAndParse(resp.Body)
	if err != nil {
		return nil, err
	}

	s.lastLive = live
	s.lastTime = time.Now()
	return live, nil
}

// Flush computes deltas from the latest Prometheus snapshot and persists them.
// Handles negative deltas (container restart resets counters) by treating
// the current reading as absolute and adding it to the cumulative total.
func (s *TrafficService) Flush(ctx context.Context) error {
	// Skip flush if container is not running
	if s.docker != nil {
		running, _ := s.docker.IsRunning(ctx)
		if !running {
			return nil
		}
	}

	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", settings.ProxyMetricsPort)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetch metrics for flush: %w", err)
	}
	defer resp.Body.Close()

	live, err := promutil.FetchAndParse(resp.Body)
	if err != nil {
		return err
	}

	// Get previous global snapshot
	globalSnap, _ := s.traffic.GetGlobal(ctx)

	// Aggregate global totals from per-user metrics
	var totalFromClient, totalToClient float64
	for _, um := range live.UserMetrics {
		totalFromClient += um.OctetsFromClient
		totalToClient += um.OctetsToClient
	}

	// Compute global deltas from Prometheus counters
	globalDeltaIn := int64(totalFromClient) - globalSnap.SnapIn
	globalDeltaOut := int64(totalToClient) - globalSnap.SnapOut

	// Handle container restart: if counter reset, treat current reading as delta
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

	// Per-user deltas
	userDeltas := make(map[string]model.TrafficSnapshot)
	for user, um := range live.UserMetrics {
		prevSnapIn, prevSnapOut, _ := s.traffic.GetUserSnapshot(ctx, user)

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
					_ = s.secrets.Update(ctx, &sec)
					log.Printf("[quota] auto-disabled secret %s (quota exceeded: %d%%)", sec.Label, pct)
				} else {
					log.Printf("[quota] cannot auto-disable %s (last active secret), quota exceeded %d%%", sec.Label, pct)
				}
				_ = s.quota.MarkAlerted(ctx, sec.Label, 100)
			}
		} else if pct >= 80 {
			alerted, _ := s.quota.WasAlerted(ctx, sec.Label, 80)
			if !alerted {
				log.Printf("[quota] warning: secret %s at %d%% of quota", sec.Label, pct)
				_ = s.quota.MarkAlerted(ctx, sec.Label, 80)
			}
		}
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
				_ = s.secrets.Update(ctx, &sec)
				log.Printf("[expiry] auto-disabled expired secret %s (expired %s)", sec.Label, sec.ExpiresAt)
			}
		}
	}
}
