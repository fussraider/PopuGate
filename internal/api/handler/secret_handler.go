package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/netutil"
)

// SecretHandler handles secret endpoints.
type SecretHandler struct {
	secrets  *service.SecretService
	settings *store.SettingsStore
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(secrets *service.SecretService, settings *store.SettingsStore) *SecretHandler {
	return &SecretHandler{secrets: secrets, settings: settings}
}

// List handles GET /api/v1/secrets
func (h *SecretHandler) List(c *gin.Context) {
	secrets, err := h.secrets.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, secrets)
}

type addSecretRequest struct {
	Label  string `json:"label" binding:"required"`
	Secret string `json:"secret"` // Optional, auto-generated if empty
}

// Add handles POST /api/v1/secrets
func (h *SecretHandler) Add(c *gin.Context) {
	var req addSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sec, err := h.secrets.Add(c.Request.Context(), req.Label, req.Secret)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, sec)
}

// Get handles GET /api/v1/secrets/:label
func (h *SecretHandler) Get(c *gin.Context) {
	label := c.Param("label")
	sec, err := h.secrets.Get(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sec == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	c.JSON(http.StatusOK, sec)
}

// Remove handles DELETE /api/v1/secrets/:label
func (h *SecretHandler) Remove(c *gin.Context) {
	label := c.Param("label")
	force := c.Query("force") == "true"

	if err := h.secrets.Remove(c.Request.Context(), label, force); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Rotate handles POST /api/v1/secrets/:label/rotate
func (h *SecretHandler) Rotate(c *gin.Context) {
	label := c.Param("label")
	sec, err := h.secrets.Rotate(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sec)
}

type secretToggleRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// Toggle handles PUT /api/v1/secrets/:label/toggle
func (h *SecretHandler) Toggle(c *gin.Context) {
	label := c.Param("label")
	var req secretToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.secrets.Toggle(c.Request.Context(), label, *req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": *req.Enabled})
}

type setLimitsRequest struct {
	MaxConns   *int   `json:"max_conns"`
	MaxIPs     *int   `json:"max_ips"`
	Quota      string `json:"quota"`       // Human-readable: "5G", "500M"
	QuotaBytes *int64 `json:"quota_bytes"` // Or raw bytes
	ExpiresAt  string `json:"expires_at"`  // ISO 8601 or "0"
}

// SetLimits handles PUT /api/v1/secrets/:label/limits
func (h *SecretHandler) SetLimits(c *gin.Context) {
	label := c.Param("label")
	var req setLimitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	maxConns := -1
	if req.MaxConns != nil {
		maxConns = *req.MaxConns
	}
	maxIPs := -1
	if req.MaxIPs != nil {
		maxIPs = *req.MaxIPs
	}
	var quotaBytes int64 = -1
	if req.QuotaBytes != nil {
		quotaBytes = *req.QuotaBytes
	} else if req.Quota != "" {
		quotaBytes = parseHumanBytes(req.Quota)
		if quotaBytes < 0 {
			quotaBytes = -1
		}
	}

	if err := h.secrets.SetLimits(c.Request.Context(), label, maxConns, maxIPs, quotaBytes, req.ExpiresAt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetLimits handles GET /api/v1/secrets/:label/limits
func (h *SecretHandler) GetLimits(c *gin.Context) {
	label := c.Param("label")
	sec, err := h.secrets.Get(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sec == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"label":       sec.Label,
		"max_conns":   sec.MaxConns,
		"max_ips":     sec.MaxIPs,
		"quota_bytes": sec.QuotaBytes,
		"expires_at":  sec.ExpiresAt,
	})
}

// GetLink handles GET /api/v1/secrets/:label/link
func (h *SecretHandler) GetLink(c *gin.Context) {
	label := c.Param("label")
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	serverIP := settings.CustomIP
	if serverIP == "" {
		if ip, err := netutil.GetPublicIP(); err == nil {
			serverIP = ip
		} else {
			serverIP = "YOUR_SERVER_IP"
		}
	}

	link, err := h.secrets.GetLink(c.Request.Context(), label, serverIP, settings.ProxyPort, settings.MaskingEnabled, settings.ProxyDomain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, link)
}

// GetQR handles GET /api/v1/secrets/:label/qr
// Returns a PNG image of the QR code for the proxy link.
// Optional query param: ?size=512 (default 256)
func (h *SecretHandler) GetQR(c *gin.Context) {
	label := c.Param("label")
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	serverIP := settings.CustomIP
	if serverIP == "" {
		if ip, err := netutil.GetPublicIP(); err == nil {
			serverIP = ip
		} else {
			serverIP = "YOUR_SERVER_IP"
		}
	}

	size := 256
	if sizeStr := c.Query("size"); sizeStr != "" {
		if parsed, err := strconv.Atoi(sizeStr); err == nil && parsed >= 64 && parsed <= 2048 {
			size = parsed
		}
	}

	pngBytes, err := h.secrets.GetQRCode(c.Request.Context(), label, serverIP, settings.ProxyPort, settings.MaskingEnabled, settings.ProxyDomain, size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Data(http.StatusOK, "image/png", pngBytes)
}

type updateNotesRequest struct {
	Notes string `json:"notes"`
}

// UpdateNotes handles PUT /api/v1/secrets/:label/notes
func (h *SecretHandler) UpdateNotes(c *gin.Context) {
	label := c.Param("label")
	var req updateNotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.secrets.UpdateNotes(c.Request.Context(), label, req.Notes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "notes": req.Notes})
}

// ResetTraffic handles POST /api/v1/secrets/:label/reset-traffic
func (h *SecretHandler) ResetTraffic(c *gin.Context) {
	label := c.Param("label")
	if err := h.secrets.ResetTraffic(c.Request.Context(), label); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ResetAllTraffic handles POST /api/v1/secrets/reset-traffic
func (h *SecretHandler) ResetAllTraffic(c *gin.Context) {
	if err := h.secrets.ResetAllTraffic(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func parseHumanBytes(s string) int64 {
	if s == "" || s == "0" {
		return 0
	}
	var amount int64
	var unit string
	fmt.Sscanf(s, "%d%s", &amount, &unit)
	switch unit {
	case "G", "g":
		return amount * 1024 * 1024 * 1024
	case "M", "m":
		return amount * 1024 * 1024
	case "K", "k":
		return amount * 1024
	}
	return amount
}
