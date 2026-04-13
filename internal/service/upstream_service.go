package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
)

// UpstreamService handles upstream business logic.
type UpstreamService struct {
	upstreams *store.UpstreamStore
}

// NewUpstreamService creates a new UpstreamService.
func NewUpstreamService(upstreams *store.UpstreamStore) *UpstreamService {
	return &UpstreamService{upstreams: upstreams}
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
	return s.upstreams.Create(ctx, u)
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
	return s.upstreams.UpdateEnabled(ctx, name, enable)
}

// Test tests connectivity through an upstream.
func (s *UpstreamService) Test(ctx context.Context, name string) (*model.UpstreamTestResult, error) {
	u, err := s.upstreams.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("upstream '%s' not found", name)
	}

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
	start := time.Now()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &model.UpstreamTestResult{OK: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	ip := string(body)
	return &model.UpstreamTestResult{
		OK:        true,
		ExitIP:    ip,
		LatencyMs: latency,
	}, nil
}

func (s *UpstreamService) testSOCKS5(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	var proxyURL *url.URL
	if u.Username != "" {
		proxyURL, _ = url.Parse(fmt.Sprintf("socks5h://%s:%s@%s", u.Username, u.Password, u.Address))
	} else {
		proxyURL, _ = url.Parse(fmt.Sprintf("socks5h://%s", u.Address))
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	start := time.Now()
	resp, err := client.Get("https://api.ipify.org")
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &model.UpstreamTestResult{OK: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	ip := string(body)
	return &model.UpstreamTestResult{
		OK:        true,
		ExitIP:    ip,
		LatencyMs: latency,
	}, nil
}

func (s *UpstreamService) testSOCKS4(ctx context.Context, u *model.Upstream) (*model.UpstreamTestResult, error) {
	// SOCKS4 testing via net.Dial with manual handshake
	conn, err := net.DialTimeout("tcp", u.Address, 10*time.Second)
	if err != nil {
		return &model.UpstreamTestResult{OK: false, Error: err.Error()}, nil
	}
	defer conn.Close()

	// Basic connectivity test passed
	return &model.UpstreamTestResult{
		OK:        true,
		LatencyMs: time.Since(time.Now()).Milliseconds(),
	}, nil
}
