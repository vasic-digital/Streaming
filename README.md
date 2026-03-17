# Streaming

Generic, reusable Go module for streaming and transport abstractions. Provides Server-Sent Events (SSE), WebSocket with rooms, gRPC streaming utilities, webhook dispatch with HMAC signing, an HTTP client with circuit breaker, a unified transport abstraction layer, Gin framework adapters, and real-time file change notification with debouncing.

**Module**: `digital.vasic.streaming` (Go 1.24+)

## Architecture

The module is organized around protocol-specific packages, each self-contained, plus a transport abstraction that unifies them behind a common interface. A Gin adapter package bridges the streaming primitives to the Gin HTTP framework. The realtime package provides file-system change debouncing for downstream consumers.

```
pkg/
  sse/          Server-Sent Events broker
  websocket/    WebSocket hub with rooms and read/write pumps
  grpc/         gRPC streaming server, interceptors, health check
  webhook/      Webhook dispatch with retry, backoff, HMAC-SHA256
  http/         HTTP client with retry, circuit breaker
  transport/    Unified Transport interface and Factory
  gin/          Gin framework adapters for SSE and WebSocket
  realtime/     File change debouncing and path filtering
```

## Package Reference

### pkg/sse -- Server-Sent Events

Manages SSE client connections, broadcasts events, and sends automatic heartbeats.

**Types:**
- `Event` -- SSE event with ID, Type, Data, and Retry fields. `Format()` serializes to SSE wire format.
- `Client` -- Connected client with ID, Channel, LastEventID, and CreatedAt.
- `Config` -- BufferSize (default 100), HeartbeatInterval (default 30s), MaxClients (default 1000).
- `Broker` -- Central manager implementing `http.Handler`.

**Key Functions:**
- `NewBroker(config *Config) *Broker` -- Creates a broker and starts the heartbeat loop.
- `Broker.AddClient(w, r) (*Client, error)` -- Upgrades an HTTP connection to SSE streaming.
- `Broker.Broadcast(event *Event)` -- Sends an event to all connected clients.
- `Broker.SendTo(clientID string, event *Event) error` -- Sends to a specific client.
- `Broker.RemoveClient(id string)` -- Disconnects and removes a client.
- `Broker.ClientCount() int` -- Returns the number of connected clients.
- `Broker.Close()` -- Stops the broker and disconnects all clients.
- `Broker.ServeHTTP(w, r)` -- Implements `http.Handler` for direct use as an endpoint.

### pkg/websocket -- WebSocket Hub

Full-featured WebSocket hub with rooms, read/write pumps, ping/pong keepalive, and message broadcasting. Built on `gorilla/websocket`.

**Types:**
- `Message` -- JSON message with Type, Room, Data (json.RawMessage), and Sender fields.
- `Config` -- ReadBufferSize, WriteBufferSize, PingInterval (54s), PongWait (60s), WriteWait (10s), MaxMessageSize (512KB), AllowedOrigins.
- `Client` -- WebSocket connection wrapper with `ID()`, `Send(data)`, and `Close()`.
- `Room` -- Named group of clients.
- `Hub` -- Central connection manager.

**Key Functions:**
- `NewHub(config *Config) *Hub` -- Creates a hub and starts the event loop.
- `Hub.ServeWS(w, r) (*Client, error)` -- Upgrades HTTP to WebSocket, starts read/write pumps.
- `Hub.OnMessage(handler func(*Client, *Message))` -- Registers an incoming message handler.
- `Hub.Broadcast(msg *Message) error` -- Sends a message to all clients.
- `Hub.SendToRoom(roomName string, msg *Message) error` -- Sends to all clients in a room.
- `Hub.JoinRoom(clientID, roomName string) error` -- Adds a client to a room (auto-creates room).
- `Hub.LeaveRoom(clientID, roomName string) error` -- Removes a client from a room.
- `Hub.ClientCount() int` / `Hub.RoomCount() int` / `Hub.RoomClientCount(roomName) int`
- `Hub.Close()` -- Stops the hub and disconnects all clients.

### pkg/grpc -- gRPC Streaming

gRPC streaming utilities including a server interface, stream interceptors, metadata handling, logging, and a health check server implementing the gRPC health protocol.

