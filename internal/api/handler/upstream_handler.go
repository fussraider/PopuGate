package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var upstreamLog = logger.WithScope("upstream")

// UpstreamHandler handles upstream endpoints.
type UpstreamHandler struct {
	upstreams    *service.UpstreamService
	containerSvc *service.ContainerService
}

// NewUpstreamHandler creates a new UpstreamHandler.
func NewUpstreamHandler(upstreams *service.UpstreamService) *UpstreamHandler {
	return &UpstreamHandler{upstreams: upstreams}
}

// SetContainerSvc sets the container service.
func (h *UpstreamHandler) SetContainerSvc(svc *service.ContainerService) {
	h.containerSvc = svc
}

func (h *UpstreamHandler) reloadInstances(ctx context.Context, reason string) {
	if h.containerSvc != nil {
		if err := h.containerSvc.Reload(ctx, reason); err != nil {
			upstreamLog.Warnf("reloadInstances: failed to hot-reload instances: %v", err)
		}
	}
}

// List handles GET /api/v1/upstreams
// @Summary      List upstreams
// @Description  Retrieve all configured upstream proxies
// @Tags         upstreams
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams [get]
func (h *UpstreamHandler) List(c *gin.Context) {
	upstreams, err := h.upstreams.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, upstreams)
}

type addUpstreamRequest struct {
	Name     string `json:"name" binding:"required,alphanumdash,max=32"`
	Type     string `json:"type" binding:"required,oneof=direct socks5 socks4"` // direct, socks5, socks4
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Weight   int    `json:"weight" binding:"omitempty,min=1,max=100"`
	Iface    string `json:"iface"`
}

type updateUpstreamRequest struct {
	Type     string `json:"type" binding:"required,oneof=direct socks5 socks4"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Weight   int    `json:"weight" binding:"omitempty,min=1,max=100"`
	Iface    string `json:"iface"`
}

// Update handles PUT /api/v1/upstreams/:name
// @Summary      Update an upstream
// @Description  Update the configuration of an existing upstream proxy
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        name  path  string                true  "Upstream name"
// @Param        body  body  updateUpstreamRequest  true  "Upstream configuration"
// @Success      200  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/{name} [put]
func (h *UpstreamHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var req updateUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	u := &model.Upstream{
		Name:     name,
		Type:     model.UpstreamType(strings.TrimSpace(req.Type)),
		Address:  strings.TrimSpace(req.Address),
		Username: strings.TrimSpace(req.Username),
		Password: strings.TrimSpace(req.Password),
		Weight:   req.Weight,
		Iface:    strings.TrimSpace(req.Iface),
	}
	if u.Weight == 0 {
		u.Weight = 10
	}

	if err := h.upstreams.Update(c.Request.Context(), name, u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	upstreamLog.Infof("updating upstream: %s", name)
	auditLog(c, "upstream.update", fmt.Sprintf("name=%s", name))
	h.reloadInstances(c.Request.Context(), fmt.Sprintf("upstream %s updated", name))
	c.JSON(http.StatusOK, u)
}

type testUpstreamRequest struct {
	Type     string `json:"type" binding:"required,oneof=direct socks5 socks4"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Iface    string `json:"iface"`
}

