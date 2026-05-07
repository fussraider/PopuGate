package handler

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
)

// BackupHandler handles backup endpoints.
type BackupHandler struct {
	backups      *store.BackupStore
	settings     *store.SettingsStore
	containerSvc *service.ContainerService
	auditSvc     *service.AuditService
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(backups *store.BackupStore, settings *store.SettingsStore, containerSvc *service.ContainerService, auditSvc *service.AuditService) *BackupHandler {
	return &BackupHandler{
		backups:      backups,
		settings:     settings,
		containerSvc: containerSvc,
		auditSvc:     auditSvc,
	}
}

// List handles GET /api/v1/backups
// @Summary      List backups
// @Description  Returns a list of all available backups and whether backup encryption is enabled.
// @Tags         backup
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups [get]
func (h *BackupHandler) List(c *gin.Context) {
	backups, err := h.backups.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"backups":            backups,
		"encryption_enabled": h.backups.EncryptionEnabled(),
	})
}

// Create handles POST /api/v1/backups
// @Summary      Create backup
// @Description  Creates a new encrypted or unencrypted backup depending on BACKUP_ENCRYPTION_KEY env var. Returns filename, size, checksum, and creation timestamp.
// @Tags         backup
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /backups [post]
func (h *BackupHandler) Create(c *gin.Context) {
	backup, err := h.backups.Create(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Audit log
	if h.auditSvc != nil {
		h.auditSvc.Log(c.Request.Context(), getUser(c), "backup.created",
			fmt.Sprintf("file=%s size=%d checksum=%s", backup.Filename, backup.Size, backup.Checksum))
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"filename":   backup.Filename,
		"size":       backup.Size,
		"created_at": backup.CreatedAt,
		"checksum":   backup.Checksum,
	})
}

// Restore handles POST /api/v1/backups/restore
// @Summary      Restore backup
// @Description  Stops the proxy engine, restores database and config from backup, rotates JWT secret, then restarts the engine. Verifies SHA256 checksum if available.
// @Tags         backup
// @Accept       json
// @Produce      json
// @Param        body  body  object{filename=string}  true  "Backup filename to restore"
// @Success      200  {object}  map[string]interface{}
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

	// Stop the proxy engine before restore
	if h.containerSvc != nil {
		if err := h.containerSvc.Stop(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop proxy engine"})
			return
		}
	}

	// Perform restore
	if err := h.backups.Restore(c.Request.Context(), req.Filename); err != nil {
		// Try to restart the engine even on failure
		if h.containerSvc != nil {
			_ = h.containerSvc.Start(c.Request.Context())
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "restore failed"})
		return
	}

	// Start the proxy engine after restore
	engineStarted := true
	if h.containerSvc != nil {
		if err := h.containerSvc.Start(c.Request.Context()); err != nil {
			engineStarted = false
		}
	}

	// Always rotate JWT secret after restore — the old DB is overwritten
	if h.settings != nil {
		if newSecret, err := generateRandomHex(32); err == nil {
			_ = h.settings.Save(c.Request.Context(), map[string]string{"jwt_secret": newSecret})
		}
	}

	// Audit log
	if h.auditSvc != nil {
		h.auditSvc.Log(c.Request.Context(), getUser(c), "backup.restored",
			fmt.Sprintf("file=%s", req.Filename))
	}

	if engineStarted {
		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"message": "Database and configuration files were restored. Proxy engine restarted. JWT secret rotated.",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"warning": "Database restored and JWT rotated but failed to restart proxy engine.",
		})
	}
}

// Delete handles DELETE /api/v1/backups/:filename
// @Summary      Delete backup
// @Description  Deletes a backup file and its checksum sidecar by filename
// @Tags         backup
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

	// Audit log
	if h.auditSvc != nil {
		h.auditSvc.Log(c.Request.Context(), getUser(c), "backup.deleted",
			fmt.Sprintf("file=%s", filename))
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Download handles GET /api/v1/backups/download/:filename
// @Summary      Download backup
// @Description  Downloads a backup file (encrypted or plain) by filename as a binary attachment
// @Tags         backup
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

	// Audit log
	if h.auditSvc != nil {
		info, _ := os.Stat(path)
		h.auditSvc.Log(c.Request.Context(), getUser(c), "backup.downloaded",
			fmt.Sprintf("file=%s size=%d", filename, info.Size()))
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filepath.Base(filename), `"`, `\"`)))
	c.Header("Content-Type", "application/octet-stream")
	c.File(path)
}

// getUser extracts the username from the request context.
func getUser(c *gin.Context) string {
	if user, exists := c.Get("username"); exists {
		if username, ok := user.(string); ok {
			return username
		}
	}
	return "unknown"
}

// generateRandomHex generates a random hex string of n bytes.
func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
