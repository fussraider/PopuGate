package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/netutil"
)

var secretLog = logger.WithScope("secret")

// SecretHandler handles secret endpoints.
type SecretHandler struct {
	secrets      *service.SecretService
	settings     *store.SettingsStore
	containerSvc *service.ContainerService
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(secrets *service.SecretService, settings *store.SettingsStore) *SecretHandler {
	return &SecretHandler{secrets: secrets, settings: settings}
}

// SetContainerSvc sets the container service for revalidation after secret changes.
func (h *SecretHandler) SetContainerSvc(svc *service.ContainerService) {
	h.containerSvc = svc
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
		HandleError(c, http.StatusInternalServerError, "failed to list secrets", err)
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

	secretLog.Infof("creating secret: label=%s", req.Label)
	auditLog(c, "secret.create", fmt.Sprintf("label=%s", req.Label))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s created", req.Label))
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
		HandleError(c, http.StatusInternalServerError, "failed to get secret", err)
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
	secretLog.Infof("removing secret: label=%s", label)
	auditLog(c, "secret.delete", fmt.Sprintf("label=%s", label))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s removed", label))
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
	secretLog.Infof("rotating secret: label=%s", label)
	auditLog(c, "secret.rotate", fmt.Sprintf("label=%s", label))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s rotated", label))
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
	action := "enabling"
	if !*req.Enabled {
		action = "disabling"
	}
	secretLog.Infof("%s secret: label=%s", action, label)
	auditLog(c, "secret.toggle", fmt.Sprintf("label=%s enabled=%v", label, *req.Enabled))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s toggled (%s)", label, action))
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": *req.Enabled})
}

type setLimitsRequest struct {
	MaxConns   *int   `json:"max_conns" binding:"omitempty,min=-1"`
	MaxIPs     *int   `json:"max_ips" binding:"omitempty,min=-1"`
	Quota      string `json:"quota"`                                  // Human-readable: "5G", "500M"
	QuotaBytes *int64 `json:"quota_bytes" binding:"omitempty,min=-1"` // Or raw bytes
	ExpiresAt  string `json:"expires_at"`                             // ISO 8601 or "0"
	// Per-user rate limits in bits per second (0 = unlimited, omitted = unchanged).
	RateLimitUpBps   *int64 `json:"rate_limit_up_bps" binding:"omitempty,min=-1"`
	RateLimitDownBps *int64 `json:"rate_limit_down_bps" binding:"omitempty,min=-1"`
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

	var rateUp int64 = -1
	if req.RateLimitUpBps != nil {
		rateUp = *req.RateLimitUpBps
	}
	var rateDown int64 = -1
	if req.RateLimitDownBps != nil {
		rateDown = *req.RateLimitDownBps
	}

	if err := h.secrets.SetLimits(c.Request.Context(), label, maxConns, maxIPs, quotaBytes, req.ExpiresAt, rateUp, rateDown); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("updating limits for secret: label=%s conns=%d ips=%d quota=%d exp=%s", label, maxConns, maxIPs, quotaBytes, req.ExpiresAt)
	auditLog(c, "secret.set_limits", fmt.Sprintf("label=%s", label))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s limits updated", label))
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

	result, err := h.secrets.GetLinks(c.Request.Context(), label, serverIP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
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
	auditLog(c, "secret.update_notes", fmt.Sprintf("label=%s", label))
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
	auditLog(c, "secret.reset_traffic", fmt.Sprintf("label=%s", label))
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
	auditLog(c, "secret.reset_all_traffic", "all secrets")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type renameSecretRequest struct {
	NewLabel string `json:"new_label" binding:"required,alphanumdash,max=32"`
}

// Rename handles PUT /api/v1/secrets/:label/rename
// @Summary      Rename a secret
// @Description  Change the label of an existing secret
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string               true  "Current secret label"
// @Param        body   body  renameSecretRequest  true  "New label"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/rename [put]
func (h *SecretHandler) Rename(c *gin.Context) {
	label := c.Param("label")
	var req renameSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.secrets.Rename(c.Request.Context(), label, req.NewLabel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("renaming secret: label=%s -> %s", label, req.NewLabel)
	auditLog(c, "secret.rename", fmt.Sprintf("%s -> %s", label, req.NewLabel))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s renamed to %s", label, req.NewLabel))
	c.JSON(http.StatusOK, gin.H{"ok": true, "old_label": label, "new_label": req.NewLabel})
}

type extendSecretRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

// Extend handles POST /api/v1/secrets/:label/extend
// @Summary      Extend secret expiry
// @Description  Extend a secret's expiry by the given number of days. Re-enables the secret if it was disabled.
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string                true  "Secret label"
// @Param        body   body  extendSecretRequest   true  "Days to extend"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/extend [post]
func (h *SecretHandler) Extend(c *gin.Context) {
	label := c.Param("label")
	var req extendSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.secrets.Extend(c.Request.Context(), label, req.Days); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("extending secret: label=%s days=%d", label, req.Days)
	auditLog(c, "secret.extend", fmt.Sprintf("label=%s days=%d", label, req.Days))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s extended", label))
	c.JSON(http.StatusOK, gin.H{"ok": true, "label": label, "extended_days": req.Days})
}

// DisableExpired handles POST /api/v1/secrets/disable-expired
// @Summary      Disable expired secrets
// @Description  Disable all secrets whose expiry date has passed
// @Tags         secrets
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/disable-expired [post]
func (h *SecretHandler) DisableExpired(c *gin.Context) {
	count, err := h.secrets.DisableExpired(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	auditLog(c, "secret.disable_expired", fmt.Sprintf("disabled=%d", count))
	if count > 0 {
		secretLog.Infof("disabling expired secrets (count=%d)", count)
		h.revalidateInstances(c.Request.Context(), "expired secrets disabled")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "disabled": count})
}

type setTagsRequest struct {
	Tags string `json:"tags" binding:"max=500"`
}

// SetTags handles PUT /api/v1/secrets/:label/tags
// @Summary      Set secret tags
// @Description  Set tags as JSON array for a secret
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string          true  "Secret label"
// @Param        body   body  setTagsRequest  true  "Tags"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/tags [put]
func (h *SecretHandler) SetTags(c *gin.Context) {
	label := c.Param("label")
	var req setTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}
	if req.Tags == "" {
		req.Tags = "[]"
	}
	if err := model.ValidateTags(req.Tags); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.secrets.SetTags(c.Request.Context(), label, req.Tags); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("updating tags for secret: label=%s tags=%s", label, req.Tags)
	auditLog(c, "secret.set_tags", fmt.Sprintf("label=%s tags=%s", label, req.Tags))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s tags updated", label))
	c.JSON(http.StatusOK, gin.H{"ok": true, "tags": req.Tags})
}

// Archive handles POST /api/v1/secrets/:label/archive
// @Summary      Archive a secret
// @Description  Archive a secret to hide it from normal listings
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/archive [post]
func (h *SecretHandler) Archive(c *gin.Context) {
	label := c.Param("label")
	if err := h.secrets.Archive(c.Request.Context(), label); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("archiving secret: label=%s", label)
	auditLog(c, "secret.archive", fmt.Sprintf("label=%s", label))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s archived", label))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Unarchive handles POST /api/v1/secrets/:label/unarchive
// @Summary      Unarchive a secret
// @Description  Restore an archived secret to normal listings
// @Tags         secrets
// @Produce      json
// @Param        label  path  string  true  "Secret label"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/unarchive [post]
func (h *SecretHandler) Unarchive(c *gin.Context) {
	label := c.Param("label")
	if err := h.secrets.Unarchive(c.Request.Context(), label); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("unarchiving secret: label=%s", label)
	auditLog(c, "secret.unarchive", fmt.Sprintf("label=%s", label))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s unarchived", label))
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

type cloneSecretRequest struct {
	NewLabel string `json:"new_label" binding:"required,alphanumdash,max=32"`
}

// Clone handles POST /api/v1/secrets/:label/clone
// @Summary      Clone a secret
// @Description  Create a copy of a secret with a new label and generated key
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        label  path  string              true  "Source secret label"
// @Param        body   body  cloneSecretRequest  true  "New label"
// @Success      201  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/{label}/clone [post]
func (h *SecretHandler) Clone(c *gin.Context) {
	label := c.Param("label")
	var req cloneSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	sec, err := h.secrets.Clone(c.Request.Context(), label, req.NewLabel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("cloning secret: source=%s -> new=%s", label, req.NewLabel)
	auditLog(c, "secret.clone", fmt.Sprintf("source=%s new=%s", label, req.NewLabel))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("secret %s cloned to %s", label, req.NewLabel))
	c.JSON(http.StatusCreated, sec)
}

type bulkExtendRequest struct {
	Labels []string `json:"labels" binding:"omitempty,min=1"`
	Tag    string   `json:"tag,omitempty"`
	Days   int      `json:"days" binding:"required,min=1"`
}

// BulkExtend handles POST /api/v1/secrets/bulk-extend
// @Summary      Bulk extend secrets
// @Description  Extend expiry for multiple secrets at once
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        body  body  bulkExtendRequest  true  "Labels and days"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/bulk-extend [post]
func (h *SecretHandler) BulkExtend(c *gin.Context) {
	var req bulkExtendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	labels, err := h.resolveBulkLabels(c, req.Labels, req.Tag)
	if err != nil {
		return
	}

	updated, err := h.secrets.BulkExtend(c.Request.Context(), labels, req.Days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("bulk extending %d secrets", len(labels))
	auditLog(c, "secret.bulk_extend", fmt.Sprintf("count=%d days=%d tag=%s", len(labels), req.Days, req.Tag))
	if updated > 0 {
		h.revalidateInstances(c.Request.Context(), fmt.Sprintf("bulk extend %d secrets", len(labels)))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "updated": updated})
}

type bulkRotateRequest struct {
	Labels []string `json:"labels" binding:"omitempty,min=1"`
	Tag    string   `json:"tag,omitempty"`
}

// BulkRotate handles POST /api/v1/secrets/bulk-rotate
// @Summary      Bulk rotate secrets
// @Description  Rotate keys for multiple secrets at once
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        body  body  bulkRotateRequest  true  "Labels"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/bulk-rotate [post]
func (h *SecretHandler) BulkRotate(c *gin.Context) {
	var req bulkRotateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	labels, err := h.resolveBulkLabels(c, req.Labels, req.Tag)
	if err != nil {
		return
	}

	updated, rotated, err := h.secrets.BulkRotate(c.Request.Context(), labels)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("bulk rotating %d secrets", len(labels))
	auditLog(c, "secret.bulk_rotate", fmt.Sprintf("count=%d tag=%s", len(labels), req.Tag))
	if updated > 0 {
		h.revalidateInstances(c.Request.Context(), fmt.Sprintf("bulk rotate %d secrets", len(labels)))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "updated": updated, "labels": rotated})
}

// Search handles GET /api/v1/secrets/search
// @Summary      Search secrets
// @Description  Search secrets by label or notes
// @Tags         secrets
// @Produce      json
// @Param        q  query  string  true  "Search query"
// @Success      200  {array}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/search [get]
func (h *SecretHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	secrets, err := h.secrets.Search(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, secrets)
}

// Top handles GET /api/v1/secrets/top
// @Summary      Top secrets by traffic
// @Description  Get secrets ordered by total traffic usage
// @Tags         secrets
// @Produce      json
// @Param        limit  query  int  false  "Number of results (default 10)"
// @Success      200  {array}  object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/top [get]
func (h *SecretHandler) Top(c *gin.Context) {
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	secrets, err := h.secrets.Top(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, secrets)
}

// Export handles GET /api/v1/secrets/export
// @Summary      Export secrets
// @Description  Export all secrets as JSON
// @Tags         secrets
// @Produce      json
// @Success      200  {array}  object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/export [get]
func (h *SecretHandler) Export(c *gin.Context) {
	secrets, err := h.secrets.ExportJSON(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, secrets)
}

type importSecretsRequest struct {
	Secrets []importSecretEntry `json:"secrets" binding:"required,min=1"`
}

type importSecretEntry struct {
	Label     string `json:"label" binding:"required"`
	SecretKey string `json:"secret_key"`
}

// Import handles POST /api/v1/secrets/import
// @Summary      Import secrets
// @Description  Import multiple secrets at once
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        body  body  importSecretsRequest  true  "Secrets to import"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/import [post]
func (h *SecretHandler) Import(c *gin.Context) {
	var req importSecretsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	entries := make([]model.Secret, len(req.Secrets))
	for i, e := range req.Secrets {
		entries[i] = model.Secret{Label: e.Label, SecretKey: e.SecretKey}
	}

	result, err := h.secrets.ImportSecrets(c.Request.Context(), entries)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLog(c, "secret.import", fmt.Sprintf("count=%d", len(req.Secrets)))
	if len(result.Imported) > 0 {
		secretLog.Infof("imported %d secrets", len(result.Imported))
		h.revalidateInstances(c.Request.Context(), fmt.Sprintf("imported %d secrets", len(result.Imported)))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"imported": result.Imported,
		"skipped":  result.Skipped,
		"errors":   result.Errors,
	})
}

// resolveBulkLabels resolves labels from either explicit list or tag.
// Returns error response via gin if validation fails.
func (h *SecretHandler) resolveBulkLabels(c *gin.Context, labels []string, tag string) ([]string, error) {
	if tag != "" {
		if len(labels) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide either 'labels' or 'tag', not both"})
			return nil, fmt.Errorf("both labels and tag provided")
		}
		resolved, err := h.secrets.LabelsByTag(c.Request.Context(), tag)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return nil, err
		}
		if len(resolved) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no secrets found with tag '%s'", tag)})
			return nil, fmt.Errorf("no secrets for tag")
		}
		return resolved, nil
	}
	if len(labels) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "labels or tag is required"})
		return nil, fmt.Errorf("no labels or tag")
	}
	return labels, nil
}

