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
