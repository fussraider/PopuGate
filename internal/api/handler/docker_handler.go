package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
)

// DockerHandler handles Docker/engine endpoints.
type DockerHandler struct {
	docker    *dockerutil.DockerClient
	dockerSvc *service.DockerService
	settings  *store.SettingsStore
}

// NewDockerHandler creates a new DockerHandler.
func NewDockerHandler(docker *dockerutil.DockerClient, dockerSvc *service.DockerService, settings *store.SettingsStore) *DockerHandler {
	return &DockerHandler{docker: docker, dockerSvc: dockerSvc, settings: settings}
}

// Install handles POST /api/v1/docker/install
// @Summary      Install Docker
// @Description  Ensures Docker is installed on the host system
// @Tags         docker
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /docker/install [post]
func (h *DockerHandler) Install(c *gin.Context) {
	ctx := c.Request.Context()
	if err := dockerutil.EnsureDockerInstalled(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Status handles GET /api/v1/docker/status
// @Summary      Docker status
// @Description  Returns Docker installation and running status along with server version
// @Tags         docker
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /docker/status [get]
func (h *DockerHandler) Status(c *gin.Context) {
	ctx := c.Request.Context()
	installed := h.docker.IsInstalled(ctx)

	result := gin.H{"installed": installed, "running": false}
	if installed {
		info, err := h.docker.Info(ctx)
		if err == nil {
			result["running"] = true
			result["version"] = info.ServerVersion
		}
	}

	c.JSON(http.StatusOK, result)
}

// EngineStatus handles GET /api/v1/engine/status
// @Summary      Engine status
// @Description  Returns the telemt engine image status, installed version, and whether it is up to date
// @Tags         engine
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/status [get]
func (h *DockerHandler) EngineStatus(c *gin.Context) {
	ctx := c.Request.Context()

	version := ""
	if h.dockerSvc != nil {
		version = h.dockerSvc.GetInstalledVersion()
	}

	// If version is known, check for that specific image.
	// Otherwise check for the base image (latest).
	imageRef := "popugate-telemt"
	if version != "" {
		imageRef = "popugate-telemt:" + version
	}

	hasImage, _ := h.docker.HasImage(ctx, imageRef)

	// If still not found but we have a version, maybe the tag is missing but :latest exists
	if !hasImage && version != "" {
		hasImage, _ = h.docker.HasImage(ctx, "popugate-telemt:latest")
	}

	// Fallback: if we don't have a version file, but we have some popugate-telemt image,
	// try to find what version it is.
	if version == "" {
		images, err := h.docker.ListImages(ctx, "popugate-telemt")
		if err == nil && len(images) > 0 {
			hasImage = true
			// Try to find a version-like tag
			for _, img := range images {
				for _, tag := range img.RepoTags {
					// tag is usually "popugate-telemt:3.3.39-bc69153"
					parts := strings.Split(tag, ":")
					if len(parts) == 2 && parts[1] != "latest" {
						version = parts[1]
						break
					}
				}
				if version != "" {
					break
				}
			}
			if version == "" && len(images[0].RepoTags) > 0 {
				version = "latest"
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"installed":    hasImage, // compatibility
		"image_exists": hasImage,
		"version":      version,
		"up_to_date":   true,
	})
}

// Build handles POST /api/v1/engine/build
// @Summary      Build engine image
// @Description  Builds the telemt engine Docker image from source. Supports optional force rebuild
// @Tags         engine
// @Accept       json
// @Produce      json
// @Param        body  body  object{force=bool}  false  "Build options"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /engine/build [post]
func (h *DockerHandler) Build(c *gin.Context) {
	if h.dockerSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "docker service not available"})
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req) // optional: defaults to force=false

	// Use a long timeout for engine builds (30 minutes) as they can take time from source
	// and may be interrupted by request context timeout (usually 30s in many setups).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := h.dockerSvc.BuildEngine(ctx, req.Force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, result)
}
