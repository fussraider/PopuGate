package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
)

// ReplicationHandler handles replication endpoints.
type ReplicationHandler struct {
	settings *store.SettingsStore
	slaves   *store.SlaveStore
	replSvc  *service.ReplicationService
}

// NewReplicationHandler creates a new ReplicationHandler.
func NewReplicationHandler(settings *store.SettingsStore, slaves *store.SlaveStore) *ReplicationHandler {
	return &ReplicationHandler{settings: settings, slaves: slaves}
}

// SetReplicationService sets the replication service.
func (h *ReplicationHandler) SetReplicationService(svc *service.ReplicationService) {
	h.replSvc = svc
}

// Status handles GET /api/v1/replication/status
func (h *ReplicationHandler) Status(c *gin.Context) {
	settings, _ := h.settings.Load(c.Request.Context())
	slaves, _ := h.slaves.List(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"role":    settings.ReplicationRole,
		"enabled": settings.ReplicationEnabled,
		"slaves":  slaves,
	})
}

type replicationSetupRequest struct {
	Role    string `json:"role" binding:"required"`
	SSHUser string `json:"ssh_user"`
	SSHPort int    `json:"ssh_port"`
}

// Setup handles POST /api/v1/replication/setup
func (h *ReplicationHandler) Setup(c *gin.Context) {
	var req replicationSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Role != "master" && req.Role != "slave" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be 'master' or 'slave'"})
		return
	}

	updates := map[string]string{
		"replication_role":    req.Role,
		"replication_enabled": "true",
	}
	if req.SSHUser != "" {
		updates["replication_ssh_user"] = req.SSHUser
	}
	if req.SSHPort > 0 {
		updates["replication_ssh_port"] = fmt.Sprintf("%d", req.SSHPort)
	}

	if err := h.settings.Save(c.Request.Context(), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "role": req.Role})
}

type addSlaveRequest struct {
	Host  string `json:"host" binding:"required"`
	Port  int    `json:"port"`
	Label string `json:"label"`
}

// AddSlave handles POST /api/v1/replication/slaves
func (h *ReplicationHandler) AddSlave(c *gin.Context) {
	var req addSlaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slave := &model.Slave{
		Host:    req.Host,
		Port:    req.Port,
		Label:   req.Label,
		Enabled: true,
		Status:  "unknown",
	}
	if slave.Port == 0 {
		slave.Port = 22
	}

	if err := slave.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.slaves.Create(c.Request.Context(), slave); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, slave)
}

// RemoveSlave handles DELETE /api/v1/replication/slaves/:host
func (h *ReplicationHandler) RemoveSlave(c *gin.Context) {
	host := c.Param("host")
	if err := h.slaves.Delete(c.Request.Context(), host); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListSlaves handles GET /api/v1/replication/slaves
func (h *ReplicationHandler) ListSlaves(c *gin.Context) {
	slaves, err := h.slaves.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, slaves)
}

// Sync handles POST /api/v1/replication/sync
func (h *ReplicationHandler) Sync(c *gin.Context) {
	if h.replSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "replication service not available"})
		return
	}

	results := h.replSvc.SyncAll(c.Request.Context())
	type syncResult struct {
		Host      string `json:"host"`
		FilesSent int    `json:"files_sent"`
		Error     string `json:"error,omitempty"`
	}
	var output []syncResult
	for _, r := range results {
		output = append(output, syncResult{Host: r.Host, FilesSent: r.FilesSent, Error: r.Error})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "results": output})
}

// Test handles POST /api/v1/replication/test
func (h *ReplicationHandler) Test(c *gin.Context) {
	if h.replSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "replication service not available"})
		return
	}

	var req struct {
		Host string `json:"host" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.replSvc.TestSSH(c.Request.Context(), req.Host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetSSHKey handles GET /api/v1/replication/ssh-key
func (h *ReplicationHandler) GetSSHKey(c *gin.Context) {
	if h.replSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "replication service not available"})
		return
	}

	publicKey, err := h.replSvc.GetSSHPublicKey(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no ssh key found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"public_key": publicKey})
}

// SSHKeygen handles POST /api/v1/replication/ssh-keygen
func (h *ReplicationHandler) SSHKeygen(c *gin.Context) {
	if h.replSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "replication service not available"})
		return
	}

	publicKey, err := h.replSvc.GenerateSSHKey(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "public_key": publicKey})
}
