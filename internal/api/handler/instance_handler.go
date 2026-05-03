package handler

import (
	"fmt"
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
// @Summary      List instances
// @Description  Retrieve all proxy instances
// @Tags         instances
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances [get]
func (h *InstanceHandler) List(c *gin.Context) {
	instances, err := h.instances.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, instances)
}

type addInstanceRequest struct {
	Port        int    `json:"port" binding:"required,min=1,max=65535"`
	MetricsPort int    `json:"metrics_port" binding:"omitempty,min=1,max=65535"`
	Label       string `json:"label" binding:"omitempty,alphanumdash,max=32"`
}

// Add handles POST /api/v1/instances
// @Summary      Add an instance
// @Description  Create a new proxy instance with a port, optional metrics port, and label
// @Tags         instances
// @Accept       json
// @Produce      json
// @Param        body  body  addInstanceRequest  true  "Instance configuration"
// @Success      201  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances [post]
func (h *InstanceHandler) Add(c *gin.Context) {
	var req addInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
	auditLog(c, "instance.create", fmt.Sprintf("port=%d label=%s", req.Port, req.Label))
	c.JSON(http.StatusCreated, inst)
}

// Remove handles DELETE /api/v1/instances/:port
// @Summary      Remove an instance
// @Description  Delete a proxy instance by port. The last instance cannot be removed.
// @Tags         instances
// @Produce      json
// @Param        port  path  int  true  "Instance port"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{port} [delete]
func (h *InstanceHandler) Remove(c *gin.Context) {
	port, err := strconv.Atoi(c.Param("port"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port"})
		return
	}

	count, err := h.instances.Count(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if count <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last instance"})
		return
	}

	if err := h.instances.Delete(c.Request.Context(), port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	auditLog(c, "instance.delete", fmt.Sprintf("port=%s", c.Param("port")))
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