**Types:**
- `Config` -- Address, TLSCertFile, TLSKeyFile, MaxRecvMsgSize (4MB), MaxSendMsgSize (4MB), MaxConcurrentStreams (100).
- `StreamServer` -- Interface with Unary, ServerStream, ClientStream, and BidirectionalStream methods.
- `StreamInterceptor` -- Function type for intercepting gRPC stream operations.
- `HealthServer` -- Implements `grpc_health_v1.HealthServer` with per-service status tracking.

**Key Functions:**
- `DefaultConfig() *Config` -- Returns default gRPC configuration.
- `ChainStreamInterceptors(interceptors...) StreamInterceptor` -- Chains multiple interceptors.
- `MetadataInterceptor() StreamInterceptor` -- Extracts and injects gRPC metadata.
- `LoggingInterceptor(logFunc) StreamInterceptor` -- Logs stream method calls with errors.
- `NewHealthServer() *HealthServer` -- Creates a health server.
- `HealthServer.SetServiceStatus(service, status)` -- Sets serving status per service.
- `HealthServer.Check(ctx, req)` / `HealthServer.Watch(req, stream)` -- Health check RPCs.
- `RegisterHealthServer(s *grpc.Server, hs *HealthServer)` -- Registers with a gRPC server.
- `ServerOptions(config *Config) []grpc.ServerOption` -- Converts config to gRPC server options.

### pkg/webhook -- Webhook Dispatch

Webhook delivery with retry, exponential backoff, HMAC-SHA256 signing, and a subscription registry.

**Types:**
- `Webhook` -- Subscription with URL, Secret, Events filter, and Active flag.
- `Payload` -- Delivery payload with ID, Event type, Data, Timestamp, and Signature.
- `DispatcherConfig` -- MaxRetries (5), Timeout (30s), BackoffBase (1s), BackoffMax (5m), SignatureHeader, UserAgent.
- `Dispatcher` -- Handles delivery with retry logic and delivery statistics.
- `Registry` -- Thread-safe webhook subscription management.

**Key Functions:**
- `Sign(payload []byte, secret string) string` -- Computes HMAC-SHA256 signature.
- `Verify(payload []byte, signature, secret string) bool` -- Verifies a signature.
- `NewDispatcher(config *DispatcherConfig) *Dispatcher` -- Creates a dispatcher.
- `Dispatcher.Send(ctx, webhook, payload) error` -- Delivers with retry and backoff.
- `Dispatcher.Stats() (delivered, failed int64)` -- Returns delivery statistics.
- `NewRegistry() *Registry` -- Creates a webhook registry.
- `Registry.Register(id, webhook)` / `Registry.Unregister(id)` / `Registry.Get(id)`
- `Registry.MatchingWebhooks(event string) []*Webhook` -- Finds active webhooks for an event.

### pkg/http -- HTTP Client

HTTP client with automatic retry, exponential backoff, configurable timeouts, and a three-state circuit breaker (Closed, Open, HalfOpen).

**Types:**
- `Config` -- MaxRetries (3), Timeout (30s), BackoffBase (1s), BackoffMax (30s), CircuitBreakerThreshold (5), CircuitBreakerTimeout (60s).
- `Response` -- Wraps `http.Response` with `BodyBytes()`, `IsSuccess()`, and `IsRetryable()` helpers.
- `CircuitState` -- CircuitClosed, CircuitOpen, CircuitHalfOpen.
- `Client` -- HTTP client with circuit breaker.

**Key Functions:**
- `NewClient(config *Config) *Client` -- Creates a client with circuit breaker.
- `Client.Request(ctx, method, url, body) (*Response, error)` -- Sends a request with retry and circuit breaker.
- `Client.CircuitState() CircuitState` -- Returns current circuit breaker state.

### pkg/transport -- Unified Transport

Protocol-agnostic transport abstraction with a factory for creating transports by type.

**Types:**
- `Type` -- Transport type constants: TypeHTTP, TypeWebSocket, TypeGRPC.
- `Config` -- Type, Address, and Options map.
- `Transport` -- Interface with Send, Receive, and Close methods.
- `Factory` -- Creates Transport instances by registered type.

**Key Functions:**
- `NewFactory() *Factory` -- Creates a factory with built-in transport creators.
- `Factory.Register(t Type, creator func(*Config) (Transport, error))` -- Registers a custom transport.
- `Factory.Create(config *Config) (Transport, error)` -- Creates a transport from config.
- `Factory.SupportedTypes() []Type` -- Lists all registered types.

### pkg/gin -- Gin Framework Adapters

