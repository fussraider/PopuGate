package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
)

// TelemtUpdateHandler handles telemt engine update endpoints.
type TelemtUpdateHandler struct {
	telemtUpdateSvc *service.TelemtUpdateService
	telemtCfg       *service.DBTelemtConfig
	dockerSvc       *service.DockerService
}

// NewTelemtUpdateHandler creates a new TelemtUpdateHandler.
func NewTelemtUpdateHandler(
	telemtUpdateSvc *service.TelemtUpdateService,
	telemtCfg *service.DBTelemtConfig,
	dockerSvc *service.DockerService,
) *TelemtUpdateHandler {
	return &TelemtUpdateHandler{
		telemtUpdateSvc: telemtUpdateSvc,
		telemtCfg:       telemtCfg,
		dockerSvc:       dockerSvc,
	}
}

// GetStatus handles GET /api/v1/engine/update
func (h *TelemtUpdateHandler) GetStatus(c *gin.Context) {
	status, err := h.telemtUpdateSvc.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	resp := gin.H{
		"current":          status.Current,
		"latest":           status.Latest,
		"update_available": status.UpdateAvailable,
		"last_checked":     status.LastChecked,
		"updating":         status.Updating,
		"updating_to":      status.UpdatingTo,
	}

	if h.dockerSvc != nil {
		resp["installed_version"] = h.dockerSvc.GetInstalledVersion()
	}

	c.JSON(http.StatusOK, resp)
}

// CheckRemote handles POST /api/v1/engine/check
func (h *TelemtUpdateHandler) CheckRemote(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	_, err := h.telemtUpdateSvc.CheckRemote(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	status, _ := h.telemtUpdateSvc.GetStatus(ctx)
	c.JSON(http.StatusOK, status)
}

// GetReleases handles GET /api/v1/engine/releases
func (h *TelemtUpdateHandler) GetReleases(c *gin.Context) {
	releases, err := h.telemtUpdateSvc.GetReleases(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if releases == nil {
		releases = []service.TelemtReleaseListItem{}
	}
	c.JSON(http.StatusOK, releases)
}

// Apply handles POST /api/v1/engine/update
func (h *TelemtUpdateHandler) Apply(c *gin.Context) {
	var req struct {
		Version string `json:"version" binding:"required"`
		Commit  string `json:"commit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	if err := h.telemtUpdateSvc.Apply(ctx, req.Version, req.Commit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "engine updated successfully",
		"version": req.Version,
		"commit":  req.Commit,
	})
}
