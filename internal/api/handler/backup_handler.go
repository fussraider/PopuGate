package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/store"
)

// BackupHandler handles backup endpoints.
type BackupHandler struct {
	backups *store.BackupStore
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(backups *store.BackupStore) *BackupHandler {
	return &BackupHandler{backups: backups}
}

// List handles GET /api/v1/backups
func (h *BackupHandler) List(c *gin.Context) {
	backups, err := h.backups.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, backups)
}

// Create handles POST /api/v1/backups
func (h *BackupHandler) Create(c *gin.Context) {
	backup, err := h.backups.Create(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"filename":   backup.Filename,
		"size":       backup.Size,
		"created_at": backup.CreatedAt,
	})
}

// Restore handles POST /api/v1/backups/restore
func (h *BackupHandler) Restore(c *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Prevent path traversal
	if strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	if err := h.backups.Restore(c.Request.Context(), req.Filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "restore failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"warning": "Database and configuration files were overwritten. Restart the proxy engine to apply changes.",
	})
}

// Delete handles DELETE /api/v1/backups/:filename
func (h *BackupHandler) Delete(c *gin.Context) {
	filename := c.Param("filename")
	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	if err := h.backups.Delete(c.Request.Context(), filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Download handles GET /api/v1/backups/download/:filename
func (h *BackupHandler) Download(c *gin.Context) {
	filename := c.Param("filename")
	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	path := h.backups.GetPath(filename)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filepath.Base(filename), `"`, `\"`)))
	c.Header("Content-Type", "application/octet-stream")
	c.File(path)
}
