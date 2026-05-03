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
// @Summary      List backups
// @Description  Returns a list of all available database backups
// @Tags         backup
// @Accept       json
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups [get]
func (h *BackupHandler) List(c *gin.Context) {
	backups, err := h.backups.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, backups)
}

// Create handles POST /api/v1/backups
// @Summary      Create backup
// @Description  Creates a new database backup and returns its filename, size, and creation timestamp
// @Tags         backup
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups [post]
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
// @Summary      Restore backup
// @Description  Restores the database from a specified backup file. Overwrites current database and config files.
// @Tags         backup
// @Accept       json
// @Produce      json
// @Param        body  body  object{filename=string}  true  "Backup filename to restore"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups/restore [post]
func (h *BackupHandler) Restore(c *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
// @Summary      Delete backup
// @Description  Deletes a backup file by its filename
// @Tags         backup
// @Accept       json
// @Produce      json
// @Param        filename  path  string  true  "Backup filename"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups/{filename} [delete]
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
// @Summary      Download backup
// @Description  Downloads a backup file by its filename as a binary attachment
// @Tags         backup
// @Accept       json
// @Produce      application/octet-stream
// @Param        filename  path  string  true  "Backup filename"
// @Success      200  {file}  binary
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups/download/{filename} [get]
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
