package netutil

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultServices is the list of IP discovery services used by GetPublicIP.
var defaultServices = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	"https://ident.me",
}

const publicIPTTL = 5 * time.Minute

var (
	ipCacheMu sync.RWMutex
	ipCache   string
	ipCacheAt time.Time

	// inFlight prevents thundering herd: only one goroutine fetches at a time
	ipInFlight sync.Mutex
)

// GetPublicIP attempts to discover the public IP of the server.
// Results are cached for 5 minutes. Concurrent callers share a single in-flight request.
func GetPublicIP() (string, error) {
	ipCacheMu.RLock()
	if ipCache != "" && time.Since(ipCacheAt) < publicIPTTL {
		cached := ipCache
		ipCacheMu.RUnlock()
		return cached, nil
	}
	ipCacheMu.RUnlock()

	// Only one caller fetches at a time; others wait and then read the cache
	ipInFlight.Lock()
	defer ipInFlight.Unlock()

	// Double-check after acquiring lock — another goroutine may have refreshed
	ipCacheMu.RLock()
	if ipCache != "" && time.Since(ipCacheAt) < publicIPTTL {
		cached := ipCache
		ipCacheMu.RUnlock()
		return cached, nil
	}
	ipCacheMu.RUnlock()

	ip, err := GetPublicIPFromServices(defaultServices)
	if err != nil {
		// Return stale cache if available
		ipCacheMu.RLock()
		cached := ipCache
		ipCacheMu.RUnlock()
		if cached != "" {
			return cached, nil
		}
		return "", err
	}

	ipCacheMu.Lock()
	ipCache = ip
	ipCacheAt = time.Now()
	ipCacheMu.Unlock()

	return ip, nil
}

// InvalidatePublicIPCache forces the next GetPublicIP call to fetch fresh data.
func InvalidatePublicIPCache() {
	ipCacheMu.Lock()
	ipCache = ""
	ipCacheAt = time.Time{}
	ipCacheMu.Unlock()
}

// GetPublicIPFromServices attempts to discover the public IP by querying the
// given service URLs in order. It returns the IP from the first successful response.
func GetPublicIPFromServices(services []string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			ip := strings.TrimSpace(string(body))
			if ip != "" {
				return ip, nil
			}
		}
	}

	return "", io.EOF
}
