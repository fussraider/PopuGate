package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var dockerLog = logger.WithScope("docker")

// DockerHandler handles Docker/engine endpoints.
type DockerHandler struct {
	docker          *dockerutil.DockerClient
	dockerSvc       *service.DockerService
	settings        *store.SettingsStore
	dockerUpdateSvc *service.DockerUpdateService
}

// NewDockerHandler creates a new DockerHandler.
func NewDockerHandler(docker *dockerutil.DockerClient, dockerSvc *service.DockerService, settings *store.SettingsStore) *DockerHandler {
	return &DockerHandler{docker: docker, dockerSvc: dockerSvc, settings: settings}
}

// SetDockerUpdateService sets the docker update service.
func (h *DockerHandler) SetDockerUpdateService(s *service.DockerUpdateService) {
	h.dockerUpdateSvc = s
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to install docker: %v", err)})
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

	hasImage, version := h.resolveImageVersion(ctx, version)

	c.JSON(http.StatusOK, gin.H{
		"installed":    hasImage,
		"image_exists": hasImage,
		"version":      version,
		"up_to_date":   true,
	})
}

func (h *DockerHandler) resolveImageVersion(ctx context.Context, version string) (bool, string) {
	imageRef := "popugate-telemt"
	if version != "" {
		imageRef = "popugate-telemt:" + version
	}

	hasImage, imgErr := h.docker.HasImage(ctx, imageRef)
	if imgErr != nil {
		dockerLog.Warnf("check image %s: %v", imageRef, imgErr)
	}

	if !hasImage && version != "" {
		hasImage, imgErr = h.docker.HasImage(ctx, "popugate-telemt:latest")
		if imgErr != nil {
			dockerLog.Warnf("check image fallback: %v", imgErr)
		}
	}

	if version == "" {
		if v, found := h.detectVersionFromImages(ctx); found {
			return true, v
		}
	}

	return hasImage, version
}

func (h *DockerHandler) detectVersionFromImages(ctx context.Context) (string, bool) {
	images, err := h.docker.ListImages(ctx, "popugate-telemt")
	if err != nil || len(images) == 0 {
		return "", false
	}
	for _, img := range images {
		for _, tag := range img.RepoTags {
			parts := strings.Split(tag, ":")
			if len(parts) == 2 && parts[1] != "latest" {
				return parts[1], true
			}
		}
	}
	if len(images[0].RepoTags) > 0 {
		return "latest", true
	}
	return "", false
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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker service not available"})
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req) // optional: defaults to force=false

	// Use a long timeout for engine builds (30 minutes) as they can take time from source.
	// Use context.Background() so client disconnect does not cancel the build.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := h.dockerSvc.BuildEngine(ctx, req.Force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to build engine: %v", err)})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CheckUpdate handles GET /api/v1/docker/update/status
// @Summary      Host Docker Engine update status
// @Description  Returns current host Docker version, latest available version, and update status
// @Tags         docker
// @Accept       json
// @Produce      json
// @Success      200  {object}  service.DockerUpdateStatus
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /docker/update/status [get]
func (h *DockerHandler) CheckUpdate(c *gin.Context) {
	if h.dockerUpdateSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker update service not available"})
		return
	}
	status, err := h.dockerUpdateSvc.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get update status: %v", err)})
		return
	}
	c.JSON(http.StatusOK, status)
}

// TriggerCheckUpdate handles POST /api/v1/docker/update/check
// @Summary      Check for host Docker updates
// @Description  Checks remote repositories for newer versions of host Docker daemon
// @Tags         docker
// @Accept       json
// @Produce      json
// @Success      200  {object}  service.DockerUpdateStatus
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /docker/update/check [post]
func (h *DockerHandler) TriggerCheckUpdate(c *gin.Context) {
	if h.dockerUpdateSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker update service not available"})
		return
	}
	status, err := h.dockerUpdateSvc.CheckRemote(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to check update: %v", err)})
		return
	}
	c.JSON(http.StatusOK, status)
}

// ApplyUpdate handles POST /api/v1/docker/update/apply
// @Summary      Apply host Docker update
// @Description  Triggers asynchronous host Docker package upgrade and proxy container state restoration
// @Tags         docker
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /docker/update/apply [post]
func (h *DockerHandler) ApplyUpdate(c *gin.Context) {
	if h.dockerUpdateSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker update service not available"})
		return
	}
	err := h.dockerUpdateSvc.Apply(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to apply update: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
