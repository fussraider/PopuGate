package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
// Downloads and installs the update, then triggers a restart
// in a goroutine after the response is sent.
func (h *UpdateHandler) Apply(c *gin.Context) {
	// Use a background context with timeout for updates (10 minutes)
	// to avoid being killed by request context timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := h.updateSvc.Apply(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"previous_version": result.PreviousVersion,
		"new_version":      result.NewVersion,
		"backup_path":      result.BackupPath,
		"message":          "update applied, restarting...",
	})

	// Restart in background after response is sent
	go func() {
		time.Sleep(1 * time.Second)
		_ = h.updateSvc.RestartSelf()
	}()
}
