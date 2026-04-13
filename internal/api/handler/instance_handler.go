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
	if inst.MetricsPort == 0 {
		inst.MetricsPort = 9091
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

	if err := h.instances.Delete(c.Request.Context(), port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
