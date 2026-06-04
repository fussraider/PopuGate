package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/netutil"
)

const ipdenyURL = "https://www.ipdeny.com/ipblocks/data/aggregated/%s-aggregated.zone"
const geoCacheTTL = 24 * time.Hour

func geoCacheDir() string { return filepath.Join(model.InstallDir, "geoblock") }

// GeoblockService handles geo-blocking via iptables/ipset.
type GeoblockService struct {
	settings  *store.SettingsStore
	instances *store.InstanceStore
	cache     *store.GeoblockCacheStore
	iptables  *netutil.IptablesManager
}

// NewGeoblockService creates a new GeoblockService.
func NewGeoblockService(settings *store.SettingsStore, instances *store.InstanceStore, cache *store.GeoblockCacheStore) *GeoblockService {
	return &GeoblockService{
		settings:  settings,
		instances: instances,
		cache:     cache,
		iptables:  netutil.NewIptablesManager(),
	}
}

// IsAvailable checks if iptables and ipset command line utilities are available on the system.
func (s *GeoblockService) IsAvailable() (bool, string) {
	if _, err := exec.LookPath("iptables"); err != nil {
		return false, "iptables is not installed or not found in PATH"
	}
	if _, err := exec.LookPath("ipset"); err != nil {
		return false, "ipset is not installed or not found in PATH"
	}
	return true, ""
}

func (s *GeoblockService) collectPorts(ctx context.Context, primaryPort int) []string {
	ports := []string{fmt.Sprintf("%d", primaryPort)}
	insts, _ := s.instances.List(ctx)
	for _, inst := range insts {
		if inst.Enabled {
			ports = append(ports, fmt.Sprintf("%d", inst.Port))
		}
	}
	return ports
}

func (s *GeoblockService) applyCountryIPSet(ctx context.Context, code string, cidrs []string, ports []string, action string) error {
	setName := netutil.SetNameForCountry(code)
	if err := s.iptables.CreateIPSet(setName, 131072); err != nil {
		return fmt.Errorf("create ipset %s: %w", setName, err)
	}
	if err := s.iptables.FlushIPSet(setName); err != nil {
		return fmt.Errorf("flush ipset %s: %w", setName, err)
	}
	if err := s.iptables.RestoreIPSet(setName, cidrs); err != nil {
		return fmt.Errorf("restore ipset %s: %w", setName, err)
	}
	for _, port := range ports {
		if err := s.iptables.SetRule(setName, port, action); err != nil {
			return fmt.Errorf("set rule for %s port %s: %w", code, port, err)
		}
	}
	return nil
}

func applyDefaultDeny(iptables *netutil.IptablesManager, ports []string) error {
	for _, port := range ports {
		if err := iptables.SetDefaultDeny(port); err != nil {
			return fmt.Errorf("set default deny port %s: %w", port, err)
		}
	}
	return nil
}

// Apply downloads CIDRs for all configured countries and applies iptables rules.
func (s *GeoblockService) Apply(ctx context.Context) error {
	settings, err := s.settings.Load(ctx)
	if err != nil {
		return err
	}

	if settings.BlocklistCountries == "" {
		return s.Clear(ctx)
	}

	action := "DROP"
	if settings.GeoblockMode == "whitelist" {
		action = "ACCEPT"
	}

	if err := s.iptables.RemoveGeoBlockRules(); err != nil {
		logger.WithScope("geoblock").Warnf("remove existing rules: %v", err)
	}

	ports := s.collectPorts(ctx, settings.ProxyPort)

	for _, raw := range strings.Split(settings.BlocklistCountries, ",") {
		code := strings.TrimSpace(strings.ToLower(raw))
		if code == "" {
			continue
		}
		if !IsValidCountryCode(code) {
			return fmt.Errorf("invalid country code: %q", code)
		}

		cidrs, err := s.getCountryCIDRs(ctx, code)
		if err != nil {
			return fmt.Errorf("get CIDRs for %s: %w", code, err)
		}

		if err := s.applyCountryIPSet(ctx, code, cidrs, ports, action); err != nil {
			return err
		}
	}

	if settings.GeoblockMode == "whitelist" {
		return applyDefaultDeny(s.iptables, ports)
	}

	return nil
}

// Clear removes all geo-blocking rules.
func (s *GeoblockService) Clear(ctx context.Context) error {
	if err := s.iptables.RemoveGeoBlockRules(); err != nil {
		return err
	}

	settings, err := s.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if settings.BlocklistCountries != "" {
		for _, code := range strings.Split(settings.BlocklistCountries, ",") {
			code = strings.TrimSpace(strings.ToLower(code))
			if code != "" {
				if err := s.iptables.DestroyIPSet(netutil.SetNameForCountry(code)); err != nil {
					logger.WithScope("geoblock").Warnf("destroy ipset %s: %v", code, err)
				}
			}
		}
	}
	return nil
}

func (s *GeoblockService) getCountryCIDRs(ctx context.Context, code string) ([]string, error) {
	// Check cache
	_, downloadedAt, err := s.cache.GetCache(ctx, code)
	if err == nil && time.Since(time.Unix(downloadedAt, 0)) < geoCacheTTL {
		// Load from cache file
		cachePath := filepath.Join(geoCacheDir(), code+".zone")
		data, err := os.ReadFile(cachePath)
		if err == nil {
			return parseCIDRs(string(data)), nil
		}
	}

	// Download
	url := fmt.Sprintf(ipdenyURL, code)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", code, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: HTTP %d", code, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return nil, err
	}

	cidrs := parseCIDRs(string(data))

	// Cache to disk
	if err := os.MkdirAll(geoCacheDir(), 0755); err != nil {
		logger.WithScope("geoblock").Warnf("create cache dir: %v", err)
	}
	cachePath := filepath.Join(geoCacheDir(), code+".zone")
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		logger.WithScope("geoblock").Warnf("write cache %s: %v", code, err)
	}

	// Update cache record
	if err := s.cache.SetCache(ctx, code, cachePath); err != nil {
		logger.WithScope("geoblock").Warnf("cache %s: %v", code, err)
	}

	return cidrs, nil
}

func parseCIDRs(data string) []string {
	var cidrs []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			cidrs = append(cidrs, line)
		}
	}
	return cidrs
}
