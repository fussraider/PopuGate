package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/store"
)

// ConfigHandler handles configuration endpoints.
type ConfigHandler struct {
	settings *store.SettingsStore
}

// allowedConfigKeys is the whitelist of keys that may be updated via the API.
// Internal keys (jwt_secret, auth_password_hash) are intentionally excluded.
var allowedConfigKeys = map[string]bool{
	// Proxy
	"proxy_port": true, "proxy_metrics_port": true, "proxy_domain": true,
	"proxy_concurrency": true, "proxy_cpus": true, "proxy_memory": true,
	"custom_ip": true, "fake_cert_len": true, "proxy_protocol": true,
	"proxy_protocol_trusted_cidrs": true,
	// Ad tag
	"ad_tag": true,
	// Geo-blocking
	"geoblock_mode": true, "blocklist_countries": true,
	// Traffic masking
	"masking_enabled": true, "masking_host": true, "masking_port": true,
	"unknown_sni_action": true,
	// Telegram
	"telegram_enabled": true, "telegram_bot_token": true, "telegram_chat_id": true,
	"telegram_interval": true, "telegram_alerts_enabled": true, "telegram_server_label": true,
	// Auto-update
	"auto_update_enabled": true,
	// Replication
	"replication_enabled": true, "replication_role": true,
	"replication_sync_interval": true, "replication_ssh_port": true,
	"replication_ssh_user": true, "replication_delete_extra": true,
	"replication_ssh_key_path": true, "replication_exclude": true,
	"replication_restart_on_change": true, "replication_log": true,
	// System
	"debug": true,
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(settings *store.SettingsStore) *ConfigHandler {
	return &ConfigHandler{settings: settings}
}

// GetAll handles GET /api/v1/config
func (h *ConfigHandler) GetAll(c *gin.Context) {
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// Update handles PUT /api/v1/config
func (h *ConfigHandler) Update(c *gin.Context) {
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to string map, filtering to allowed keys only
	strUpdates := make(map[string]string)
	var rejected []string
	for k, v := range updates {
		if !allowedConfigKeys[k] {
			rejected = append(rejected, k)
			continue
		}
		switch val := v.(type) {
		case nil:
			continue // skip null values
		case bool:
			strUpdates[k] = strconv.FormatBool(val)
		case float64:
			strUpdates[k] = strconv.FormatInt(int64(val), 10)
		case string:
			strUpdates[k] = val
		default:
			continue // skip unsupported types
		}
	}

	if len(strUpdates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid settings provided", "rejected": rejected})
		return
	}

	if err := h.settings.Save(c.Request.Context(), strUpdates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Return updated keys
	applied := make([]string, 0, len(strUpdates))
	for k := range strUpdates {
		applied = append(applied, k)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied, "rejected": rejected})
}

// GetKey handles GET /api/v1/config/:key
func (h *ConfigHandler) GetKey(c *gin.Context) {
	key := c.Param("key")
	value, err := h.settings.Get(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}
