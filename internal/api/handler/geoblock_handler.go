package handler

import (
	"fmt"
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
// @Summary      Get geo-block settings
// @Description  Returns the current geo-blocking mode and list of blocked/allowed countries
// @Tags         geoblock
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geoblock [get]
func (h *GeoblockHandler) Get(c *gin.Context) {
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"mode":      settings.GeoblockMode,
		"countries": settings.BlocklistCountries,
	})
}

type geoRequest struct {
	Country string `json:"country" binding:"required,alpha,len=2"`
}

// Add handles POST /api/v1/geoblock/add
// @Summary      Add country to blocklist
// @Description  Adds a country (ISO 3166-1 alpha-2 code) to the geo-block list and applies iptables rules
// @Tags         geoblock
// @Accept       json
// @Produce      json
// @Param        body  body  object{country=string}  true  "Country code to add"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geoblock/add [post]
func (h *GeoblockHandler) Add(c *gin.Context) {
	var req geoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if !service.IsValidCountryCode(req.Country) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid country code, must be ISO 3166-1 alpha-2"})
		return
	}

	ctx := c.Request.Context()
	settings, _ := h.settings.Load(ctx)
	countries := settings.BlocklistCountries
	if countries != "" {
		countries += ","
	}
	countries += req.Country

	if err := h.settings.Save(ctx, map[string]string{"blocklist_countries": countries}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Apply rules
	if h.geoSvc != nil {
		if err := h.geoSvc.Apply(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to apply iptables rules: %v", err)})
			return
		}
	}

	auditLog(c, "geoblock.add", fmt.Sprintf("country=%s", req.Country))
	c.JSON(http.StatusOK, gin.H{"ok": true, "country": req.Country})
}

// Remove handles POST /api/v1/geoblock/remove
// @Summary      Remove country from blocklist
// @Description  Removes a country (ISO 3166-1 alpha-2 code) from the geo-block list and re-applies rules
// @Tags         geoblock
// @Accept       json
// @Produce      json
// @Param        body  body  object{country=string}  true  "Country code to remove"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geoblock/remove [post]
func (h *GeoblockHandler) Remove(c *gin.Context) {
	var req geoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
	if err := h.settings.Save(ctx, map[string]string{"blocklist_countries": stringsJoinComma(remaining)}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Re-apply rules
	if h.geoSvc != nil {
		if err := h.geoSvc.Apply(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to apply iptables rules: %v", err)})
			return
		}
	}

	auditLog(c, "geoblock.remove", fmt.Sprintf("country=%s", req.Country))
	c.JSON(http.StatusOK, gin.H{"ok": true, "country": req.Country})
}

// Clear handles POST /api/v1/geoblock/clear
// @Summary      Clear all geo-block rules
// @Description  Removes all countries from the geo-block list and clears iptables rules
// @Tags         geoblock
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geoblock/clear [post]
func (h *GeoblockHandler) Clear(c *gin.Context) {
	ctx := c.Request.Context()

	if h.geoSvc != nil {
		if err := h.geoSvc.Clear(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to clear iptables rules: %v", err)})
			return
		}
	}

	if err := h.settings.Save(ctx, map[string]string{"blocklist_countries": ""}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	auditLog(c, "geoblock.clear", "all countries removed")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type modeRequest struct {
	Mode string `json:"mode" binding:"required,oneof=blacklist whitelist"`
}

// SetMode handles PUT /api/v1/geoblock/mode
// @Summary      Set geo-block mode
// @Description  Sets the geo-blocking mode to either blacklist or whitelist and re-applies rules
// @Tags         geoblock
// @Accept       json
// @Produce      json
// @Param        body  body  object{mode=string}  true  "Mode to set (blacklist or whitelist)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geoblock/mode [put]
func (h *GeoblockHandler) SetMode(c *gin.Context) {
	var req modeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	ctx := c.Request.Context()
	if err := h.settings.Save(ctx, map[string]string{"geoblock_mode": req.Mode}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Re-apply rules with new mode
	if h.geoSvc != nil {
		if err := h.geoSvc.Apply(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to apply iptables rules: %v", err)})
			return
		}
	}

	auditLog(c, "geoblock.set_mode", fmt.Sprintf("mode=%s", req.Mode))
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
