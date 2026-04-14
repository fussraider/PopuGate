package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
)

// InstanceHandler handles instance endpoints.
type InstanceHandler struct {
	instances *store.InstanceStore
}

// NewInstanceHandler creates a new InstanceHandler.
func NewInstanceHandler(instances *store.InstanceStore) *InstanceHandler {
	return &InstanceHandler{instances: instances}
}

// List handles GET /api/v1/instances
func (h *InstanceHandler) List(c *gin.Context) {
	instances, err := h.instances.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, instances)
}

type addInstanceRequest struct {
	Port        int    `json:"port" binding:"required"`
	MetricsPort int    `json:"metrics_port"`
	Label       string `json:"label"`
}

// Add handles POST /api/v1/instances
func (h *InstanceHandler) Add(c *gin.Context) {
	var req addInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inst := &model.Instance{
		Port:        req.Port,
		MetricsPort: req.MetricsPort,
		Enabled:     true,
		Label:       req.Label,
	}

	// Auto-assign metrics_port: use request value, or derive from the highest
	// existing metrics_port + 1 (minimum 9091).
	if inst.MetricsPort == 0 {
		inst.MetricsPort = h.nextMetricsPort(c)
	}

	if err := inst.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.instances.Create(c.Request.Context(), inst); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, inst)
}

// Remove handles DELETE /api/v1/instances/:port
func (h *InstanceHandler) Remove(c *gin.Context) {
	port, err := strconv.Atoi(c.Param("port"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port"})
		return
	}

	count, err := h.instances.Count(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last instance"})
		return
	}

	if err := h.instances.Delete(c.Request.Context(), port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// nextMetricsPort returns the next available metrics port (max existing + 1, minimum 9091).
func (h *InstanceHandler) nextMetricsPort(c *gin.Context) int {
	instances, err := h.instances.List(c.Request.Context())
	if err != nil || len(instances) == 0 {
		return 9091
	}

	maxPort := 0
	for _, inst := range instances {
		if inst.MetricsPort > maxPort {
			maxPort = inst.MetricsPort
		}
	}
	next := maxPort + 1
	if next < 9091 {
		next = 9091
	}

	// Ensure the port isn't already taken by an existing instance
	for {
		taken := false
		for _, inst := range instances {
			if inst.MetricsPort == next || inst.Port == next {
				taken = true
				break
			}
		}
		if !taken {
			return next
		}
		next++
	}
}
