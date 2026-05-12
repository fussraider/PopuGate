package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/netutil"
)

// ProxyHandler handles proxy control endpoints.
type ProxyHandler struct {
	container *service.ContainerService
	secrets   *store.SecretStore
	settings  *store.SettingsStore
	secretSvc *service.SecretService
	docker    *dockerutil.DockerClient
	instances *store.InstanceStore
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(container *service.ContainerService, secrets *store.SecretStore, settings *store.SettingsStore, secretSvc *service.SecretService) *ProxyHandler {
	return &ProxyHandler{container: container, secrets: secrets, settings: settings, secretSvc: secretSvc}
}

// SetInstanceStore sets the instance store for log streaming.
func (h *ProxyHandler) SetInstanceStore(s *store.InstanceStore) {
	h.instances = s
}

// SetDockerClient sets the Docker client for log streaming.
func (h *ProxyHandler) SetDockerClient(d *dockerutil.DockerClient) {
	h.docker = d
}

// Start handles POST /api/v1/proxy/start
// @Summary      Start proxy
// @Description  Start the proxy container and return links for all enabled secrets
// @Tags         proxy
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /proxy/start [post]
func (h *ProxyHandler) Start(c *gin.Context) {
	if h.container == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable (docker might be missing)"})
		return
	}
	if err := h.container.Start(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to start proxy: %v", err)})
		return
	}

	// Return links for all enabled secrets across all instances
	settings, _ := h.settings.Load(c.Request.Context())
	serverIP := settings.CustomIP
	if serverIP == "" {
		if ip, err := netutil.GetPublicIP(); err == nil {
			serverIP = ip
		} else {
			serverIP = "YOUR_SERVER_IP"
		}
	}

	allLinks, err := h.secretSvc.GetAllLinks(c.Request.Context(), serverIP)
	if err != nil {
		auditLog(c, "proxy.start", "proxy started (links unavailable)")
		c.JSON(http.StatusOK, gin.H{"ok": true, "warning": "links unavailable"})
		return
	}

	auditLog(c, "proxy.start", "proxy started")
	c.JSON(http.StatusOK, gin.H{"ok": true, "links": allLinks})
}

// Stop handles POST /api/v1/proxy/stop
// @Summary      Stop proxy
// @Description  Stop the proxy container
// @Tags         proxy
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /proxy/stop [post]
func (h *ProxyHandler) Stop(c *gin.Context) {
	if h.container == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return
	}
	if err := h.container.Stop(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to stop proxy: %v", err)})
		return
	}
	auditLog(c, "proxy.stop", "proxy stopped")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Restart handles POST /api/v1/proxy/restart
// @Summary      Restart proxy
// @Description  Restart the proxy container
// @Tags         proxy
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /proxy/restart [post]
func (h *ProxyHandler) Restart(c *gin.Context) {
	if h.container == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return
	}
	if err := h.container.Restart(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to restart proxy: %v", err)})
		return
	}
	auditLog(c, "proxy.restart", "proxy restarted")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Reload handles POST /api/v1/proxy/reload
// @Summary      Reload proxy configuration
// @Description  Reload the proxy configuration without restarting the container
// @Tags         proxy
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /proxy/reload [post]
func (h *ProxyHandler) Reload(c *gin.Context) {
	if h.container == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return
	}
	if err := h.container.Reload(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reload proxy: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Status handles GET /api/v1/proxy/status
// @Summary      Proxy status
// @Description  Retrieve the current status of the proxy container
// @Tags         proxy
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /proxy/status [get]
func (h *ProxyHandler) Status(c *gin.Context) {
	if h.container == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return
	}
	status, err := h.container.Status(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get proxy status: %v", err)})
		return
	}
	c.JSON(http.StatusOK, status)
}

// Logs handles GET /api/v1/proxy/logs
// @Summary      Proxy logs
// @Description  Retrieve proxy container logs. Supports SSE streaming when follow=true.
// @Tags         proxy
// @Produce      plain
// @Param        tail    query  int     false  "Number of log lines to return (default 100)"
// @Param        follow  query  bool    false  "Enable SSE streaming of log output"
// @Success      200  {string}  string
// @Failure      500   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Security     BearerAuth
// @Router       /proxy/logs [get]
func (h *ProxyHandler) Logs(c *gin.Context) {
	if h.docker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker not available"})
		return
	}

	// Find first running instance container for log streaming
	var containerName string
	if h.container != nil {
		status, _ := h.container.Status(c.Request.Context())
		if status != nil {
			for _, is := range status.Instances {
				if is.Running {
					containerName = is.ContainerName
					break
				}
			}
		}
	}
	if containerName == "" && h.instances != nil {
		insts, err := h.instances.List(c.Request.Context())
		if err == nil && len(insts) > 0 {
			containerName = insts[0].ContainerName()
		}
	}
	if containerName == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no instances available"})
		return
	}

	tail := c.DefaultQuery("tail", "100")
	follow := c.Query("follow") == "true"
	streamLogs(c, h.docker, containerName, tail, follow)
}
