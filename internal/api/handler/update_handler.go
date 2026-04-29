package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fussraider/PopuGate/pkg/logger"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
)

// UpdateHandler handles auto-update endpoints.
type UpdateHandler struct {
	updateSvc *service.UpdateService
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(svc *service.UpdateService) *UpdateHandler {
	return &UpdateHandler{updateSvc: svc}
}

// Check handles GET /api/v1/update/check
func (h *UpdateHandler) Check(c *gin.Context) {
	status, err := h.updateSvc.Check(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"current":          "unknown",
			"latest":           "unknown",
			"update_available": false,
			"error":            fmt.Sprintf("failed to check: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, status)
}

// Apply handles POST /api/v1/update/apply
// In binary mode: downloads and installs the update, then triggers a restart.
// In Docker mode: pulls new image, then spawns sidecar to recreate the container.
func (h *UpdateHandler) Apply(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := h.updateSvc.Apply(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{
		"ok":               true,
		"previous_version": result.PreviousVersion,
		"new_version":      result.NewVersion,
		"message":          "update applied, restarting...",
	}
	if result.BackupPath != "" {
		resp["backup_path"] = result.BackupPath
	}
	if result.ImagePulled != "" {
		resp["image_pulled"] = result.ImagePulled
		resp["container_name"] = result.ContainerName
	}

	c.JSON(http.StatusOK, resp)

	go func() {
		time.Sleep(1 * time.Second)
		if err := h.updateSvc.RestartSelf(result.ImagePulled); err != nil {
			logger.WithScope("update").Warnf("restart after update: %v", err)
		}
	}()
}
