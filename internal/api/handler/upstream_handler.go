package handler

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
)

// UpstreamHandler handles upstream endpoints.
type UpstreamHandler struct {
	upstreams *service.UpstreamService
}

// NewUpstreamHandler creates a new UpstreamHandler.
func NewUpstreamHandler(upstreams *service.UpstreamService) *UpstreamHandler {
	return &UpstreamHandler{upstreams: upstreams}
}

// List handles GET /api/v1/upstreams
func (h *UpstreamHandler) List(c *gin.Context) {
	upstreams, err := h.upstreams.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, upstreams)
}

type addUpstreamRequest struct {
	Name     string `json:"name" binding:"required"`
	Type     string `json:"type" binding:"required"` // direct, socks5, socks4
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Weight   int    `json:"weight"`
	Iface    string `json:"iface"`
}

type testUpstreamRequest struct {
	Type     string `json:"type" binding:"required"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Iface    string `json:"iface"`
}

// Add handles POST /api/v1/upstreams
func (h *UpstreamHandler) Add(c *gin.Context) {
	var req addUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	c.JSON(http.StatusCreated, u)
}

// Remove handles DELETE /api/v1/upstreams/:name
func (h *UpstreamHandler) Remove(c *gin.Context) {
	name := c.Param("name")
	if err := h.upstreams.Remove(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type upstreamToggleRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// Toggle handles PUT /api/v1/upstreams/:name/toggle
func (h *UpstreamHandler) Toggle(c *gin.Context) {
	name := c.Param("name")
	var req upstreamToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.upstreams.Toggle(c.Request.Context(), name, *req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": *req.Enabled})
}

// Test handles POST /api/v1/upstreams/:name/test
func (h *UpstreamHandler) Test(c *gin.Context) {
	name := c.Param("name")
	result, err := h.upstreams.Test(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// TestConfig handles POST /api/v1/upstreams/test — tests raw upstream data without saving.
func (h *UpstreamHandler) TestConfig(c *gin.Context) {
	var req testUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
