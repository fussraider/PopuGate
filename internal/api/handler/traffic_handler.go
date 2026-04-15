package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
)

// TrafficHandler handles traffic monitoring endpoints.
type TrafficHandler struct {
	traffic    *store.TrafficStore
	settings   *store.SettingsStore
	trafficSvc *service.TrafficService
}

// NewTrafficHandler creates a new TrafficHandler.
func NewTrafficHandler(traffic *store.TrafficStore, settings *store.SettingsStore) *TrafficHandler {
	return &TrafficHandler{traffic: traffic, settings: settings}
}

// SetTrafficService sets the traffic service for live metrics.
func (h *TrafficHandler) SetTrafficService(svc *service.TrafficService) {
	h.trafficSvc = svc
}

// Get handles GET /api/v1/traffic
func (h *TrafficHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()

	global, err := h.traffic.GetGlobal(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	users, err := h.traffic.ListUserTraffic(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"global": gin.H{
			"bytes_in":  global.BytesIn,
			"bytes_out": global.BytesOut,
		},
		"users": users,
	})
}

// GetLive handles GET /api/v1/traffic/live
func (h *TrafficHandler) GetLive(c *gin.Context) {
	if h.trafficSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "traffic service not available"})
		return
	}

	live, err := h.trafficSvc.GetLiveMetrics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connections":              live.ConnsCurrent,
		"connections_total":        live.ConnsTotal,
		"connections_bad_total":    live.ConnsBadTotal,
		"connections_me_current":   live.ConnsMECurrent,
		"connections_direct_current": live.ConnsDirectCurrent,
		"upstream_attempt_total":   live.UpstreamAttemptTotal,
		"upstream_success_total":   live.UpstreamSuccessTotal,
		"upstream_fail_total":      live.UpstreamFailTotal,
		"me_writers_active":        live.MEWritersActive,
		"me_writers_warm":          live.MEWritersWarm,
		"uptime_seconds":           live.UptimeSeconds,
		"user_metrics":             live.UserMetrics,
	})
}

// GetUser handles GET /api/v1/traffic/:label
func (h *TrafficHandler) GetUser(c *gin.Context) {
	label := c.Param("label")
	user, err := h.traffic.GetUserTraffic(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}
