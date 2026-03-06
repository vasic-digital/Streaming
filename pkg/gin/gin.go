// Package gin provides Gin framework adapters for digital.vasic.streaming.
package gin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/websocket"
)

// SSEHandler creates a Gin handler that serves Server-Sent Events using the SSE Broker.
// The Broker implements http.Handler, so we delegate directly.
func SSEHandler(broker *sse.Broker) gin.HandlerFunc {
	return func(c *gin.Context) {
		broker.ServeHTTP(c.Writer, c.Request)
	}
}

// WebSocketConfig holds configuration for the WebSocket Gin adapter.
type WebSocketConfig struct {
	// UserIDParam is the query parameter name for the user ID.
	UserIDParam string
}

// DefaultWebSocketConfig returns sensible defaults.
func DefaultWebSocketConfig() *WebSocketConfig {
	return &WebSocketConfig{
		UserIDParam: "userId",
	}
}

// WebSocketHandler creates a Gin handler for WebSocket connections using the Hub.
// It upgrades the HTTP connection via Hub.ServeWS and optionally joins the client
// to a room based on the userId query parameter.
func WebSocketHandler(hub *websocket.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		hub.ServeWS(c.Writer, c.Request)
	}
}

// HubStatsHandler creates a Gin handler that returns WebSocket hub statistics.
func HubStatsHandler(hub *websocket.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"connectedClients": hub.ClientCount(),
			"rooms":            hub.RoomCount(),
		})
	}
}
