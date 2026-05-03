package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/scheduler"
	"github.com/fussraider/PopuGate/internal/service"
)

// SchedulerHandler handles scheduler management endpoints.
type SchedulerHandler struct {
	svc *service.SchedulerService
}

// NewSchedulerHandler creates a new SchedulerHandler.
func NewSchedulerHandler(svc *service.SchedulerService) *SchedulerHandler {
	return &SchedulerHandler{svc: svc}
}

type updateTaskRequest struct {
	Enabled  *bool   `json:"enabled"`
	Schedule *string `json:"schedule"`
}

// List handles GET /api/v1/scheduler/tasks
func (h *SchedulerHandler) List(c *gin.Context) {
	tasks, err := h.svc.ListTasks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

// Update handles PUT /api/v1/scheduler/tasks/:name
func (h *SchedulerHandler) Update(c *gin.Context) {
	name := c.Param("name")

	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Enabled == nil && req.Schedule == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to update"})
		return
	}

	if err := h.svc.UpdateTask(c.Request.Context(), name, req.Enabled, req.Schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RunNow handles POST /api/v1/scheduler/tasks/:name/run
func (h *SchedulerHandler) RunNow(c *gin.Context) {
	name := c.Param("name")

	rec, err := h.svc.RunTaskNow(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rec)
}

// History handles GET /api/v1/scheduler/tasks/:name/history
func (h *SchedulerHandler) History(c *gin.Context) {
	name := c.Param("name")
	limit, offset := getPagination(c)

	records, err := h.svc.GetHistory(c.Request.Context(), name, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if records == nil {
		records = make([]scheduler.ExecutionRecord, 0)
	}
	c.JSON(http.StatusOK, records)
}

// AllHistory handles GET /api/v1/scheduler/history
func (h *SchedulerHandler) AllHistory(c *gin.Context) {
	limit, offset := getPagination(c)

	records, err := h.svc.GetAllHistory(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if records == nil {
		records = make([]scheduler.ExecutionRecord, 0)
	}
	c.JSON(http.StatusOK, records)
}

func getPagination(c *gin.Context) (limit, offset int) {
	limit = 20
	offset = 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}
