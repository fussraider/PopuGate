package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/fussraider/PopuGate/internal/service"
)

// allowedWSOrigins controls which origins can open WebSocket connections.
// Set from router config via SetWSAllowedOrigins.
var allowedWSOrigins []string

// SetWSAllowedOrigins configures allowed WebSocket origins.
func SetWSAllowedOrigins(origins []string) {
	allowedWSOrigins = origins
}

// WSUpgrader is the shared upgrader for all WebSocket connections.
var WSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if len(allowedWSOrigins) == 0 {
			return true
		}
		for _, o := range allowedWSOrigins {
			if o == "*" || o == origin {
				return true
			}
		}
		return strings.EqualFold(origin, r.Host)
	},
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
func (h *WSHandler) Handle(c *gin.Context) {
	conn, err := WSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		live, err := h.traffic.GetLiveMetrics(c.Request.Context())
		if err != nil {
			_ = conn.WriteJSON(gin.H{"error": err.Error()}) // best-effort: client may disconnect
			continue
		}
		if err := conn.WriteJSON(live); err != nil {
			return // client disconnected
		}
	}
}

// HandleEngineUpdate upgrades to WebSocket and streams engine update status.
func (h *WSHandler) HandleEngineUpdate(c *gin.Context) {
	conn, err := WSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

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
