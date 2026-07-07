package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
)

// ConfigHandler handles configuration endpoints.
type ConfigHandler struct {
	settings     *store.SettingsStore
	containerSvc *service.ContainerService
	upstreams    *store.UpstreamStore
}

var configLog = logger.WithScope("config")

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
	// Middle-Proxy mode
	"use_middle_proxy": true,
	// Geo-blocking
	"geoblock_mode": true, "blocklist_countries": true,
	// Traffic masking
	"masking_enabled": true, "masking_host": true, "masking_port": true,
	"masking_relay_max_bytes": true,
	"unknown_sni_action":      true,
	// Custom Telegram URLs (restricted regions)
	"proxy_secret_url": true, "proxy_config_v4_url": true, "proxy_config_v6_url": true,
	// Telegram
	"telegram_enabled": true, "telegram_bot_token": true, "telegram_chat_id": true,
	"telegram_interval": true, "telegram_alerts_enabled": true, "telegram_server_label": true,
	"web_url": true,
	// Auto-update
	"auto_update_enabled":     true,
	"secret_auto_rotate_days": true,
	// Maintenance
	"maintenance_mode": true,
	// Replication
	"replication_enabled": true, "replication_role": true,
	"replication_sync_interval": true, "replication_ssh_port": true,
	"replication_ssh_user": true, "replication_delete_extra": true,
	"replication_ssh_key_path": true, "replication_exclude": true,
	"replication_restart_on_change": true, "replication_log": true,
	// System
	"debug": true,
	// SYN limiter
	"synlimit_enabled": true, "synlimit_backend": true,
	"synlimit_seconds": true, "synlimit_hitcount": true, "synlimit_burst": true,
	// Backup
	"backup_retention_days": true,
	// telemt engine
	"telemt_version": true, "telemt_commit": true, "telemt_repo": true,
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(settings *store.SettingsStore) *ConfigHandler {
	return &ConfigHandler{settings: settings}
}

// SetContainerSvc sets the container service.
func (h *ConfigHandler) SetContainerSvc(svc *service.ContainerService) {
	h.containerSvc = svc
}

// SetUpstreams sets the upstream store (used to guard the use_middle_proxy ↔ shadowsocks conflict).
func (h *ConfigHandler) SetUpstreams(upstreams *store.UpstreamStore) {
	h.upstreams = upstreams
}

