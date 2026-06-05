package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var log = logger.WithScope("upstream")

// UpstreamService handles upstream business logic.
type UpstreamService struct {
	upstreams      *store.UpstreamStore
	client         *http.Client
	notify         NotifyFunc
	notifyWithBtns NotifyWithButtonsFunc
	settings       *store.SettingsStore
}

// NewUpstreamService creates a new UpstreamService.
func NewUpstreamService(upstreams *store.UpstreamStore) *UpstreamService {
	return &UpstreamService{
		upstreams: upstreams,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// SetNotify sets the notification function.
func (s *UpstreamService) SetNotify(fn NotifyFunc) {
	s.notify = fn
}

func (s *UpstreamService) SetNotifyWithButtons(fn NotifyWithButtonsFunc) {
	s.notifyWithBtns = fn
}

func (s *UpstreamService) SetSettings(settings *store.SettingsStore) {
	s.settings = settings
}

func resolveTestResult(res *model.UpstreamTestResult, testErr error) (bool, int64, string) {
	ok := testErr == nil && res.OK
	errMsg := ""
	if testErr != nil {
		errMsg = testErr.Error()
	} else if res != nil && !res.OK {
		errMsg = res.Error
	}
	latency := int64(0)
	if res != nil {
		latency = res.LatencyMs
	}
	return ok, latency, errMsg
}

func (s *UpstreamService) handleFailover(ctx context.Context, name string, errMsg string) {
	latest, err := s.upstreams.GetByName(ctx, name)
	if err != nil {
		log.Warnf("handleFailover: failed to get upstream %s: %v", name, err)
		return
	}
	if latest == nil || latest.FailCount < 3 || !latest.Enabled {
		return
	}
	if err := s.upstreams.DisableAutomatically(ctx, name, time.Now().Unix()); err == nil {
		log.Warnf("upstream %s auto-disabled after %d failures", name, latest.FailCount)
		var btn KeyboardButton
		if s.settings != nil {
			s2, _ := s.settings.Load(ctx)
			if s2 != nil && s2.WebURL != "" {
				btn = KeyboardButton{Text: "Upstreams", URL: s2.WebURL + "/upstreams"}
			}
		}
		if s.notifyWithBtns != nil && btn.URL != "" {
			s.notifyWithBtns(ctx, "🚫 *%s* Upstream auto-disabled after 3 failures: %s", []KeyboardButton{btn}, name, errMsg)
		} else if s.notify != nil {
			s.notify(ctx, "🚫 *%s* Upstream auto-disabled after 3 failures: %s", name, errMsg)
		}
	}
}

func (s *UpstreamService) handleAutoRecovery(ctx context.Context, name string, latency int64) {
	if err := s.upstreams.EnableAutomatically(ctx, name); err == nil {
		log.Infof("upstream %s auto-recovered and re-enabled", name)
		var btn KeyboardButton
		if s.settings != nil {
			s2, _ := s.settings.Load(ctx)
			if s2 != nil && s2.WebURL != "" {
				btn = KeyboardButton{Text: "Upstreams", URL: s2.WebURL + "/upstreams"}
			}
		}
		if s.notifyWithBtns != nil && btn.URL != "" {
			s.notifyWithBtns(ctx, "✅ *%s* Upstream auto-recovered and re-enabled (latency: %dms)", []KeyboardButton{btn}, name, latency)
		} else if s.notify != nil {
			s.notify(ctx, "✅ *%s* Upstream auto-recovered and re-enabled (latency: %dms)", name, latency)
		}
	} else {
		log.Errorf("failed to auto-enable upstream %s: %v", name, err)
	}
}

func (s *UpstreamService) checkUpstream(ctx context.Context, u model.Upstream) {
	res, testErr := s.testUpstream(ctx, &u)
	ok, latency, errMsg := resolveTestResult(res, testErr)

	if err := s.upstreams.UpdateHealth(ctx, u.Name, ok, latency, errMsg); err != nil {
		log.Errorf("failed to update health for %s: %v", u.Name, err)
	}

	if !ok {
		s.handleFailover(ctx, u.Name, errMsg)
	} else if u.AutoDisabled {
		s.handleAutoRecovery(ctx, u.Name, latency)
	}
}

// CheckAllUpstreams iterates through all enabled or auto-disabled upstreams and performs health checks.
func (s *UpstreamService) CheckAllUpstreams(ctx context.Context) error {
	upstreams, err := s.upstreams.List(ctx)
	if err != nil {
		return err
	}

	for _, u := range upstreams {
		if !u.Enabled && !u.AutoDisabled {
			continue
		}
		s.checkUpstream(ctx, u)
	}
	return nil
}

// List returns all upstreams.
func (s *UpstreamService) List(ctx context.Context) ([]model.Upstream, error) {
	return s.upstreams.List(ctx)
}

// Get returns a single upstream by name.
func (s *UpstreamService) Get(ctx context.Context, name string) (*model.Upstream, error) {
	return s.upstreams.GetByName(ctx, name)
}

// Add creates a new upstream.
func (s *UpstreamService) Add(ctx context.Context, u *model.Upstream) error {
	if err := u.Validate(); err != nil {
		return err
	}

	existing, err := s.upstreams.GetByName(ctx, u.Name)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("upstream '%s' already exists", u.Name)
	}

	u.Enabled = true
	if err := s.upstreams.Create(ctx, u); err != nil {
		return err
	}

	// Run initial health check asynchronously so the API responds immediately.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Warnf("goroutine panic (initial health check %s): %v", u.Name, r)
			}
		}()
		s.runInitialHealthCheck(u.Name, u)
	}()

	return nil
}

