package netutil

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultServices is the list of IP discovery services used by GetPublicIP.
var defaultServices = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	"https://ident.me",
}

// GetPublicIP attempts to discover the public IP of the server.
func GetPublicIP() (string, error) {
	return GetPublicIPFromServices(defaultServices)
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
