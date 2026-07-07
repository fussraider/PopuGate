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

// upstreamConfigFields holds the connection fields shared by every upstream
// request. Embedded into the request structs so the type whitelist and the
// request→model mapping live in exactly one place.
type upstreamConfigFields struct {
	Type         string `json:"type" binding:"required,oneof=direct socks5 socks4 shadowsocks"`
	Address      string `json:"address"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	URL          string `json:"url"`
	Iface        string `json:"iface"`
	IPv4         *bool  `json:"ipv4"`
	IPv6         *bool  `json:"ipv6"`
	Prefer       int    `json:"prefer" binding:"omitempty,oneof=0 4 6"`
	BindToDevice string `json:"bindtodevice"`
}

// toUpstream maps (and trims) the shared fields into a model.Upstream.
// Caller sets Name/Weight/Enabled as appropriate.
func (f upstreamConfigFields) toUpstream() *model.Upstream {
	return &model.Upstream{
		Type:         model.UpstreamType(strings.TrimSpace(f.Type)),
		Address:      strings.TrimSpace(f.Address),
		Username:     strings.TrimSpace(f.Username),
		Password:     strings.TrimSpace(f.Password),
		URL:          strings.TrimSpace(f.URL),
		Iface:        strings.TrimSpace(f.Iface),
		IPv4:         f.IPv4,
		IPv6:         f.IPv6,
		Prefer:       f.Prefer,
		BindToDevice: strings.TrimSpace(f.BindToDevice),
	}
}

// defaultWeight returns w, or 10 when unset (0).
func defaultWeight(w int) int {
	if w == 0 {
		return 10
	}
	return w
}

type addUpstreamRequest struct {
	Name   string `json:"name" binding:"required,alphanumdash,max=32"`
	Weight int    `json:"weight" binding:"omitempty,min=1,max=100"`
	upstreamConfigFields
}

type updateUpstreamRequest struct {
	Weight int `json:"weight" binding:"omitempty,min=1,max=100"`
	upstreamConfigFields
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

	u := req.toUpstream()
	u.Name = name
	u.Weight = defaultWeight(req.Weight)

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
	upstreamConfigFields
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

	u := req.toUpstream()
	u.Name = strings.TrimSpace(req.Name)
	u.Weight = defaultWeight(req.Weight)

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

	u := req.toUpstream()

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

			select {
			case resultsChan <- h.checkProxyLine(ctx, rawLine):
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

// checkProxyLine parses one bulk-check input line and probes the resulting
// upstream, falling back from SOCKS5 to SOCKS4 for scheme-less lines.
func (h *UpstreamHandler) checkProxyLine(ctx context.Context, rawLine string) bulkCheckResult {
	u, err := service.ParseProxyLine(rawLine)
	if err != nil {
		return bulkCheckResult{Input: rawLine, Error: err.Error(), OK: false}
	}

	res, err := h.upstreams.TestConfig(ctx, u)
	if shouldRetryAsSOCKS4(rawLine, u, res, err) {
		u.Type = model.UpstreamSOCKS4
		if res2, err2 := h.upstreams.TestConfig(ctx, u); err2 == nil && res2 != nil && res2.OK {
			res = res2
			err = nil
		}
	}

	return buildBulkCheckResult(rawLine, u, res, err)
}

// shouldRetryAsSOCKS4 reports whether a failed SOCKS5 probe of a scheme-less
// line should be retried as SOCKS4 (the line's protocol is ambiguous).
func shouldRetryAsSOCKS4(rawLine string, u *model.Upstream, res *model.UpstreamTestResult, err error) bool {
	hasScheme := strings.HasPrefix(rawLine, "socks5://") || strings.HasPrefix(rawLine, "socks4://") || strings.HasPrefix(rawLine, "ss://")
	failed := err != nil || (res != nil && !res.OK)
	return !hasScheme && failed && u.Type == model.UpstreamSOCKS5
}

func buildBulkCheckResult(rawLine string, u *model.Upstream, res *model.UpstreamTestResult, err error) bulkCheckResult {
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
	return bulkCheckResult{
		Input:     rawLine,
		Address:   u.Address,
		Type:      string(u.Type),
		OK:        ok,
		ExitIP:    exitIP,
		LatencyMs: latency,
		Error:     errMsg,
	}
}

type bulkAddRequestItem struct {
	Weight int `json:"weight" binding:"omitempty,min=1,max=100"`
	upstreamConfigFields
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
		u := item.toUpstream()
		u.Weight = defaultWeight(item.Weight)
		u.Name = service.GenerateBulkUpstreamName(u)
		upstreams = append(upstreams, u)
	}

	inserted, skippedSS, err := h.upstreams.AddMultiple(c.Request.Context(), upstreams)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	insertedCount := len(inserted)
	skipped := len(upstreams) - insertedCount - len(skippedSS)
	upstreamLog.Infof("bulk add: %d inserted, %d skipped (duplicates), %d skipped (shadowsocks/middle-proxy)", insertedCount, skipped, len(skippedSS))

	names := make([]string, 0, insertedCount)
	for _, u := range inserted {
		names = append(names, u.Name)
	}

	auditLog(c, "upstream.bulk_create", fmt.Sprintf("count=%d skipped=%d skipped_ss=%d", insertedCount, skipped, len(skippedSS)))
	if insertedCount > 0 {
		h.reloadInstances(c.Request.Context(), fmt.Sprintf("bulk added %d upstreams", insertedCount))
	}
	c.JSON(http.StatusCreated, gin.H{
		"ok":                   true,
		"count":                insertedCount,
		"skipped":              skipped,
		"skipped_middle_proxy": skippedSS,
		"names":                names,
	})
}