Adapters that bridge SSE and WebSocket to Gin HTTP handlers.

**Functions:**
- `SSEHandler(broker *sse.Broker) gin.HandlerFunc` -- Creates a Gin handler serving SSE.
- `WebSocketHandler(hub *websocket.Hub) gin.HandlerFunc` -- Creates a Gin handler for WebSocket upgrades.
- `HubStatsHandler(hub *websocket.Hub) gin.HandlerFunc` -- Returns JSON stats (connectedClients, rooms).

### pkg/realtime -- File Change Notification

Debounced file change notification with path filtering. Aggregates rapid filesystem events into consolidated batches.

**Types:**
- `ChangeType` -- Created, Modified, Deleted, Renamed.
- `ChangeEvent` -- Path, Type, Timestamp, IsDir, OldPath.
- `EventHandler` -- Callback receiving a batch of `[]ChangeEvent`.
- `Debouncer` -- Aggregates events over a configurable time window.
- `PathFilter` -- Matches paths against include/exclude glob patterns.

**Key Functions:**
- `NewDebouncer(window, handler, logger) *Debouncer` -- Creates a debouncer.
- `Debouncer.Start(ctx) error` -- Starts the debounce loop.
- `Debouncer.Notify(event ChangeEvent) error` -- Submits an event.
- `Debouncer.Stop()` -- Flushes pending events and stops.
- `NewPathFilter(includes, excludes []string) *PathFilter` -- Creates a path filter.
- `PathFilter.Match(path string) bool` -- Tests whether a path passes the filter.

## Usage Examples

### SSE Broadcasting

```go
broker := sse.NewBroker(&sse.Config{
    BufferSize:        100,
    HeartbeatInterval: 30 * time.Second,
    MaxClients:        1000,
})
defer broker.Close()

// Use as http.Handler
http.Handle("/events", broker)

// Broadcast an event
broker.Broadcast(&sse.Event{
    Type: "update",
    Data: []byte(`{"status":"ready"}`),
})
```

### WebSocket with Rooms

```go
hub := websocket.NewHub(nil) // uses DefaultConfig
defer hub.Close()

hub.OnMessage(func(c *websocket.Client, msg *websocket.Message) {
    hub.SendToRoom(msg.Room, msg)
})

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    client, _ := hub.ServeWS(w, r)
    hub.JoinRoom(client.ID(), "lobby")
})
```

### Webhook Dispatch with Signing

```go
dispatcher := webhook.NewDispatcher(nil)
registry := webhook.NewRegistry()

registry.Register("hook-1", &webhook.Webhook{
    URL:    "https://example.com/webhook",
    Secret: "my-secret",
    Active: true,
})

wh, _ := registry.Get("hook-1")
dispatcher.Send(ctx, wh, &webhook.Payload{
    ID:    "evt-1",
    Event: "user.created",
    Data:  map[string]string{"name": "Alice"},
})
```

### HTTP Client with Circuit Breaker

```go
client := http.NewClient(&http.Config{
    MaxRetries:              3,
    CircuitBreakerThreshold: 5,
})

resp, err := client.Request(ctx, "GET", "https://api.example.com/data", nil)
if err == nil && resp.IsSuccess() {
    body, _ := resp.BodyBytes()
    // process body
}
```

## Configuration

All packages use a Config struct with a `DefaultConfig()` constructor providing sensible defaults. Pass `nil` to any `New*` constructor to use defaults.

## Testing

```bash
go test ./... -count=1 -race       # All tests with race detection
go test ./... -short                # Unit tests only
go test -tags=integration ./...    # Integration tests
go test -bench=. ./...             # Benchmarks
```

## Integration with HelixAgent

The Streaming module is used throughout HelixAgent for real-time communication:
- SSE broker powers streaming LLM responses via `/v1/chat/completions` (stream mode)
- WebSocket hub enables real-time notifications and debate session updates
- gRPC streaming supports the gRPC server (`cmd/grpc-server/`)
- Webhook dispatch delivers event notifications to external systems
- HTTP client with circuit breaker handles provider API calls with fault tolerance
- Transport abstraction allows HelixAgent to switch protocols transparently
- The Gin adapters integrate directly with HelixAgent's Gin-based HTTP handlers
- The realtime package powers the Constitution Watcher for detecting project changes

The internal adapter at `internal/adapters/streaming/` bridges these generic types to HelixAgent-specific interfaces.

## License

Proprietary.
