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
func (h *SystemHandler) GetOS(c *gin.Context) {
	c.JSON(http.StatusOK, service.DetectOS())
}

// InstallService handles POST /api/v1/system/service/install
func (h *SystemHandler) InstallService(c *gin.Context) {
	if err := service.InstallSystemdService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "systemd service installed and enabled"})
}

// UninstallService handles DELETE /api/v1/system/service/uninstall
func (h *SystemHandler) UninstallService(c *gin.Context) {
	if err := service.UninstallSystemdService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ServiceStatus handles GET /api/v1/system/service/status
func (h *SystemHandler) ServiceStatus(c *gin.Context) {
	status := service.GetServiceStatus()
	c.JSON(http.StatusOK, status)
}

// RestartService handles POST /api/v1/system/service/restart
func (h *SystemHandler) RestartService(c *gin.Context) {
	if err := service.RestartService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "service restarting"})
}

// ReloadService handles POST /api/v1/system/service/reload
func (h *SystemHandler) ReloadService(c *gin.Context) {
	if err := service.ReloadService(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "service reload signaled"})
}
