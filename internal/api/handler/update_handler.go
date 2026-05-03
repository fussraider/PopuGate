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
// @Summary      Check for PopuGate updates
// @Description  Checks remote source for new PopuGate releases and returns update availability status
// @Tags         update
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /update/check [get]
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
// @Summary      Apply PopuGate update
// @Description  Downloads and installs the latest PopuGate update, then triggers a restart. In binary mode: replaces the binary. In Docker mode: pulls a new image and recreates the container.
// @Tags         update
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /update/apply [post]
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

	auditLog(c, "system.update", "system update applied")
	c.JSON(http.StatusOK, resp)

	go func() {
		time.Sleep(1 * time.Second)
		if err := h.updateSvc.RestartSelf(result.ImagePulled); err != nil {
			logger.WithScope("update").Warnf("restart after update: %v", err)
		}
	}()
}