// GetAll handles GET /api/v1/config
// @Summary      Get all settings
// @Description  Returns all application settings
// @Tags         config
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /config [get]
func (h *ConfigHandler) GetAll(c *gin.Context) {
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		HandleError(c, http.StatusInternalServerError, "failed to load settings", err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

// Update handles PUT /api/v1/config
// @Summary      Update settings
// @Description  Updates application settings. Only whitelisted keys are accepted; internal keys (jwt_secret, auth_password_hash) are rejected.
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        body  body  map[string]any  true  "Settings key-value pairs to update"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /config [put]
func (h *ConfigHandler) Update(c *gin.Context) {
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	strUpdates, rejected := filterAllowedConfigUpdates(updates)

	if len(strUpdates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid settings provided", "rejected": rejected})
		return
	}

	if !h.middleProxyGuardPasses(c, strUpdates) {
		return
	}

	// Snapshot current settings before the write so we can tell whether the
	// SYN-limiter config actually changed. The Web UI PUTs the whole settings
	// object on every save, so key-presence alone is not a reliable signal.
	prev, _ := h.settings.Load(c.Request.Context())

	if err := h.settings.Save(c.Request.Context(), strUpdates); err != nil {
		HandleError(c, http.StatusInternalServerError, "failed to save settings", err)
		return
	}

	// Return updated keys
	applied := make([]string, 0, len(strUpdates))
	for k := range strUpdates {
		applied = append(applied, k)
	}

	configLog.Infof("updating settings: %v", applied)
	auditLog(c, "settings.update", "updated settings")
	if h.containerSvc != nil {
		// SYN-limiter changes toggle a container capability (CAP_NET_ADMIN),
		// which can only be applied at container create time — a SIGHUP
		// hot-reload cannot pick it up. Recreate the containers instead.
		if synlimitChanged(prev, strUpdates) {
			if err := h.containerSvc.Restart(c.Request.Context()); err != nil {
				configLog.Warnf("failed to recreate instances after SYN-limiter change: %v", err)
			}
		} else if err := h.containerSvc.Reload(c.Request.Context(), "settings updated"); err != nil {
			configLog.Warnf("failed to hot-reload instances: %v", err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied, "rejected": rejected})
}

// synlimitChanged reports whether the update set actually changes any
// SYN-limiter setting versus the previous state. Such a change requires
// recreating containers (capability change), not a SIGHUP hot-reload. The Web
// UI PUTs the full settings object on every save, so we compare values rather
// than relying on key presence. A nil prev (load failed) is treated as changed
// so a real toggle is never missed.
func synlimitChanged(prev *model.Settings, strUpdates map[string]string) bool {
	if prev == nil {
		for k := range strUpdates {
			if strings.HasPrefix(k, "synlimit_") {
				return true
			}
		}
		return false
	}
	cur := map[string]string{
		"synlimit_enabled":  strconv.FormatBool(prev.SynlimitEnabled),
		"synlimit_backend":  prev.SynlimitBackend,
		"synlimit_seconds":  strconv.Itoa(prev.SynlimitSeconds),
		"synlimit_hitcount": strconv.Itoa(prev.SynlimitHitcount),
		"synlimit_burst":    strconv.Itoa(prev.SynlimitBurst),
	}
	for k, curVal := range cur {
		if newVal, ok := strUpdates[k]; ok && newVal != curVal {
			return true
		}
	}
	return false
}

// filterAllowedConfigUpdates converts the raw JSON map to a string map,
// keeping only whitelisted keys and supported value types.
func filterAllowedConfigUpdates(updates map[string]any) (strUpdates map[string]string, rejected []string) {
	strUpdates = make(map[string]string)
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
			strUpdates[k] = strconv.FormatFloat(val, 'f', -1, 64)
		case string:
			strUpdates[k] = val
		default:
			continue // skip unsupported types
		}
	}
	return strUpdates, rejected
}

// middleProxyGuardPasses enforces the ADR-001 symmetric guard: refuse enabling
// Middle-Proxy mode while an enabled shadowsocks upstream exists, because
// telemt rejects shadowsocks upstreams in ME mode (which would break the whole
// engine config, not just that upstream). Writes the HTTP error response and
// returns false when the update must be rejected.
func (h *ConfigHandler) middleProxyGuardPasses(c *gin.Context, strUpdates map[string]string) bool {
	if strUpdates["use_middle_proxy"] != "true" || h.upstreams == nil {
		return true
	}
	ups, err := h.upstreams.ListEnabled(c.Request.Context())
	if err != nil {
		HandleError(c, http.StatusInternalServerError, "failed to check upstreams", err)
		return false
	}
	for _, u := range ups {
		if u.Type == model.UpstreamShadowsocks {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot enable Middle-Proxy mode while shadowsocks upstream '" + u.Name + "' is enabled; disable it first"})
			return false
		}
	}
	return true
}

// sensitiveKeys are settings that must never be exposed via the API.
var sensitiveKeys = map[string]bool{
	"jwt_secret":         true,
	"auth_password_hash": true,
}

// GetKey handles GET /api/v1/config/:key
// @Summary      Get a single setting
// @Description  Returns the value of a specific setting by key name
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        key  path  string  true  "Setting key"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /config/{key} [get]
func (h *ConfigHandler) GetKey(c *gin.Context) {
	key := c.Param("key")
	if sensitiveKeys[key] {
		HandleError(c, http.StatusForbidden, "access denied for this key", nil)
		return
	}
	value, err := h.settings.Get(c.Request.Context(), key)
	if err != nil {
		HandleError(c, http.StatusInternalServerError, "failed to get setting", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}
