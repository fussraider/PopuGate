package handler

import (
	"net/http"
	"strconv"

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
// @Description  Retrieve audit log entries with pagination
// @Tags         audit
// @Produce      json
// @Param        limit   query  int  false  "Max entries (1-1000, default 100)"
// @Param        offset  query  int  false  "Offset (default 0)"
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

	entries, err := h.audit.List(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if entries == nil {
		entries = []model.AuditEntry{}
	}
	c.JSON(http.StatusOK, entries)
}
