package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var instanceLog = logger.WithScope("instance")

// InstanceHandler handles instance endpoints.
type InstanceHandler struct {
	instances    *store.InstanceStore
	containerSvc *service.ContainerService
	docker       *dockerutil.DockerClient
}

// NewInstanceHandler creates a new InstanceHandler.
func NewInstanceHandler(instances *store.InstanceStore) *InstanceHandler {
	return &InstanceHandler{instances: instances}
}

// SetContainerSvc sets the container service for per-instance management.
func (h *InstanceHandler) SetContainerSvc(svc *service.ContainerService) {
	h.containerSvc = svc
}

// SetDockerClient sets the Docker client for log streaming.
func (h *InstanceHandler) SetDockerClient(d *dockerutil.DockerClient) {
	h.docker = d
}

// List handles GET /api/v1/instances
// @Summary      List instances
// @Description  Retrieve all proxy instances
// @Tags         instances
// @Produce      json
// @Success      200  {array}   object
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances [get]
func (h *InstanceHandler) List(c *gin.Context) {
	instances, err := h.instances.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, instances)
}

type addInstanceRequest struct {
	Port          int    `json:"port" binding:"required,min=1,max=65535"`
	MetricsPort   int    `json:"metrics_port" binding:"omitempty,min=1,max=65535"`
	Label         string `json:"label" binding:"omitempty,alphanumdash,max=32"`
	TLSDomain     string `json:"tls_domain" binding:"required"`
	TLSDomains    string `json:"tls_domains"` // JSON array
	FakeTLS       *bool  `json:"fake_tls"`    // pointer to distinguish unset from false
	MaskHost      string `json:"mask_host"`
	MaskPort      int    `json:"mask_port" binding:"omitempty,min=1,max=65535"`
	Tags          string `json:"tags"` // JSON array
	TCPMSSEnabled *bool  `json:"tcp_mss_enabled"`
	TCPMSS        int    `json:"tcp_mss" binding:"omitempty,min=1,max=1460"`
	TLSFronting   *bool  `json:"tls_fronting"`
}

// Add handles POST /api/v1/instances
// @Summary      Add an instance
// @Description  Create a new proxy instance with a port, optional metrics port, and label
// @Tags         instances
// @Accept       json
// @Produce      json
// @Param        body  body  addInstanceRequest  true  "Instance configuration"
// @Success      201  {object}  object
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances [post]
func (h *InstanceHandler) Add(c *gin.Context) {
	var req addInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if err := validateAddRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inst := buildInstanceFromRequest(&req)

	if inst.MetricsPort == 0 {
		inst.MetricsPort = h.nextMetricsPort(c)
	}

	if inst.MetricsPort == inst.Port {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metrics_port must differ from port"})
		return
	}

	if err := inst.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.instances.Create(c.Request.Context(), inst); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLog(c, "instance.create", fmt.Sprintf("port=%d label=%s domain=%s", req.Port, req.Label, req.TLSDomain))
	c.JSON(http.StatusCreated, inst)
}

func validateAddRequest(req *addInstanceRequest) error {
	if req.TLSDomains != "" && req.TLSDomains != "[]" {
		var domains []string
		if err := json.Unmarshal([]byte(req.TLSDomains), &domains); err != nil {
			return fmt.Errorf("tls_domains must be a valid JSON array")
		}
	}
	if req.Tags != "" && req.Tags != "[]" {
		return model.ValidateTags(req.Tags)
	}
	return nil
}

func buildInstanceFromRequest(req *addInstanceRequest) *model.Instance {
	fakeTLS := true
	if req.FakeTLS != nil {
		fakeTLS = *req.FakeTLS
	}
	tcpMSSEnabled := false
	if req.TCPMSSEnabled != nil {
		tcpMSSEnabled = *req.TCPMSSEnabled
	}
	tcpMSS := 88
	if req.TCPMSS > 0 {
		tcpMSS = req.TCPMSS
	}
	tlsFronting := false
	if req.TLSFronting != nil {
		tlsFronting = *req.TLSFronting
	}

	return &model.Instance{
		Port:          req.Port,
		MetricsPort:   req.MetricsPort,
		Enabled:       true,
		Label:         req.Label,
		TLSDomain:     req.TLSDomain,
		TLSDomains:    req.TLSDomains,
		FakeTLS:       fakeTLS,
		MaskHost:      req.MaskHost,
		MaskPort:      req.MaskPort,
		Tags:          req.Tags,
		TCPMSSEnabled: tcpMSSEnabled,
		TCPMSS:        tcpMSS,
		TLSFronting:   tlsFronting,
	}
}

