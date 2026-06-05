package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
)

// AuditHandler handles audit log endpoints.
type AuditHandler struct {
	audit *service.AuditService
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(audit *service.AuditService) *AuditHandler {
	return &AuditHandler{audit: audit}
}

// List handles GET /api/v1/audit
// @Summary      List audit entries
// @Description  Retrieve audit log entries with pagination and filtering
// @Tags         audit
// @Produce      json
// @Param        limit   query  int      false  "Max entries (1-1000, default 100)"
// @Param        offset  query  int      false  "Offset (default 0)"
// @Param        users   query  string   false  "Comma-separated list of usernames to filter by"
// @Param        actions query  string   false  "Comma-separated list of actions to filter by"
// @Param        from    query  int      false  "From Unix timestamp (inclusive)"
// @Param        to      query  int      false  "To Unix timestamp (inclusive)"
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /audit [get]
func (h *AuditHandler) List(c *gin.Context) {
	limit := 100
	offset := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 1000 {
				n = 1000
			}
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	var filter model.AuditFilter

	if v := c.Query("users"); v != "" {
		filter.Users = strings.Split(v, ",")
	} else if users := c.QueryArray("user"); len(users) > 0 {
		filter.Users = users
	}

	if v := c.Query("actions"); v != "" {
		filter.Actions = strings.Split(v, ",")
	} else if actions := c.QueryArray("action"); len(actions) > 0 {
		filter.Actions = actions
	}

	if v := c.Query("from"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			filter.From = n
		}
	}

	if v := c.Query("to"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			filter.To = n
		}
	}

	entries, err := h.audit.List(c.Request.Context(), limit, offset, &filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if entries == nil {
		entries = []model.AuditEntry{}
	}
	c.JSON(http.StatusOK, entries)
}

// GetFilters handles GET /api/v1/audit/filters
// @Summary      Get audit filters list
// @Description  Retrieve a list of unique users and actions present in the logs
// @Tags         audit
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /audit/filters [get]
func (h *AuditHandler) GetFilters(c *gin.Context) {
	users, actions, err := h.audit.GetFilters(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if users == nil {
		users = []string{}
	}
	if actions == nil {
		actions = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"users":   users,
		"actions": actions,
	})
}