// ListTags handles GET /api/v1/secrets/tags
// @Summary      List all tags
// @Description  Retrieve all unique tags assigned to secrets
// @Tags         secrets
// @Produce      json
// @Success      200  {object}  map[string][]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/tags [get]
func (h *SecretHandler) ListTags(c *gin.Context) {
	tags, err := h.secrets.ListAllTags(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if tags == nil {
		tags = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// ListByTag handles GET /api/v1/secrets/by-tag/:tag
// @Summary      List secrets by tag
// @Description  Retrieve all secrets that have the specified tag
// @Tags         secrets
// @Produce      json
// @Param        tag  path  string  true  "Tag name"
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/by-tag/{tag} [get]
func (h *SecretHandler) ListByTag(c *gin.Context) {
	tag := c.Param("tag")
	secrets, err := h.secrets.ListByTag(c.Request.Context(), tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if secrets == nil {
		secrets = []model.Secret{}
	}
	c.JSON(http.StatusOK, secrets)
}

type bulkToggleRequest struct {
	Labels []string `json:"labels" binding:"omitempty,min=1"`
	Tag    string   `json:"tag,omitempty"`
	Enable bool     `json:"enable"`
}

// BulkToggle handles POST /api/v1/secrets/bulk-toggle
// @Summary      Bulk toggle secrets
// @Description  Enable or disable multiple secrets at once by labels or tag
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        body  body  bulkToggleRequest  true  "Labels or tag and enable flag"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/bulk-toggle [post]
func (h *SecretHandler) BulkToggle(c *gin.Context) {
	var req bulkToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	labels, err := h.resolveBulkLabels(c, req.Labels, req.Tag)
	if err != nil {
		return
	}

	updated, err := h.secrets.BulkToggle(c.Request.Context(), labels, req.Enable)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	action := "bulk enabling"
	if !req.Enable {
		action = "bulk disabling"
	}
	secretLog.Infof("%s %d secrets", action, len(labels))
	auditLog(c, "secret.bulk_toggle", fmt.Sprintf("count=%d enable=%v", updated, req.Enable))
	h.revalidateInstances(c.Request.Context(), fmt.Sprintf("bulk toggle %d secrets (%s)", len(labels), action))
	c.JSON(http.StatusOK, gin.H{"ok": true, "updated": updated})
}

type bulkSetLimitsRequest struct {
	Labels           []string `json:"labels" binding:"omitempty,min=1"`
	Tag              string   `json:"tag,omitempty"`
	MaxConns         *int     `json:"max_conns"`
	MaxIPs           *int     `json:"max_ips"`
	QuotaBytes       *int64   `json:"quota_bytes"`
	ExpiresAt        string   `json:"expires_at"`
	RateLimitUpBps   *int64   `json:"rate_limit_up_bps"`
	RateLimitDownBps *int64   `json:"rate_limit_down_bps"`
}

// BulkSetLimits handles POST /api/v1/secrets/bulk-set-limits
// @Summary      Bulk set secret limits
// @Description  Configure connection, IP, quota, and expiry limits for multiple secrets at once by labels or tag
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Param        body  body  bulkSetLimitsRequest  true  "Labels or tag and limits configuration"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /secrets/bulk-set-limits [post]
func (h *SecretHandler) BulkSetLimits(c *gin.Context) {
	var req bulkSetLimitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	labels, err := h.resolveBulkLabels(c, req.Labels, req.Tag)
	if err != nil {
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
	}
	var rateUp int64 = -1
	if req.RateLimitUpBps != nil {
		rateUp = *req.RateLimitUpBps
	}
	var rateDown int64 = -1
	if req.RateLimitDownBps != nil {
		rateDown = *req.RateLimitDownBps
	}

	updated, err := h.secrets.BulkSetLimits(c.Request.Context(), labels, maxConns, maxIPs, quotaBytes, req.ExpiresAt, rateUp, rateDown)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secretLog.Infof("bulk updating limits for %d secrets", len(labels))
	auditLog(c, "secret.bulk_set_limits", fmt.Sprintf("count=%d", updated))
	if updated > 0 {
		h.revalidateInstances(c.Request.Context(), fmt.Sprintf("bulk limits updated for %d secrets", len(labels)))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "updated": updated})
}

// revalidateInstances rechecks running instances after a secret change.
func (h *SecretHandler) revalidateInstances(ctx context.Context, reason string) {
	if h.containerSvc != nil {
		h.containerSvc.RevalidateAllInstances(ctx)
		if err := h.containerSvc.Reload(ctx, reason); err != nil {
			secretLog.Warnf("revalidateInstances: failed to hot-reload instances: %v", err)
		}
	}
}
