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

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(settings *store.SettingsStore) *ConfigHandler {
	return &ConfigHandler{settings: settings}
}

// GetAll handles GET /api/v1/config
func (h *ConfigHandler) GetAll(c *gin.Context) {
	settings, err := h.settings.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// Update handles PUT /api/v1/config
func (h *ConfigHandler) Update(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to string map for storage
	strUpdates := make(map[string]string)
	for k, v := range updates {
		switch val := v.(type) {
		case bool:
			strUpdates[k] = strconv.FormatBool(val)
		case float64:
			strUpdates[k] = strconv.FormatInt(int64(val), 10)
		case string:
			strUpdates[k] = val
		default:
			strUpdates[k] = ""
		}
	}

	if err := h.settings.Save(c.Request.Context(), strUpdates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated keys
	applied := make([]string, 0, len(strUpdates))
	for k := range strUpdates {
		applied = append(applied, k)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied})
}

// GetKey handles GET /api/v1/config/:key
func (h *ConfigHandler) GetKey(c *gin.Context) {
	key := c.Param("key")
	value, err := h.settings.Get(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}
