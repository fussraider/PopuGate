package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
)

// HealthHandler handles health check endpoint.
type HealthHandler struct {
	healthSvc *service.HealthService
	docker    interface {
		IsInstalled(ctx interface{}) bool
	}
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// SetHealthService sets the health service.
func (h *HealthHandler) SetHealthService(svc *service.HealthService) {
	h.healthSvc = svc
}

// Check handles GET /api/v1/health
// @Summary      Health check
// @Description  Returns the health status of the service, including version, Docker, and container info
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /health [get]
func (h *HealthHandler) Check(c *gin.Context) {
	if h.healthSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"version":     model.Version,
			"commit":      model.Commit,
			"version_url": model.VersionURL(),
		})
		return
	}

	status := h.healthSvc.Check(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"version":     model.Version,
		"commit":      model.Commit,
		"version_url": model.VersionURL(),
		"docker":      status.Docker,
		"container":   status.Container,
		"port":        status.Port,
		"metrics":     status.Metrics,
	})
}