func (s *UpstreamService) runInitialHealthCheck(name string, u *model.Upstream) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, testErr := s.testUpstream(bgCtx, u)
	ok := testErr == nil && res.OK
	errMsg := ""
	if testErr != nil {
		errMsg = testErr.Error()
	} else if res != nil && !res.OK {
		errMsg = res.Error
	}
	latency := int64(0)
	if res != nil {
		latency = res.LatencyMs
	}
	if err := s.upstreams.UpdateHealth(bgCtx, name, ok, latency, errMsg); err != nil {
		log.Errorf("failed to update initial health for %s: %v", name, err)
	}
}

// Update modifies an existing upstream.
func (s *UpstreamService) Update(ctx context.Context, name string, u *model.Upstream) error {
	if err := u.Validate(); err != nil {
		return err
	}

	existing, err := s.upstreams.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("upstream '%s' not found", name)
	}

	// Preserve the original name and enabled state
	u.Name = existing.Name
	u.Enabled = existing.Enabled

	return s.upstreams.Update(ctx, name, u)
}

// Remove deletes an upstream by name.
func (s *UpstreamService) Remove(ctx context.Context, name string) error {
	existing, err := s.upstreams.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("upstream '%s' not found", name)
	}
	return s.upstreams.Delete(ctx, name)
}

// Toggle enables or disables an upstream.
func (s *UpstreamService) Toggle(ctx context.Context, name string, enable bool) error {
	existing, err := s.upstreams.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("upstream '%s' not found", name)
	}
	if err := s.upstreams.UpdateEnabled(ctx, name, enable); err != nil {
		return err
	}
	// Clear stale health data when disabling — CheckAllUpstreams skips disabled
	// upstreams, so the last recorded status would be misleading.
	if !enable {
		if err := s.upstreams.ClearHealth(ctx, name); err != nil {
			log.Warnf("failed to clear health for disabled upstream %s: %v", name, err)
		}
	}
	return nil
}

// Test tests connectivity through an upstream and persists the result.
func (s *UpstreamService) Test(ctx context.Context, name string) (*model.UpstreamTestResult, error) {
	u, err := s.upstreams.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("upstream '%s' not found", name)
	}

	res, testErr := s.testUpstream(ctx, u)
	ok := testErr == nil && res.OK
	errMsg := ""
	if testErr != nil {
		errMsg = testErr.Error()
	} else if res != nil && !res.OK {
		errMsg = res.Error
	}
	latency := int64(0)
	if res != nil {
		latency = res.LatencyMs
	}

	if err := s.upstreams.UpdateHealth(ctx, name, ok, latency, errMsg); err != nil {
		log.Errorf("failed to update health for %s after manual test: %v", name, err)
	}

	return res, testErr
}

// TestConfig tests connectivity using raw upstream data (no DB lookup).
func (s *UpstreamService) TestConfig(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	return s.testUpstream(ctx, u)
}

