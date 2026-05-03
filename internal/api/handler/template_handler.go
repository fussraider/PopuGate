package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
)

// TemplateHandler handles secret template endpoints.
type TemplateHandler struct {
	templates *service.TemplateService
}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler(templates *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{templates: templates}
}

// List handles GET /api/v1/templates
// @Summary      List templates
// @Description  Retrieve all secret templates
// @Tags         templates
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /templates [get]
func (h *TemplateHandler) List(c *gin.Context) {
	templates, err := h.templates.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if templates == nil {
		templates = []model.SecretTemplate{}
	}
	c.JSON(http.StatusOK, templates)
}

// Get handles GET /api/v1/templates/:name
// @Summary      Get a template
// @Description  Retrieve a single template by name
// @Tags         templates
// @Produce      json
// @Param        name  path  string  true  "Template name"
// @Success      200  {object}  object
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /templates/{name} [get]
func (h *TemplateHandler) Get(c *gin.Context) {
	name := c.Param("name")
	tmpl, err := h.templates.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if tmpl == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	c.JSON(http.StatusOK, tmpl)
}

type createTemplateRequest struct {
	Name        string `json:"name" binding:"required,alphanumdash,max=64"`
	MaxConns    int    `json:"max_conns"`
	MaxIPs      int    `json:"max_ips"`
	QuotaBytes  int64  `json:"quota_bytes"`
	ExpiresDays int    `json:"expires_days"`
	Notes       string `json:"notes" binding:"max=500"`
}

// Create handles POST /api/v1/templates
// @Summary      Create a template
// @Description  Create a new secret template
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        body  body  createTemplateRequest  true  "Template"
// @Success      201  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /templates [post]
func (h *TemplateHandler) Create(c *gin.Context) {
	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	tmpl := &model.SecretTemplate{
		Name:        req.Name,
		MaxConns:    req.MaxConns,
		MaxIPs:      req.MaxIPs,
		QuotaBytes:  req.QuotaBytes,
		ExpiresDays: req.ExpiresDays,
		Notes:       req.Notes,
	}

	if err := h.templates.Create(c.Request.Context(), tmpl); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLog(c, "template.create", fmt.Sprintf("name=%s", req.Name))
	c.JSON(http.StatusCreated, tmpl)
}

// Delete handles DELETE /api/v1/templates/:name
// @Summary      Delete a template
// @Description  Delete a template by name
// @Tags         templates
// @Produce      json
// @Param        name  path  string  true  "Template name"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /templates/{name} [delete]
func (h *TemplateHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.templates.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLog(c, "template.delete", fmt.Sprintf("name=%s", name))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type applyTemplateRequest struct {
	SecretLabel string `json:"secret_label" binding:"required"`
}

// Apply handles POST /api/v1/templates/:name/apply
// @Summary      Apply template to secret
// @Description  Apply a template's limits to an existing secret
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        name  path  string                true  "Template name"
// @Param        body  body  applyTemplateRequest  true  "Secret label"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /templates/{name}/apply [post]
func (h *TemplateHandler) Apply(c *gin.Context) {
	name := c.Param("name")
	var req applyTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := h.templates.ApplyToSecret(c.Request.Context(), name, req.SecretLabel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLog(c, "template.apply", fmt.Sprintf("template=%s secret=%s", name, req.SecretLabel))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
