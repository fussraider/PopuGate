package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
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
// @Summary      Engine update status
// @Description  Returns the current telemt engine version, latest available version, and update availability
// @Tags         engine
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/update [get]
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
// @Summary      Check for engine updates
// @Description  Checks remote source for new telemt engine releases and returns updated status
// @Tags         engine
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      502  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/check [post]
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
// @Summary      List engine releases
// @Description  Returns a list of available telemt engine releases from the remote source
// @Tags         engine
// @Accept       json
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/releases [get]
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
// @Summary      Apply engine update
// @Description  Applies a telemt engine update by downloading and installing the specified version
// @Tags         engine
// @Accept       json
// @Produce      json
// @Param        body     body  object{version=string,commit=string}  true  "Version to apply"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/update [post]
func (h *TelemtUpdateHandler) Apply(c *gin.Context) {
	var req struct {
		Version string `json:"version" binding:"required"`
		Commit  string `json:"commit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.telemtUpdateSvc.Apply(c.Request.Context(), req.Version, req.Commit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	auditLog(c, "engine.update.start", fmt.Sprintf("version=%s", req.Version))
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "engine update started",
		"version": req.Version,
		"commit":  req.Commit,
	})
}

// Cancel handles POST /api/v1/engine/update/cancel
// @Summary      Cancel engine update
// @Description  Cancels a running telemt engine update build process
// @Tags         engine
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/update/cancel [post]
func (h *TelemtUpdateHandler) Cancel(c *gin.Context) {
	if err := h.telemtUpdateSvc.Cancel(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLog(c, "engine.update.cancel", "")
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "engine update cancelled",
	})
}

// GetBuildLogs handles GET /api/v1/engine/update/logs
// @Summary      Get engine build logs
// @Description  Returns the build log contents of the telemt engine build process
// @Tags         engine
// @Produce      text/plain
// @Success      200  {string}  string
// @Security     BearerAuth
// @Router       /engine/update/logs [get]
func (h *TelemtUpdateHandler) GetBuildLogs(c *gin.Context) {
	logPath := model.TelemtBuildLogPath()

	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		c.String(http.StatusOK, "")
		return
	}

	c.File(logPath)
}
