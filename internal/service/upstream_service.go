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
	containerSvc   *ContainerService
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

func (s *UpstreamService) SetContainerSvc(svc *ContainerService) {
	s.containerSvc = svc
}

// middleProxyEnabled reports the effective use_middle_proxy setting. It defaults to
// true (matching the engine default) when the setting is unset or unreadable.
func (s *UpstreamService) middleProxyEnabled(ctx context.Context) bool {
	if s.settings == nil {
		log.Debugf("middleProxyEnabled: settings store not wired, assuming Middle-Proxy enabled")
		return true
	}
	// Get returns "" (no error) for a missing key, so a non-nil error here is a real
	// DB failure. Fail closed (assume enabled) but surface it — otherwise shadowsocks
	// upstreams would be blocked with no trace of why.
	v, err := s.settings.Get(ctx, "use_middle_proxy")
	if err != nil {
		log.Warnf("failed to read use_middle_proxy setting, assuming Middle-Proxy enabled: %v", err)
		return true
	}
	return v != "false"
}

// ensureShadowsocksAllowed enforces ADR-001: a shadowsocks upstream cannot be used
// while Middle-Proxy mode is enabled, because telemt rejects shadowsocks upstreams in
// ME mode. Returns a clear, actionable error rather than letting the engine reject the
// whole config. No-op for non-shadowsocks upstreams.
func (s *UpstreamService) ensureShadowsocksAllowed(ctx context.Context, u *model.Upstream) error {
	if u == nil || u.Type != model.UpstreamShadowsocks {
		return nil
	}
	if s.middleProxyEnabled(ctx) {
		return fmt.Errorf("shadowsocks upstream requires Middle-Proxy mode disabled: set use_middle_proxy=false (note: this turns off ad_tag / promoted-channel support)")
	}
	return nil
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
		if s.containerSvc != nil {
			if err := s.containerSvc.Reload(ctx, fmt.Sprintf("upstream %s auto-disabled", name)); err != nil {
				log.Warnf("handleFailover: failed to hot-reload instances: %v", err)
			}
		}
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

func (s *UpstreamService) handleAutoRecovery(ctx context.Context, u *model.Upstream, latency int64) {
	name := u.Name
	// ADR-001: never auto-re-enable a shadowsocks upstream while Middle-Proxy
	// mode is on — telemt would reject the whole engine config on reload.
	if err := s.ensureShadowsocksAllowed(ctx, u); err != nil {
		log.Warnf("upstream %s recovered but stays disabled: %v", name, err)
		return
	}
	if err := s.upstreams.EnableAutomatically(ctx, name); err == nil {
		log.Infof("upstream %s auto-recovered and re-enabled", name)
		if s.containerSvc != nil {
			if err := s.containerSvc.Reload(ctx, fmt.Sprintf("upstream %s auto-recovered", name)); err != nil {
				log.Warnf("handleAutoRecovery: failed to hot-reload instances: %v", err)
			}
		}
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
		s.handleAutoRecovery(ctx, &u, latency)
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
	if err := s.ensureShadowsocksAllowed(ctx, u); err != nil {
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
	log.Infof("running initial health check for upstream %s (%s)", name, u.Address)
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, testErr := s.testUpstream(bgCtx, u)
	ok, latency, errMsg := resolveTestResult(res, testErr)

	log.Infof("initial health check completed for %s: ok=%v, latency=%dms, err=%s", name, ok, latency, errMsg)

	if err := s.upstreams.UpdateHealth(bgCtx, name, ok, latency, errMsg); err != nil {
		log.Errorf("failed to update initial health for %s: %v", name, err)
	}
}

// Update modifies an existing upstream.
func (s *UpstreamService) Update(ctx context.Context, name string, u *model.Upstream) error {
	if err := u.Validate(); err != nil {
		return err
	}
	if err := s.ensureShadowsocksAllowed(ctx, u); err != nil {
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
	if enable {
		if err := s.ensureShadowsocksAllowed(ctx, existing); err != nil {
			return err
		}
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
	ok, latency, errMsg := resolveTestResult(res, testErr)

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
	case model.UpstreamShadowsocks:
		return s.testShadowsocks(ctx, u)
	default:
		return nil, fmt.Errorf("unsupported upstream type: %s", u.Type)
	}
}

// tcpProbe checks TCP reachability of addr (10s timeout) and reports the connect
// latency. Used by the SOCKS4 and Shadowsocks testers, which verify reachability
// only (no protocol handshake).
func tcpProbe(ctx context.Context, addr string) *model.UpstreamTestResult {
	start := time.Now()
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		log.Debugf("TCP connect to %s failed: %v", addr, err)
		return &model.UpstreamTestResult{OK: false, Error: fmt.Sprintf("TCP connect to %s failed: %v", addr, err)}
	}
	_ = conn.Close()
	latency := time.Since(start).Milliseconds()
	log.Debugf("TCP probe of %s ok: latency=%dms", addr, latency)
	return &model.UpstreamTestResult{OK: true, LatencyMs: latency}
}

// testShadowsocks performs a TCP-reachability probe to the shadowsocks server's
// host:port parsed from the ss:// URL. A full Shadowsocks handshake test is
// intentionally out of scope for v1 (the telemt engine performs the real connection).
func (s *UpstreamService) testShadowsocks(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	addr, err := shadowsocksHostPort(u.URL)
	if err != nil {
		// Legacy fully-base64 ss:// URLs keep the host inside the base64 blob,
		// so no probe can run — report OK instead of failing, otherwise the
		// scheduler would auto-disable a perfectly healthy upstream forever.
		log.Debugf("shadowsocks test: skipping reachability probe: %v", err)
		return &model.UpstreamTestResult{OK: true}, nil
	}
	return tcpProbe(ctx, addr), nil
}

// shadowsocksHostPort extracts the server host:port from an ss:// URL.
// It supports the SIP002 userinfo form (ss://<base64>@host:port#tag) by taking the
// authority after the last '@', stripping any '#fragment' or '/path' suffix.
// The legacy fully-base64 form (ss://base64(method:pw@host:port)) is not decoded
// here; such URLs return an error and skip the reachability probe.
func shadowsocksHostPort(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "ss://")
	if i := strings.LastIndex(s, "@"); i != -1 {
		s = s[i+1:]
	} else {
		return "", fmt.Errorf("cannot determine shadowsocks host:port from URL (legacy/base64 form)")
	}
	if i := strings.IndexAny(s, "#/?"); i != -1 {
		s = s[:i]
	}
	if _, _, err := net.SplitHostPort(s); err != nil {
		return "", fmt.Errorf("invalid shadowsocks host:port %q: %w", s, err)
	}
	return s, nil
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
	return tcpProbe(ctx, u.Address), nil
}

// ParseProxyLine parses a proxy string in various formats and returns a model.Upstream.
func ParseProxyLine(line string) (*model.Upstream, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty line")
	}

	u := &model.Upstream{
		Type:   model.UpstreamSOCKS5, // Default type
		Weight: 10,                   // Default weight
	}

	// Shadowsocks: the whole line is an ss:// URL — no host:port parsing applies.
	if strings.HasPrefix(line, "ss://") {
		u.Type = model.UpstreamShadowsocks
		u.URL = line
		return u, nil
	}

	// 1. Extract scheme
	if strings.HasPrefix(line, "socks5://") {
		u.Type = model.UpstreamSOCKS5
		line = line[len("socks5://"):]
	} else if strings.HasPrefix(line, "socks4://") {
		u.Type = model.UpstreamSOCKS4
		line = line[len("socks4://"):]
	}

	// 2. Check for @ format: credentials@host:port
	// IPv6 hosts must be bracketed (e.g. user:pass@[::1]:1080); net.SplitHostPort
	// handles the bracketed address below.
	if idx := strings.LastIndex(line, "@"); idx != -1 {
		creds := line[:idx]
		addr := line[idx+1:]

		u.Address = addr

		// Split credentials
		if cIdx := strings.Index(creds, ":"); cIdx != -1 {
			u.Username = creds[:cIdx]
			u.Password = creds[cIdx+1:]
		} else {
			u.Username = creds
		}
	} else if strings.HasPrefix(line, "[") {
		if err := parseBracketedIPv6Line(line, u); err != nil {
			return nil, err
		}
	} else if err := parseColonFormLine(line, u); err != nil {
		return nil, err
	}

	// Basic validation of address
	if u.Address == "" {
		return nil, fmt.Errorf("missing address")
	}
	if _, _, err := net.SplitHostPort(u.Address); err != nil {
		return nil, fmt.Errorf("invalid host:port address: %s", u.Address)
	}

	return u, nil
}

// parseBracketedIPv6Line parses a bracketed IPv6 line with optional suffix
// credentials: [::1]:1080 or [::1]:1080:user:pass.
func parseBracketedIPv6Line(line string, u *model.Upstream) error {
	end := strings.Index(line, "]")
	if end == -1 {
		return fmt.Errorf("invalid IPv6 proxy format (missing ']'): %s", line)
	}
	host := line[:end+1] // includes brackets
	rest := strings.TrimPrefix(line[end+1:], ":")
	parts := strings.SplitN(rest, ":", 3) // port, user, pass (pass may contain colons)
	if parts[0] == "" {
		return fmt.Errorf("invalid IPv6 proxy format (missing port): %s", line)
	}
	u.Address = fmt.Sprintf("%s:%s", host, parts[0])
	if len(parts) >= 2 {
		u.Username = parts[1]
	}
	if len(parts) >= 3 {
		u.Password = parts[2]
	}
	return nil
}

// parseColonFormLine parses host:port with optional suffix credentials:
// host:port:user:pass or host:port:user or host:port. This colon-splitting
// form only supports IPv4/hostnames; IPv6 must use the bracketed form.
func parseColonFormLine(line string, u *model.Upstream) error {
	parts := strings.Split(line, ":")
	switch {
	case len(parts) >= 4:
		// host:port:user:pass
		u.Address = fmt.Sprintf("%s:%s", parts[0], parts[1])
		u.Username = parts[2]
		u.Password = strings.Join(parts[3:], ":") // In case password has colons
	case len(parts) == 3:
		// host:port:user
		u.Address = fmt.Sprintf("%s:%s", parts[0], parts[1])
		u.Username = parts[2]
	case len(parts) == 2:
		// host:port
		u.Address = line
	default:
		return fmt.Errorf("invalid proxy format: %s", line)
	}
	return nil
}

// GenerateBulkUpstreamName generates a unique, valid upstream name for a bulk proxy.
//
// The name embeds an 8-hex identity hash derived from the full upstream identity
// (type, address, credentials, interface). This ensures two proxies sharing the
// same host:port but differing in credentials produce distinct names instead of
// silently colliding under the store's INSERT OR IGNORE de-duplication, while
// genuinely identical upstreams still map to the same name (intentional dedup).
// The resulting name is always at most 32 characters.
func GenerateBulkUpstreamName(u *model.Upstream) string {
	prefix := bulkNamePrefix(u.Type)
	host, port := bulkNameHostPort(u)
	hashStr := upstreamIdentityHash(u)
	cleanedHost := sanitizeHostSegment(host)

	// Budget the host segment so the final name never exceeds 32 chars.
	// Layout: <prefix>-<host>-<port>-<hash>  (3 separators).
	maxHostLen := 32 - len(prefix) - 3 - len(port) - len(hashStr)
	if maxHostLen < 0 {
		maxHostLen = 0
	}
	if len(cleanedHost) > maxHostLen {
		cleanedHost = cleanedHost[:maxHostLen]
	}
	cleanedHost = strings.Trim(cleanedHost, "-")

	name := fmt.Sprintf("%s-%s-%s-%s", prefix, cleanedHost, port, hashStr)
	return strings.Trim(collapseDashes(name), "-")
}

func bulkNamePrefix(t model.UpstreamType) string {
	switch t {
	case model.UpstreamSOCKS4:
		return "s4"
	case model.UpstreamDirect:
		return "dir"
	case model.UpstreamShadowsocks:
		return "ss"
	default:
		return "s5"
	}
}

// bulkNameHostPort derives a display host:port for the generated name.
func bulkNameHostPort(u *model.Upstream) (host, port string) {
	if u.Type == model.UpstreamShadowsocks {
		// Shadowsocks has no Address; derive a display host:port from the ss:// URL.
		if addr, err := shadowsocksHostPort(u.URL); err == nil {
			host, port, _ = net.SplitHostPort(addr)
			return host, port
		}
		return "ss", "0"
	}
	host, port, err := net.SplitHostPort(u.Address)
	if err != nil {
		return u.Address, "0"
	}
	return host, port
}

// upstreamIdentityHash hashes the full upstream identity (NUL-separated to
// avoid ambiguity between adjacent fields). URL is appended only when set so
// distinct shadowsocks servers produce distinct names, while hashes of
// pre-existing non-shadowsocks upstreams stay identical to earlier releases
// (name-based dedup on re-import must keep matching old rows).
// Always returns exactly 8 hex chars.
func upstreamIdentityHash(u *model.Upstream) string {
	parts := []string{string(u.Type), u.Address, u.Username, u.Password, u.Iface}
	if u.URL != "" {
		parts = append(parts, u.URL)
	}
	identity := strings.Join(parts, "\x00")
	h := uint32(0)
	for i := 0; i < len(identity); i++ {
		h = h*31 + uint32(identity[i])
	}
	return fmt.Sprintf("%08x", h)
}

// sanitizeHostSegment replaces non-alphanumeric chars with dashes, collapses
// runs of dashes and trims them from both ends.
func sanitizeHostSegment(host string) string {
	var b strings.Builder
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(collapseDashes(b.String()), "-")
}

func collapseDashes(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

// AddMultiple inserts multiple upstreams inside a transaction.
// Returns the upstreams that were actually inserted (duplicates skipped by the
// store are excluded) plus the names of shadowsocks entries skipped because
// Middle-Proxy mode is enabled (ADR-001) — skipping them keeps the rest of the
// batch usable instead of rejecting it wholesale.
func (s *UpstreamService) AddMultiple(ctx context.Context, upstreams []*model.Upstream) ([]*model.Upstream, []string, error) {
	middleProxyOn := s.middleProxyEnabled(ctx)

	toAdd := make([]*model.Upstream, 0, len(upstreams))
	var skippedSS []string
	for _, u := range upstreams {
		if err := u.Validate(); err != nil {
			return nil, nil, err
		}
		if u.Type == model.UpstreamShadowsocks && middleProxyOn {
			skippedSS = append(skippedSS, u.Name)
			continue
		}
		u.Enabled = true
		toAdd = append(toAdd, u)
	}
	if len(skippedSS) > 0 {
		log.Warnf("bulk add: skipped %d shadowsocks upstream(s), Middle-Proxy mode is enabled: %v", len(skippedSS), skippedSS)
	}
	if len(toAdd) == 0 {
		return nil, skippedSS, nil
	}

	inserted, err := s.upstreams.CreateMultiple(ctx, toAdd)
	if err != nil {
		return nil, skippedSS, err
	}

	// Run initial health checks asynchronously only for the newly inserted
	// upstreams (skipping duplicates that were ignored by the store).
	for _, u := range inserted {
		go func(name string, upstreamCopy model.Upstream) {
			defer func() {
				if r := recover(); r != nil {
					log.Warnf("goroutine panic (initial health check %s): %v", name, r)
				}
			}()
			s.runInitialHealthCheck(name, &upstreamCopy)
		}(u.Name, *u)
	}

	return inserted, skippedSS, nil
}
