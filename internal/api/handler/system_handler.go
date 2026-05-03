package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
)

// SystemHandler handles system-level endpoints.
type SystemHandler struct{}

// NewSystemHandler creates a new SystemHandler.
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// GetOS handles GET /api/v1/system/os
// @Summary      Detect operating system
// @Description  Returns detected OS information including distribution, version, and architecture
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /system/os [get]
func (h *SystemHandler) GetOS(c *gin.Context) {
	c.JSON(http.StatusOK, service.DetectOS())
}

// InstallService handles POST /api/v1/system/service/install
// @Summary      Install systemd service
// @Description  Installs and enables the PopuGate systemd service unit
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /system/service/install [post]
func (h *SystemHandler) InstallService(c *gin.Context) {
	if err := service.InstallSystemdService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "systemd service installed and enabled"})
}

// UninstallService handles DELETE /api/v1/system/service/uninstall
// @Summary      Uninstall systemd service
// @Description  Stops and removes the PopuGate systemd service unit
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /system/service/uninstall [delete]
func (h *SystemHandler) UninstallService(c *gin.Context) {
	if err := service.UninstallSystemdService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ServiceStatus handles GET /api/v1/system/service/status
// @Summary      Service status
// @Description  Returns the current systemd service status (running, stopped, etc.)
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /system/service/status [get]
func (h *SystemHandler) ServiceStatus(c *gin.Context) {
	status := service.GetServiceStatus()
	c.JSON(http.StatusOK, status)
}

// RestartService handles POST /api/v1/system/service/restart
// @Summary      Restart service
// @Description  Restarts the PopuGate systemd service
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /system/service/restart [post]
func (h *SystemHandler) RestartService(c *gin.Context) {
	if err := service.RestartService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "service restarting"})
}

// ReloadService handles POST /api/v1/system/service/reload
// @Summary      Reload service
// @Description  Sends a reload signal to the PopuGate systemd service
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /system/service/reload [post]
func (h *SystemHandler) ReloadService(c *gin.Context) {
	if err := service.ReloadService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "service reload signaled"})
}