type updateInstanceRequest struct {
	Port          *int    `json:"port" binding:"omitempty,min=1,max=65535"`
	MetricsPort   *int    `json:"metrics_port" binding:"omitempty,max=65535"`
	Label         *string `json:"label" binding:"omitempty,alphanumdash,max=32"`
	Enabled       *bool   `json:"enabled"`
	TLSDomain     *string `json:"tls_domain"`
	TLSDomains    *string `json:"tls_domains"`
	FakeTLS       *bool   `json:"fake_tls"`
	MaskHost      *string `json:"mask_host"`
	MaskPort      *int    `json:"mask_port" binding:"omitempty,min=1,max=65535"`
	Tags          *string `json:"tags"`
	TCPMSSEnabled *bool   `json:"tcp_mss_enabled"`
	TCPMSS        *int    `json:"tcp_mss" binding:"omitempty,min=1,max=1460"`
	TLSFronting   *bool   `json:"tls_fronting"`
}

// Update handles PUT /api/v1/instances/:id
// @Summary      Update an instance
// @Description  Update an existing proxy instance configuration
// @Tags         instances
// @Accept       json
// @Produce      json
// @Param        id    path  int                   true  "Instance ID"
// @Param        body  body  updateInstanceRequest  true  "Instance updates"
// @Success      200  {object}  object
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id} [put]
func (h *InstanceHandler) getInstanceForUpdate(c *gin.Context) (int64, *model.Instance, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
		return 0, nil, false
	}
	inst, err := h.instances.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return 0, nil, false
	}
	if inst == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return 0, nil, false
	}
	return id, inst, true
}

func validateTLSDomainsField(val string) error {
	if val == "" || val == "[]" {
		return nil
	}
	var domains []string
	return json.Unmarshal([]byte(val), &domains)
}

func validateTagsField(val string) error {
	if val == "" || val == "[]" {
		return nil
	}
	return model.ValidateTags(val)
}

func (h *InstanceHandler) applyInstanceUpdates(c *gin.Context, inst *model.Instance, req *updateInstanceRequest) bool {
	if req.Port != nil {
		inst.Port = *req.Port
	}
	if req.MetricsPort != nil {
		inst.MetricsPort = *req.MetricsPort
	}
	if req.Label != nil {
		inst.Label = *req.Label
	}
	if req.Enabled != nil && !*req.Enabled && inst.Enabled {
		h.stopInstanceSafe(c, inst.ID)
	}
	if req.Enabled != nil {
		inst.Enabled = *req.Enabled
	}
	if req.TLSDomain != nil {
		inst.TLSDomain = *req.TLSDomain
	}
	if req.TLSDomains != nil {
		if err := validateTLSDomainsField(*req.TLSDomains); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tls_domains must be a valid JSON array"})
			return false
		}
		inst.TLSDomains = *req.TLSDomains
	}
	if !h.applySimpleFields(c, inst, req) {
		return false
	}
	return true
}

func (h *InstanceHandler) stopInstanceSafe(c *gin.Context, instID int64) {
	if h.containerSvc == nil {
		return
	}
	if err := h.containerSvc.StopInstance(c.Request.Context(), instID); err != nil {
		instanceLog.Warnf("stop instance %d before disable: %v", instID, err)
	}
}

func (h *InstanceHandler) applySimpleFields(c *gin.Context, inst *model.Instance, req *updateInstanceRequest) bool {
	if req.FakeTLS != nil {
		inst.FakeTLS = *req.FakeTLS
	}
	if req.MaskHost != nil {
		inst.MaskHost = *req.MaskHost
	}
	if req.MaskPort != nil {
		inst.MaskPort = *req.MaskPort
	}
	if req.Tags != nil {
		if err := validateTagsField(*req.Tags); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return false
		}
		inst.Tags = *req.Tags
	}
	if req.TCPMSSEnabled != nil {
		inst.TCPMSSEnabled = *req.TCPMSSEnabled
	}
	if req.TCPMSS != nil {
		inst.TCPMSS = *req.TCPMSS
	}
	if req.TLSFronting != nil {
		inst.TLSFronting = *req.TLSFronting
	}
	return true
}

