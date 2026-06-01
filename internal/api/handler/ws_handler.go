package handler

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/fussraider/PopuGate/internal/service"
)

var (
	wsOriginsMu sync.RWMutex
	wsOrigins   []string
)

// SetWSAllowedOrigins configures allowed WebSocket origins.
func SetWSAllowedOrigins(origins []string) {
	wsOriginsMu.Lock()
	wsOrigins = origins
	wsOriginsMu.Unlock()
}

func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	wsOriginsMu.RLock()
	allowed := wsOrigins
	wsOriginsMu.RUnlock()
	if len(allowed) == 0 {
		return true
	}
	for _, o := range allowed {
		if o == "*" || o == origin {
			return true
		}
	}
	return strings.EqualFold(origin, r.Host)
}

// WSUpgrader is the shared upgrader for all WebSocket connections.
var WSUpgrader = websocket.Upgrader{
	CheckOrigin: checkWSOrigin,
}

// WSHandler handles WebSocket live metrics streaming.
type WSHandler struct {
	traffic *service.TrafficService
	telemt  *service.TelemtUpdateService
}

// NewWSHandler creates a new WSHandler.
func NewWSHandler(traffic *service.TrafficService, telemt *service.TelemtUpdateService) *WSHandler {
	return &WSHandler{
		traffic: traffic,
		telemt:  telemt,
	}
}

// Handle upgrades to WebSocket and streams live metrics every 2 seconds.
// @Summary      Stream real-time traffic/connections metrics
// @Description  Upgrades HTTP connection to WebSocket to receive periodic traffic and connections metrics.
// @Tags         traffic
// @Success      101  {object}  object "Switching Protocols"
// @Security     BearerAuth
// @Router       /traffic/live/ws [get]
func (h *WSHandler) Handle(c *gin.Context) {
	conn, err := WSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		live, err := h.traffic.GetLiveMetrics(c.Request.Context())
		if err != nil {
			_ = conn.WriteJSON(gin.H{"error": err.Error()})
			continue
		}
		if err := conn.WriteJSON(gin.H{
			"connections":                live.ConnsCurrent,
			"connections_total":          live.ConnsTotal,
			"connections_bad_total":      live.ConnsBadTotal,
			"connections_me_current":     live.ConnsMECurrent,
			"connections_direct_current": live.ConnsDirectCurrent,
			"upstream_attempt_total":     live.UpstreamAttemptTotal,
			"upstream_success_total":     live.UpstreamSuccessTotal,
			"upstream_fail_total":        live.UpstreamFailTotal,
			"me_writers_active":          live.MEWritersActive,
			"me_writers_warm":            live.MEWritersWarm,
			"uptime_seconds":             live.UptimeSeconds,
			"user_metrics":               live.UserMetrics,
		}); err != nil {
			return
		}
	}
}

// HandleEngineUpdate upgrades to WebSocket and streams engine update status.
// @Summary      Stream engine update status
// @Description  Upgrades HTTP connection to WebSocket to receive updates on telemt engine download, verify, and application status.
// @Tags         engine
// @Success      101  {object}  object "Switching Protocols"
// @Security     BearerAuth
// @Router       /engine/update/ws [get]
func (h *WSHandler) HandleEngineUpdate(c *gin.Context) {
	conn, err := WSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	ch, unsubscribe := h.telemt.Subscribe()
	defer unsubscribe()

	// Send current state immediately
	if status, err := h.telemt.GetStatus(c.Request.Context()); err == nil {
		if err := conn.WriteJSON(status); err != nil {
			return
		}
	}

	for {
		select {
		case status, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(status); err != nil {
				return
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}
