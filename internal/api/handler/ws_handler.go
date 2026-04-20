package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/fussraider/PopuGate/internal/service"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSHandler handles WebSocket live metrics streaming.
type WSHandler struct {
	traffic *service.TrafficService
}

// NewWSHandler creates a new WSHandler.
func NewWSHandler(traffic *service.TrafficService) *WSHandler {
	return &WSHandler{traffic: traffic}
}

// Handle upgrades to WebSocket and streams live metrics every 2 seconds.
func (h *WSHandler) Handle(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
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