func (h *InstanceHandler) validateAndSaveInstance(c *gin.Context, id int64, inst *model.Instance) bool {
	if inst.MetricsPort == 0 {
		inst.MetricsPort = h.nextMetricsPort(c)
	}
	if inst.MetricsPort == inst.Port {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metrics_port must differ from port"})
		return false
	}
	if err := inst.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	if err := h.instances.Update(c.Request.Context(), inst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return false
	}
	auditLog(c, "instance.update", fmt.Sprintf("id=%d label=%s port=%d", id, inst.Label, inst.Port))
	if h.containerSvc != nil {
		h.containerSvc.RevalidateInstance(c.Request.Context(), id)
		if err := h.containerSvc.ReconcileInstanceRules(c.Request.Context(), id); err != nil {
			c.Header("X-Warning", fmt.Sprintf("iptables rules failed: %v", err))
		}
	}
	c.JSON(http.StatusOK, inst)
	return true
}

func (h *InstanceHandler) Update(c *gin.Context) {
	id, inst, ok := h.getInstanceForUpdate(c)
	if !ok {
		return
	}

	var req updateInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	if !h.applyInstanceUpdates(c, inst, &req) {
		return
	}

	h.validateAndSaveInstance(c, id, inst)
}

// Remove handles DELETE /api/v1/instances/:id
// @Summary      Remove an instance
// @Description  Delete a proxy instance by ID. The last instance cannot be removed.
// @Tags         instances
// @Produce      json
// @Param        id  path  int  true  "Instance ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id} [delete]
func (h *InstanceHandler) Remove(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	var instLabel string
	var instPort int
	if err != nil {
		// Try port-based lookup for backward compat
		port, pErr := strconv.Atoi(c.Param("id"))
		if pErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
			return
		}
		inst, gErr := h.instances.GetByPort(c.Request.Context(), port)
		if gErr != nil || inst == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
			return
		}
		id = inst.ID
		instLabel = inst.Label
		instPort = inst.Port
	} else {
		inst, gErr := h.instances.GetByID(c.Request.Context(), id)
		if gErr != nil || inst == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
			return
		}
		instLabel = inst.Label
		instPort = inst.Port
	}

	count, err := h.instances.Count(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if count <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last instance"})
		return
	}

	if err := h.instances.DeleteByID(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	auditLog(c, "instance.delete", fmt.Sprintf("id=%d label=%s port=%d", id, instLabel, instPort))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// StartInstance handles POST /api/v1/instances/:id/start
// @Summary      Start an instance
// @Description  Start a specific proxy instance container
// @Tags         instances
// @Produce      json
// @Param        id  path  int  true  "Instance ID"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id}/start [post]
func (h *InstanceHandler) StartInstance(c *gin.Context) {
	id, ok := h.parseInstanceID(c)
	if !ok {
		return
	}
	if err := h.containerSvc.StartInstance(c.Request.Context(), id); err != nil {
		if errors.Is(err, service.ErrNoMatchingSecrets) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to start instance: %v", err)})
		return
	}
	h.auditInstanceAction(c, "instance.start", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// StopInstance handles POST /api/v1/instances/:id/stop
// @Summary      Stop an instance
// @Description  Stop a specific proxy instance container
// @Tags         instances
// @Produce      json
// @Param        id  path  int  true  "Instance ID"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id}/stop [post]
func (h *InstanceHandler) StopInstance(c *gin.Context) {
	id, ok := h.parseInstanceID(c)
	if !ok {
		return
	}
	if err := h.containerSvc.StopInstance(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to stop instance: %v", err)})
		return
	}
	h.auditInstanceAction(c, "instance.stop", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ReloadInstance handles POST /api/v1/instances/:id/reload
// @Summary      Reload an instance
// @Description  Reload a specific proxy instance configuration
// @Tags         instances
// @Produce      json
// @Param        id  path  int  true  "Instance ID"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id}/reload [post]
func (h *InstanceHandler) ReloadInstance(c *gin.Context) {
	id, ok := h.parseInstanceID(c)
	if !ok {
		return
	}
	if err := h.containerSvc.ReloadInstance(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reload instance: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RefreshFronting handles POST /api/v1/instances/:id/refresh-fronting
// @Summary      Refresh TLS fronting content
// @Description  Re-download TLS fronting content for an instance
// @Tags         instances
// @Produce      json
// @Param        id  path  int  true  "Instance ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id}/refresh-fronting [post]
func (h *InstanceHandler) RefreshFronting(c *gin.Context) {
	if h.containerSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return
	}
	id, ok := h.parseInstanceID(c)
	if !ok {
		return
	}
	if err := h.containerSvc.RefreshFrontingContent(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to refresh fronting: %v", err)})
		return
	}
	h.auditInstanceAction(c, "instance.refresh_fronting", id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// InstanceStatus handles GET /api/v1/instances/:id/status
// @Summary      Instance status
// @Description  Retrieve the current status of a specific proxy instance
// @Tags         instances
// @Produce      json
// @Param        id  path  int  true  "Instance ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id}/status [get]
func (h *InstanceHandler) InstanceStatus(c *gin.Context) {
	if h.containerSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
		return
	}
	status, err := h.containerSvc.InstanceStatus(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "status": status})
}

// InstanceLogs handles GET /api/v1/instances/:id/logs
// @Summary      Instance logs
// @Description  Retrieve logs for a specific proxy instance. Supports SSE streaming when follow=true.
// @Tags         instances
// @Produce      plain
// @Param        id      path  int   true  "Instance ID"
// @Param        tail    query int   false "Number of log lines to return (default 100)"
// @Param        follow  query bool  false "Enable SSE streaming of log output"
// @Success      200  {string}  string
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/{id}/logs [get]
func (h *InstanceHandler) InstanceLogs(c *gin.Context) {
	if h.docker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker not available"})
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
		return
	}

	inst, err := h.instances.GetByID(c.Request.Context(), id)
	if err != nil || inst == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	tail := c.DefaultQuery("tail", "100")
	follow := c.Query("follow") == "true"
	streamLogs(c, h.docker, inst.ContainerName(), tail, follow)
}

// CheckPort handles GET /api/v1/instances/check-port?port=XXX&exclude=ID
// @Summary      Check port availability
// @Description  Check if a TCP port is available for a new instance
// @Tags         instances
// @Produce      json
// @Param        port     query  int  true  "Port number to check"
// @Param        exclude  query  int  false "Instance ID to exclude from conflict check"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Router       /instances/check-port [get]
func (h *InstanceHandler) CheckPort(c *gin.Context) {
	port, err := strconv.Atoi(c.Query("port"))
	if err != nil || port < 1 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port"})
		return
	}

	excludeID, _ := strconv.ParseInt(c.Query("exclude"), 10, 64)

	conflict := ""
	instances, _ := h.instances.List(c.Request.Context())
	for _, inst := range instances {
		if inst.ID == excludeID {
			continue
		}
		if inst.Port == port {
			conflict = fmt.Sprintf("proxy port of instance %q (id=%d)", inst.Label, inst.ID)
			break
		}
		if inst.MetricsPort == port {
			conflict = fmt.Sprintf("metrics port of instance %q (id=%d)", inst.Label, inst.ID)
			break
		}
	}

	free := isPortFree(port)

	if conflict != "" {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": conflict})
		return
	}
	if !free {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": "port is already in use on this host"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": true})
}

// nextMetricsPort returns the next available metrics port.
func (h *InstanceHandler) nextMetricsPort(c *gin.Context) int {
	instances, err := h.instances.List(c.Request.Context())
	if err != nil || len(instances) == 0 {
		return 9091
	}

	maxPort := 0
	for _, inst := range instances {
		if inst.MetricsPort > maxPort {
			maxPort = inst.MetricsPort
		}
	}
	next := maxPort + 1
	if next < 9091 {
		next = 9091
	}

	for {
		taken := false
		for _, inst := range instances {
			if inst.MetricsPort == next || inst.Port == next {
				taken = true
				break
			}
		}
		if !taken && isPortFree(next) {
			return next
		}
		next++
	}
}

// parseInstanceID extracts and validates the :id param, checks containerSvc availability.
func (h *InstanceHandler) parseInstanceID(c *gin.Context) (int64, bool) {
	if h.containerSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "proxy service unavailable"})
		return 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
		return 0, false
	}
	return id, true
}

// auditInstanceAction logs an instance action with label and port.
func (h *InstanceHandler) auditInstanceAction(c *gin.Context, action string, id int64) {
	inst, _ := h.instances.GetByID(c.Request.Context(), id)
	label, port := "", 0
	if inst != nil {
		label = inst.Label
		port = inst.Port
	}
	auditLog(c, action, fmt.Sprintf("id=%d label=%s port=%d", id, label, port))
}

// isPortFree checks whether a TCP port is available on the host.
var isPortFree = func(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
