package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
)

// GeoblockHandler handles geo-blocking endpoints.
type GeoblockHandler struct {
	settings *store.SettingsStore
	geoSvc   *service.GeoblockService
}

// NewGeoblockHandler creates a new GeoblockHandler.
func NewGeoblockHandler(settings *store.SettingsStore, geoSvc *service.GeoblockService) *GeoblockHandler {
	return &GeoblockHandler{settings: settings, geoSvc: geoSvc}
}

// Get handles GET /api/v1/geoblock
func (h *GeoblockHandler) Get(c *gin.Context) {
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"mode":      settings.GeoblockMode,
		"countries": settings.BlocklistCountries,
	})
}

type geoRequest struct {
	Country string `json:"country" binding:"required"`
}

// Add handles POST /api/v1/geoblock/add
func (h *GeoblockHandler) Add(c *gin.Context) {
	var req geoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	settings, _ := h.settings.Load(ctx)
	countries := settings.BlocklistCountries
	if countries != "" {
		countries += ","
	}
	countries += req.Country

	_ = h.settings.Save(ctx, map[string]string{"blocklist_countries": countries})

	// Apply rules
	if h.geoSvc != nil {
		if err := h.geoSvc.Apply(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "country": req.Country})
}

// Remove handles POST /api/v1/geoblock/remove
func (h *GeoblockHandler) Remove(c *gin.Context) {
	var req geoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	settings, _ := h.settings.Load(ctx)
	var remaining []string
	for _, c := range splitCountries(settings.BlocklistCountries) {
		if c != req.Country {
			remaining = append(remaining, c)
		}
	}
	_ = h.settings.Save(ctx, map[string]string{"blocklist_countries": stringsJoinComma(remaining)})

	// Re-apply rules
	if h.geoSvc != nil {
		if err := h.geoSvc.Apply(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "country": req.Country})
}

// Clear handles POST /api/v1/geoblock/clear
func (h *GeoblockHandler) Clear(c *gin.Context) {
	ctx := c.Request.Context()

	if h.geoSvc != nil {
		_ = h.geoSvc.Clear(ctx)
	}

	_ = h.settings.Save(ctx, map[string]string{"blocklist_countries": ""})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type modeRequest struct {
	Mode string `json:"mode" binding:"required"`
}

// SetMode handles PUT /api/v1/geoblock/mode
func (h *GeoblockHandler) SetMode(c *gin.Context) {
	var req modeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Mode != "blacklist" && req.Mode != "whitelist" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'blacklist' or 'whitelist'"})
		return
	}

	ctx := c.Request.Context()
	_ = h.settings.Save(ctx, map[string]string{"geoblock_mode": req.Mode})

	// Re-apply rules with new mode
	if h.geoSvc != nil {
		_ = h.geoSvc.Apply(ctx)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "mode": req.Mode})
}

func splitCountries(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			result = append(result, c)
		}
	}
	return result
}

func stringsJoinComma(parts []string) string {
	return strings.Join(parts, ",")
}
