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
// @Summary      Replication status
// @Description  Returns the current replication role, enabled state, and list of slaves
// @Tags         replication
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/status [get]
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
	Role    string `json:"role" binding:"required,oneof=master slave"`
	SSHUser string `json:"ssh_user"`
	SSHPort int    `json:"ssh_port" binding:"omitempty,min=1,max=65535"`
}

// Setup handles POST /api/v1/replication/setup
// @Summary      Setup replication
// @Description  Configures replication role (master or slave) with optional SSH settings
// @Tags         replication
// @Accept       json
// @Produce      json
// @Param        body  body  object{role=string,ssh_user=string,ssh_port=int}  true  "Replication configuration"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/setup [post]
func (h *ReplicationHandler) Setup(c *gin.Context) {
	var req replicationSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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

	auditLog(c, "replication.setup", fmt.Sprintf("role=%s", req.Role))
	c.JSON(http.StatusOK, gin.H{"ok": true, "role": req.Role})
}

type addSlaveRequest struct {
	Host  string `json:"host" binding:"required"`
	Port  int    `json:"port" binding:"omitempty,min=1,max=65535"`
	Label string `json:"label"`
}

// AddSlave handles POST /api/v1/replication/slaves
// @Summary      Add replication slave
// @Description  Registers a new replication slave with host, port, and optional label
// @Tags         replication
// @Accept       json
// @Produce      json
// @Param        body  body  object{host=string,port=int,label=string}  true  "Slave configuration"
// @Success      201  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/slaves [post]
func (h *ReplicationHandler) AddSlave(c *gin.Context) {
	var req addSlaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
	auditLog(c, "replication.add_slave", fmt.Sprintf("host=%s", req.Host))
	c.JSON(http.StatusCreated, slave)
}

// RemoveSlave handles DELETE /api/v1/replication/slaves/:host
// @Summary      Remove replication slave
// @Description  Removes a replication slave by its host address
// @Tags         replication
// @Accept       json
// @Produce      json
// @Param        host  path  string  true  "Slave host address"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/slaves/{host} [delete]
func (h *ReplicationHandler) RemoveSlave(c *gin.Context) {
	host := c.Param("host")
	if err := h.slaves.Delete(c.Request.Context(), host); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	auditLog(c, "replication.remove_slave", fmt.Sprintf("host=%s", host))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListSlaves handles GET /api/v1/replication/slaves
// @Summary      List replication slaves
// @Description  Returns a list of all configured replication slaves
// @Tags         replication
// @Accept       json
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/slaves [get]
func (h *ReplicationHandler) ListSlaves(c *gin.Context) {
	slaves, err := h.slaves.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, slaves)
}

// Sync handles POST /api/v1/replication/sync
// @Summary      Sync to all slaves
// @Description  Triggers a configuration sync to all registered replication slaves via SSH/SFTP
// @Tags         replication
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/sync [post]
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
	auditLog(c, "replication.sync", "all slaves")
	c.JSON(http.StatusOK, gin.H{"ok": true, "results": output})
}

// Test handles POST /api/v1/replication/test
// @Summary      Test SSH connection
// @Description  Tests SSH connectivity to a specified replication slave host
// @Tags         replication
// @Accept       json
// @Produce      json
// @Param        body  body  object{host=string}  true  "Host to test"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/test [post]
func (h *ReplicationHandler) Test(c *gin.Context) {
	if h.replSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "replication service not available"})
		return
	}

	var req struct {
		Host string `json:"host" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
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
// @Summary      Get SSH public key
// @Description  Returns the SSH public key used for replication
// @Tags         replication
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/ssh-key [get]
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
// @Summary      Generate SSH key pair
// @Description  Generates a new SSH key pair for replication and returns the public key
// @Tags         replication
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /replication/ssh-keygen [post]
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
