# Architecture -- Streaming

## Purpose

Generic, reusable Go module for streaming and transport abstractions. Provides Server-Sent Events (SSE), WebSocket with rooms, gRPC streaming utilities, webhook dispatch with HMAC signing, an HTTP client with circuit breaker, a unified transport abstraction layer, Gin framework adapters, and real-time file change notification with debouncing.

## Structure

```
pkg/
  sse/          Server-Sent Events broker with heartbeat, client management, and http.Handler
  websocket/    WebSocket hub with rooms, read/write pumps, ping/pong keepalive (gorilla/websocket)
  grpc/         gRPC streaming server, interceptors, metadata handling, health check server
  webhook/      Webhook dispatch with retry, exponential backoff, HMAC-SHA256 signing, registry
  http/         HTTP client with retry, exponential backoff, and three-state circuit breaker
  transport/    Unified Transport interface (Send/Receive/Close) and Factory for creating by type
  gin/          Gin framework adapters for SSE and WebSocket endpoints
  realtime/     File change debouncing and path filtering for downstream consumers
```

## Key Components

- **`sse.Broker`** -- Manages SSE connections, broadcasts events, sends heartbeats. Implements `http.Handler`
- **`websocket.Hub`** -- Connection manager with rooms, OnMessage handler, Broadcast, SendToRoom, JoinRoom/LeaveRoom
- **`grpc.StreamServer`** -- Interface for unary, server-stream, client-stream, and bidirectional streaming
- **`grpc.HealthServer`** -- Implements gRPC health protocol with per-service status tracking
- **`webhook.Dispatcher`** -- Delivers webhooks with retry/backoff and HMAC-SHA256 signing
- **`http.Client`** -- HTTP client with configurable retry, backoff, and circuit breaker (Closed/Open/HalfOpen)
- **`transport.Factory`** -- Creates Transport instances by registered type (HTTP, WebSocket, gRPC)
- **`realtime.Debouncer`** -- Aggregates rapid filesystem events into consolidated batches

## Data Flow

```
SSE: broker.Broadcast(event) -> format SSE -> send to all connected clients via channels
WebSocket: hub.ServeWS(w,r) -> upgrade -> read/write pumps -> hub.OnMessage(handler) -> hub.SendToRoom()
Webhook: dispatcher.Send(ctx, webhook, payload) -> Sign(payload, secret) -> POST with retry/backoff
HTTP: client.Request(ctx, method, url, body) -> circuit breaker check -> send -> retry on failure
Transport: factory.Create(config) -> Transport.Send(data) / Transport.Receive() / Transport.Close()
```

## Dependencies

- `github.com/gorilla/websocket` -- WebSocket protocol
- `github.com/gin-gonic/gin` -- Gin framework (for adapter package)
- `google.golang.org/grpc` -- gRPC framework
- `github.com/stretchr/testify` -- Test assertions

## Testing Strategy

Table-driven tests with `testify` and race detection. SSE tests use httptest servers. WebSocket tests use gorilla/websocket test helpers. gRPC tests verify interceptor chaining and health server status. Webhook tests verify HMAC signing and retry behavior. HTTP client tests verify circuit breaker state transitions.
