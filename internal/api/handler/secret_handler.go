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
// @Summary      List secrets
// @Description  Retrieve all registered secrets
// @Tags         secrets
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets [get]
func (h *SecretHandler) List(c *gin.Context) {
	secrets, err := h.secrets.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, secrets)
}

type addSecretRequest struct {
	Label  string `json:"label" binding:"required,alphanumdash,max=32"`
	Secret string `json:"secret" binding:"omitempty,hexadecimal,len=32"`
}

// Add handles POST /api/v1/secrets
// @Summary      Add a secret
// @Description  Create a new secret with a unique label and optional hex secret value
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        body  body  addSecretRequest  true  "Secret to add"
// @Success      201  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets [post]
func (h *SecretHandler) Add(c *gin.Context) {
	var req addSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
// @Summary      Get a secret
// @Description  Retrieve a single secret by its label
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  object
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label} [get]
func (h *SecretHandler) Get(c *gin.Context) {
	label := c.Param("label")
	sec, err := h.secrets.Get(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if sec == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	c.JSON(http.StatusOK, sec)
}

// Remove handles DELETE /api/v1/secrets/:label
// @Summary      Remove a secret
// @Description  Delete a secret by its label. Use force=true to remove even if in use.
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Param        force  query  bool   false  "Force removal"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label} [delete]
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
// @Summary      Rotate a secret
// @Description  Generate a new secret value for the given label
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/rotate [post]
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
// @Summary      Toggle a secret
// @Description  Enable or disable a secret by label
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string              true  "Secret label"
// @Param        body   body  secretToggleRequest  true  "Enabled state"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/toggle [put]
func (h *SecretHandler) Toggle(c *gin.Context) {
	label := c.Param("label")
	var req secretToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.secrets.Toggle(c.Request.Context(), label, *req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": *req.Enabled})
}

type setLimitsRequest struct {
	MaxConns   *int   `json:"max_conns" binding:"omitempty,min=-1"`
	MaxIPs     *int   `json:"max_ips" binding:"omitempty,min=-1"`
	Quota      string `json:"quota"`                                  // Human-readable: "5G", "500M"
	QuotaBytes *int64 `json:"quota_bytes" binding:"omitempty,min=-1"` // Or raw bytes
	ExpiresAt  string `json:"expires_at"`                             // ISO 8601 or "0"
}

// SetLimits handles PUT /api/v1/secrets/:label/limits
// @Summary      Set secret limits
// @Description  Configure connection, IP, quota, and expiry limits for a secret
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string             true  "Secret label"
// @Param        body   body  setLimitsRequest   true  "Limits configuration"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/limits [put]
func (h *SecretHandler) SetLimits(c *gin.Context) {
	label := c.Param("label")
	var req setLimitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
// @Summary      Get secret limits
// @Description  Retrieve the current limits (connections, IPs, quota, expiry) for a secret
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/limits [get]
func (h *SecretHandler) GetLimits(c *gin.Context) {
	label := c.Param("label")
	sec, err := h.secrets.Get(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
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
// @Summary      Get proxy link
// @Description  Generate the Telegram proxy link for a given secret label
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/link [get]
func (h *SecretHandler) GetLink(c *gin.Context) {
	label := c.Param("label")
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
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
// @Summary      Get QR code
// @Description  Generate a PNG QR code image for the proxy link associated with the secret
// @Tags         secrets
// @Produce      image/png
// @Param        label  path  string  true  "Secret label"
// @Param        size   query  int    false  "QR code image size in pixels (64-2048, default 256)"
// @Success      200  {file}  binary
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/qr [get]
func (h *SecretHandler) GetQR(c *gin.Context) {
	label := c.Param("label")
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
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
	Notes string `json:"notes" binding:"max=500"`
}

// UpdateNotes handles PUT /api/v1/secrets/:label/notes
// @Summary      Update secret notes
// @Description  Set or update the notes field for a secret
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string              true  "Secret label"
// @Param        body   body  updateNotesRequest   true  "Notes content"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/notes [put]
func (h *SecretHandler) UpdateNotes(c *gin.Context) {
	label := c.Param("label")
	var req updateNotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.secrets.UpdateNotes(c.Request.Context(), label, req.Notes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "notes": req.Notes})
}

// ResetTraffic handles POST /api/v1/secrets/:label/reset-traffic
// @Summary      Reset secret traffic
// @Description  Reset traffic counters for a specific secret
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/reset-traffic [post]
func (h *SecretHandler) ResetTraffic(c *gin.Context) {
	label := c.Param("label")
	if err := h.secrets.ResetTraffic(c.Request.Context(), label); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ResetAllTraffic handles POST /api/v1/secrets/reset-traffic
// @Summary      Reset all traffic
// @Description  Reset traffic counters for all secrets
// @Tags         secrets
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/reset-traffic [post]
func (h *SecretHandler) ResetAllTraffic(c *gin.Context) {
	if err := h.secrets.ResetAllTraffic(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
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
	n, _ := fmt.Sscanf(s, "%d%s", &amount, &unit)
	if n < 1 || amount < 0 {
		return -1
	}
	switch unit {
	case "G", "g":
		return amount * 1024 * 1024 * 1024
	case "M", "m":
		return amount * 1024 * 1024
	case "K", "k":
		return amount * 1024
	case "":
		return amount
	}
	return -1 // unknown unit
}