func (s *UpstreamService) testUpstream(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	switch u.Type {
	case model.UpstreamDirect:
		return s.testDirect(ctx)
	case model.UpstreamSOCKS5:
		return s.testSOCKS5(ctx, u)
	case model.UpstreamSOCKS4:
		return s.testSOCKS4(ctx, u)
	default:
		return nil, fmt.Errorf("unsupported upstream type: %s", u.Type)
	}
}

func (s *UpstreamService) testDirect(ctx context.Context) (*model.UpstreamTestResult, error) {
	log.Debugf("testing direct connection")
	start := time.Now()
	ip, err := s.detectIP(s.client)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		log.Debugf("direct test failed: %v", err)
		return &model.UpstreamTestResult{OK: false, Error: err.Error()}, nil
	}
	log.Debugf("direct test ok: ip=%s latency=%dms", ip, latency)
	return &model.UpstreamTestResult{
		OK:        true,
		ExitIP:    ip,
		LatencyMs: latency,
	}, nil
}

func (s *UpstreamService) testSOCKS5(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	log.Debugf("testing SOCKS5 upstream: addr=%s user=%s iface=%s", u.Address, u.Username, u.Iface)

	// Step 1: basic TCP reachability
	start := time.Now()
	conn, err := net.DialTimeout("tcp", u.Address, 10*time.Second)
	if err != nil {
		log.Debugf("TCP connect to %s failed: %v", u.Address, err)
		return &model.UpstreamTestResult{OK: false, Error: fmt.Sprintf("TCP connect to %s failed: %v", u.Address, err)}, nil
	}
	_ = conn.Close()
	tcpMs := time.Since(start).Milliseconds()
	log.Debugf("TCP connect to %s ok (%dms)", u.Address, tcpMs)

	// Step 2: SOCKS5 handshake + HTTP request to detect exit IP
	proxyURL := &url.URL{Scheme: "socks5h", Host: u.Address}
	if u.Username != "" {
		proxyURL.User = url.UserPassword(u.Username, u.Password)
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	start = time.Now()
	ip, err := s.detectIP(client)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		log.Debugf("SOCKS5 HTTP request failed (TCP was ok %dms): %v", tcpMs, err)
		return &model.UpstreamTestResult{
			OK:    false,
			Error: fmt.Sprintf("proxy reachable (TCP %dms) but HTTP request failed: %v", tcpMs, err),
		}, nil
	}
	log.Debugf("SOCKS5 test ok: exit_ip=%s latency=%dms (tcp=%dms)", ip, latency, tcpMs)
	return &model.UpstreamTestResult{
		OK:        true,
		ExitIP:    ip,
		LatencyMs: latency,
	}, nil
}

// detectIP tries multiple IP detection services with fallback.
func (s *UpstreamService) detectIP(client *http.Client) (string, error) {
	endpoints := []string{
		"https://api.ipify.org",
		"https://checkip.amazonaws.com",
		"https://icanhazip.com",
	}
	var lastErr error
	for _, ep := range endpoints {
		log.Debugf("trying IP detection service: %s", ep)
		resp, err := client.Get(ep)
		if err != nil {
			log.Debugf("service %s failed: %v", ep, err)
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		if resp.StatusCode == 200 && len(body) > 0 {
			ip := strings.TrimSpace(string(body))
			log.Debugf("service %s returned ip=%s (status %d)", ep, ip, resp.StatusCode)
			return ip, nil
		}
		log.Debugf("service %s returned status %d, trying next", ep, resp.StatusCode)
		lastErr = fmt.Errorf("%s returned %d", ep, resp.StatusCode)
	}
	log.Debugf("all IP detection services failed, last error: %v", lastErr)
	return "", lastErr
}

func (s *UpstreamService) testSOCKS4(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	log.Debugf("testing SOCKS4 upstream: addr=%s", u.Address)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", u.Address, 10*time.Second)
	if err != nil {
		log.Debugf("SOCKS4 TCP connect to %s failed: %v", u.Address, err)
		return &model.UpstreamTestResult{OK: false, Error: err.Error()}, nil
	}
	defer func() { _ = conn.Close() }()

	latency := time.Since(start).Milliseconds()
	log.Debugf("SOCKS4 test ok: latency=%dms", latency)
	return &model.UpstreamTestResult{
		OK:        true,
		LatencyMs: latency,
	}, nil
}
