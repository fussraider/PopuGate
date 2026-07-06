package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
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
// @Summary      Get traffic summary
// @Description  Returns global traffic statistics (bytes in/out) and per-user traffic breakdown
// @Tags         traffic
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /traffic [get]
func (h *TrafficHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()

	global, err := h.traffic.GetGlobal(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	users, err := h.traffic.ListUserTraffic(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
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
// @Summary      Get live traffic metrics
// @Description  Returns real-time connection metrics scraped from the telemt engine Prometheus endpoint
// @Tags         traffic
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /traffic/live [get]
func (h *TrafficHandler) GetLive(c *gin.Context) {
	if h.trafficSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "traffic service not available"})
		return
	}

	live, err := h.trafficSvc.GetLiveMetrics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connections":            live.ConnsCurrent,
		"connections_total":      live.ConnsTotal,
		"connections_bad_total":  live.ConnsBadTotal,
		"upstream_attempt_total": live.UpstreamAttemptTotal,
		"upstream_success_total": live.UpstreamSuccessTotal,
		"upstream_fail_total":    live.UpstreamFailTotal,
		"me_writers_active":      live.MEWritersActive,
		"me_writers_warm":        live.MEWritersWarm,
		"uptime_seconds":         live.UptimeSeconds,
		"user_metrics":           live.UserMetrics,
	})
}

// GetUser handles GET /api/v1/traffic/:label
// @Summary      Get user traffic
// @Description  Returns traffic statistics for a specific user identified by label
// @Tags         traffic
// @Accept       json
// @Produce      json
// @Param        label  path  string  true  "User label"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /traffic/{label} [get]
func (h *TrafficHandler) GetUser(c *gin.Context) {
	label := c.Param("label")
	user, err := h.traffic.GetUserTraffic(c.Request.Context(), label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// GetHistory handles GET /api/v1/traffic/history
// @Summary      Get traffic history
// @Description  Returns historical traffic records with optional time range filtering and aggregation
// @Tags         traffic
// @Accept       json
// @Produce      json
// @Param        start      query  int     false  "Start timestamp (Unix epoch, default: 24h ago)"
// @Param        end        query  int     false  "End timestamp (Unix epoch, default: now)"
// @Param        label      query  string  false  "Filter by user label"
// @Param        aggregate  query  string  false  "Aggregation mode: none, hour, or day (default: none)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Security     BearerAuth
// @Router       /traffic/history [get]
func (h *TrafficHandler) GetHistory(c *gin.Context) {
	if h.trafficSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "traffic service not available"})
		return
	}

	end, _ := strconv.ParseInt(c.Query("end"), 10, 64)
	if end == 0 {
		end = time.Now().Unix()
	}
	start, _ := strconv.ParseInt(c.Query("start"), 10, 64)
	if start == 0 {
		start = end - 86400 // default: last 24 hours
	}
	label := c.Query("label")
	aggregate := c.DefaultQuery("aggregate", "none")

	if aggregate != "none" && aggregate != "hour" && aggregate != "day" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "aggregate must be 'none', 'hour', or 'day'"})
		return
	}

	records, err := h.trafficSvc.GetHistory(c.Request.Context(), start, end, label, aggregate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if records == nil {
		records = make([]model.TrafficHistoryRecord, 0)
	}
	c.JSON(http.StatusOK, gin.H{"history": records})
}
