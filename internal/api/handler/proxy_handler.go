package handler

import (
	"bufio"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/netutil"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

// ProxyHandler handles proxy control endpoints.
type ProxyHandler struct {
	container *service.ContainerService
	secrets   *store.SecretStore
	settings  *store.SettingsStore
	docker    *dockerutil.DockerClient
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(container *service.ContainerService, secrets *store.SecretStore, settings *store.SettingsStore) *ProxyHandler {
	return &ProxyHandler{container: container, secrets: secrets, settings: settings}
}

// SetDockerClient sets the Docker client for log streaming.
func (h *ProxyHandler) SetDockerClient(d *dockerutil.DockerClient) {
	h.docker = d
}

// Start handles POST /api/v1/proxy/start
func (h *ProxyHandler) Start(c *gin.Context) {
	if err := h.container.Start(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Return links for all enabled secrets
	settings, _ := h.settings.Load(c.Request.Context())
	secrets, _ := h.secrets.List(c.Request.Context())

	type linkEntry struct {
		Label   string `json:"label"`
		TGLink  string `json:"tg_link"`
		WebLink string `json:"web_link"`
	}
	var links []linkEntry

	serverIP := settings.CustomIP
	if serverIP == "" {
		if ip, err := netutil.GetPublicIP(); err == nil {
			serverIP = ip
		} else {
			serverIP = "YOUR_SERVER_IP"
		}
	}

	for _, sec := range secrets {
		if !sec.Enabled {
			continue
		}
		fullSecret := telemt.BuildFakeTLSSecret(sec.SecretKey, settings.ProxyDomain, settings.MaskingEnabled)
		links = append(links, linkEntry{
			Label:   sec.Label,
			TGLink:  telemt.BuildProxyLink(serverIP, settings.ProxyPort, fullSecret),
			WebLink: telemt.BuildWebLink(serverIP, settings.ProxyPort, fullSecret),
		})
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "links": links})
}

// Stop handles POST /api/v1/proxy/stop
func (h *ProxyHandler) Stop(c *gin.Context) {
	if err := h.container.Stop(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Restart handles POST /api/v1/proxy/restart
func (h *ProxyHandler) Restart(c *gin.Context) {
	if err := h.container.Restart(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Reload handles POST /api/v1/proxy/reload
func (h *ProxyHandler) Reload(c *gin.Context) {
	if err := h.container.Reload(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Status handles GET /api/v1/proxy/status
func (h *ProxyHandler) Status(c *gin.Context) {
	status, err := h.container.Status(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, status)
}

// Logs handles GET /api/v1/proxy/logs (supports SSE when follow=true)
func (h *ProxyHandler) Logs(c *gin.Context) {
	if h.docker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker not available"})
		return
	}

	tail := c.DefaultQuery("tail", "100")
	follow := c.Query("follow") == "true"

	if follow {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
	} else {
		c.Header("Content-Type", "text/plain")
	}

	logs, err := h.docker.Logs(c.Request.Context(), tail, follow)
	if err != nil {
		if follow {
			c.SSEvent("error", fmt.Sprintf("failed to get logs: %v", err))
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	defer logs.Close()

	scanner := bufio.NewScanner(logs)

	// Heartbeat ticker for SSE to detect dead connections (L-P06)
	var heartbeat *time.Ticker
	var done chan struct{}
	if follow {
		heartbeat = time.NewTicker(15 * time.Second)
		done = make(chan struct{})
		defer func() {
			heartbeat.Stop()
			close(done)
		}()
		go func() {
			for {
				select {
				case <-heartbeat.C:
					c.SSEvent("heartbeat", time.Now().Unix())
					c.Writer.Flush()
				case <-done:
					return
				}
			}
		}()
	}

	for scanner.Scan() {
		line := scanner.Text()
		// Docker log format: first 8 bytes are header (usually), then payload
		// However, when using a TTY, the header is not present.
		// MTProxyMax usually doesn't use TTY for the container.
		if len(line) > 8 {
			line = line[8:]
		}
		if follow {
			c.SSEvent("message", line)
			c.Writer.Flush()
		} else {
			fmt.Fprintln(c.Writer, line)
		}
	}
}