// Add handles POST /api/v1/upstreams
// @Summary      Add an upstream
// @Description  Create a new upstream proxy configuration
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        body  body  addUpstreamRequest  true  "Upstream to add"
// @Success      201  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams [post]
func (h *UpstreamHandler) Add(c *gin.Context) {
	var req addUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	u := &model.Upstream{
		Name:     strings.TrimSpace(req.Name),
		Type:     model.UpstreamType(strings.TrimSpace(req.Type)),
		Address:  strings.TrimSpace(req.Address),
		Username: strings.TrimSpace(req.Username),
		Password: strings.TrimSpace(req.Password),
		Weight:   req.Weight,
		Iface:    strings.TrimSpace(req.Iface),
	}
	if u.Weight == 0 {
		u.Weight = 10
	}

	if err := h.upstreams.Add(c.Request.Context(), u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	upstreamLog.Infof("adding upstream: name=%s type=%s address=%s", u.Name, u.Type, u.Address)
	auditLog(c, "upstream.create", fmt.Sprintf("name=%s", req.Name))
	h.reloadInstances(c.Request.Context(), fmt.Sprintf("upstream %s added", u.Name))
	c.JSON(http.StatusCreated, u)
}

// Remove handles DELETE /api/v1/upstreams/:name
// @Summary      Remove an upstream
// @Description  Delete an upstream proxy configuration by name
// @Tags         upstreams
// @Produce      json
// @Param        name  path  string  true  "Upstream name"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/{name} [delete]
func (h *UpstreamHandler) Remove(c *gin.Context) {
	name := c.Param("name")
	if err := h.upstreams.Remove(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	upstreamLog.Infof("removing upstream: %s", name)
	auditLog(c, "upstream.delete", fmt.Sprintf("name=%s", name))
	h.reloadInstances(c.Request.Context(), fmt.Sprintf("upstream %s removed", name))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type upstreamToggleRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// Toggle handles PUT /api/v1/upstreams/:name/toggle
// @Summary      Toggle an upstream
// @Description  Enable or disable an upstream proxy by name
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        name  path  string                 true  "Upstream name"
// @Param        body  body  upstreamToggleRequest   true  "Enabled state"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/{name}/toggle [put]
func (h *UpstreamHandler) Toggle(c *gin.Context) {
	name := c.Param("name")
	var req upstreamToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.upstreams.Toggle(c.Request.Context(), name, *req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	action := "starting"
	if !*req.Enabled {
		action = "stopping"
	}
	upstreamLog.Infof("%s upstream: %s", action, name)
	auditLog(c, "upstream.toggle", fmt.Sprintf("name=%s enabled=%v", name, *req.Enabled))
	h.reloadInstances(c.Request.Context(), fmt.Sprintf("upstream %s toggled (%s)", name, action))
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": *req.Enabled})
}

// Test handles POST /api/v1/upstreams/:name/test
// @Summary      Test an upstream
// @Description  Run a connectivity test against an existing upstream by name
// @Tags         upstreams
// @Produce      json
// @Param        name  path  string  true  "Upstream name"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/{name}/test [post]
func (h *UpstreamHandler) Test(c *gin.Context) {
	name := c.Param("name")
	result, err := h.upstreams.Test(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// TestConfig handles POST /api/v1/upstreams/test
// @Summary      Test upstream configuration
// @Description  Test raw upstream configuration without saving it
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        body  body  testUpstreamRequest  true  "Upstream configuration to test"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/test [post]
func (h *UpstreamHandler) TestConfig(c *gin.Context) {
	var req testUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	u := &model.Upstream{
		Type:     model.UpstreamType(strings.TrimSpace(req.Type)),
		Address:  strings.TrimSpace(req.Address),
		Username: strings.TrimSpace(req.Username),
		Password: strings.TrimSpace(req.Password),
		Iface:    strings.TrimSpace(req.Iface),
	}

	result, err := h.upstreams.TestConfig(c.Request.Context(), u)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// netIface describes a host network interface.
type netIface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

// Interfaces handles GET /api/v1/upstreams/interfaces
// @Summary      List network interfaces
// @Description  Retrieve the host's active network interfaces and their addresses
// @Tags         upstreams
// @Produce      json
// @Success      200  {array}   netIface
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/interfaces [get]
func (h *UpstreamHandler) Interfaces(c *gin.Context) {
	ifaces, err := net.Interfaces()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list interfaces"})
		return
	}

	result := make([]netIface, 0, len(ifaces))
	for _, iface := range ifaces {
		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		addrStrs := make([]string, 0, len(addrs))
		for _, a := range addrs {
			addrStrs = append(addrStrs, a.String())
		}
		result = append(result, netIface{
			Name:      iface.Name,
			Addresses: addrStrs,
		})
	}
	c.JSON(http.StatusOK, result)
}

type bulkCheckRequest struct {
	Proxies []string `json:"proxies" binding:"required,min=1,max=100"`
}

type bulkCheckResult struct {
	Input     string `json:"input"`
	Address   string `json:"address"`
	Type      string `json:"type"`
	OK        bool   `json:"ok"`
	ExitIP    string `json:"exit_ip,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// BulkCheck handles POST /api/v1/upstreams/bulk-check
// @Summary      Bulk check proxies
// @Description  Check availability of multiple proxies in real-time, streaming results back via SSE
// @Tags         upstreams
// @Accept       json
// @Produce      text/event-stream
// @Param        body  body  bulkCheckRequest  true  "List of proxies to check"
// @Success      200  {string}  string  "SSE stream of check progress"
// @Security     BearerAuth
// @Router       /upstreams/bulk-check [post]
func (h *UpstreamHandler) BulkCheck(c *gin.Context) {
	var req bulkCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	resultsChan := make(chan bulkCheckResult, len(req.Proxies))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for _, line := range req.Proxies {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		wg.Add(1)
		go func(rawLine string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			u, err := service.ParseProxyLine(rawLine)
			if err != nil {
				select {
				case resultsChan <- bulkCheckResult{Input: rawLine, Error: err.Error(), OK: false}:
				case <-ctx.Done():
				}
				return
			}

			res, err := h.upstreams.TestConfig(ctx, u)
			hasScheme := strings.HasPrefix(rawLine, "socks5://") || strings.HasPrefix(rawLine, "socks4://")
			if !hasScheme && (err != nil || (res != nil && !res.OK)) && u.Type == model.UpstreamSOCKS5 {
				u.Type = model.UpstreamSOCKS4
				res2, err2 := h.upstreams.TestConfig(ctx, u)
				if err2 == nil && res2 != nil && res2.OK {
					res = res2
					err = nil
				}
			}

			ok := err == nil && res.OK
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			} else if res != nil && !res.OK {
				errMsg = res.Error
			}
			latency := int64(0)
			exitIP := ""
			if res != nil {
				latency = res.LatencyMs
				exitIP = res.ExitIP
			}

			select {
			case resultsChan <- bulkCheckResult{
				Input:     rawLine,
				Address:   u.Address,
				Type:      string(u.Type),
				OK:        ok,
				ExitIP:    exitIP,
				LatencyMs: latency,
				Error:     errMsg,
			}:
			case <-ctx.Done():
			}
		}(line)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for {
		select {
		case res, open := <-resultsChan:
			if !open {
				c.SSEvent("complete", "all tests finished")
				c.Writer.Flush()
				return
			}
			c.SSEvent("progress", res)
			c.Writer.Flush()
		case <-ctx.Done():
			return
		}
	}
}

type bulkAddRequestItem struct {
	Type     string `json:"type" binding:"required,oneof=direct socks5 socks4"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Weight   int    `json:"weight" binding:"omitempty,min=1,max=100"`
	Iface    string `json:"iface"`
}

type bulkAddRequest struct {
	Upstreams []bulkAddRequestItem `json:"upstreams" binding:"required,min=1,max=100"`
}

// BulkAdd handles POST /api/v1/upstreams/bulk
// @Summary      Bulk add upstreams
// @Description  Create multiple upstream proxy configurations in a transaction
// @Tags         upstreams
// @Accept       json
// @Produce      json
// @Param        body  body  bulkAddRequest  true  "List of upstreams to add"
// @Success      201  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /upstreams/bulk [post]
func (h *UpstreamHandler) BulkAdd(c *gin.Context) {
	var req bulkAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	upstreams := make([]*model.Upstream, 0, len(req.Upstreams))
	for _, item := range req.Upstreams {
		weight := item.Weight
		if weight == 0 {
			weight = 10
		}
		u := &model.Upstream{
			Type:     model.UpstreamType(strings.TrimSpace(item.Type)),
			Address:  strings.TrimSpace(item.Address),
			Username: strings.TrimSpace(item.Username),
			Password: strings.TrimSpace(item.Password),
			Weight:   weight,
			Iface:    strings.TrimSpace(item.Iface),
		}
		// Generate unique name
		u.Name = service.GenerateBulkUpstreamName(u)

		upstreams = append(upstreams, u)
	}

	insertedCount, err := h.upstreams.AddMultiple(c.Request.Context(), upstreams)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	upstreamLog.Infof("bulk adding %d upstreams", len(upstreams))

	names := make([]string, 0, len(upstreams))
	for _, u := range upstreams {
		names = append(names, u.Name)
	}

	auditLog(c, "upstream.bulk_create", fmt.Sprintf("count=%d", insertedCount))
	h.reloadInstances(c.Request.Context(), fmt.Sprintf("bulk added %d upstreams", len(upstreams)))
	c.JSON(http.StatusCreated, gin.H{
		"ok":    true,
		"count": insertedCount,
		"names": names,
	})
}
