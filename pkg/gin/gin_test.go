package gin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"digital.vasic.streaming/pkg/sse"
	"digital.vasic.streaming/pkg/websocket"

	adapter "digital.vasic.streaming/pkg/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestSSEHandler_SetsHeaders(t *testing.T) {
	broker := sse.NewBroker(sse.DefaultConfig())

	r := gin.New()
	r.GET("/events", adapter.SSEHandler(broker))

	w := httptest.NewRecorder()
	// Create a request with a context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "/events", nil).WithContext(ctx)

	// Run in goroutine since SSE blocks until context is done
	done := make(chan struct{})
	go func() {
		r.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
		// SSE broker sets Content-Type to text/event-stream
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	case <-time.After(5 * time.Second):
		t.Fatal("SSE handler did not return after context cancellation")
	}

	broker.Close()
}

func TestWebSocketUpgrade_RejectsNonWebSocket(t *testing.T) {
	cfg := websocket.DefaultConfig()
	hub := websocket.NewHub(cfg)

	r := gin.New()
	r.GET("/ws", adapter.WebSocketHandler(hub))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws", nil)
	r.ServeHTTP(w, req)

	// Non-WebSocket request should fail the upgrade (400 Bad Request from gorilla/websocket)
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestHubStatsHandler_ReturnsStats(t *testing.T) {
	cfg := websocket.DefaultConfig()
	hub := websocket.NewHub(cfg)

	r := gin.New()
	r.GET("/stats", adapter.HubStatsHandler(hub))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stats", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "connectedClients")
	assert.Contains(t, w.Body.String(), "rooms")
}

func TestWebSocketConfig_Defaults(t *testing.T) {
	cfg := adapter.DefaultWebSocketConfig()
	assert.Equal(t, "userId", cfg.UserIDParam)
}
