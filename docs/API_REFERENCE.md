# API Reference

Complete reference for all exported types, functions, and methods in `digital.vasic.streaming`.

---

## Package `sse`

**Import**: `digital.vasic.streaming/pkg/sse`

Server-Sent Events broker for managing client connections, broadcasting events, and automatic heartbeats.

### Types

#### `Event`

```go
type Event struct {
    ID    string  // Optional event ID for reconnection tracking
    Type  string  // Event type (maps to SSE "event:" field)
    Data  []byte  // Event payload
    Retry int     // Optional reconnection time in milliseconds
}
```

##### Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Format` | `(e *Event) Format() []byte` | Serializes the Event into SSE wire format with `id:`, `event:`, `retry:`, and `data:` fields. |

#### `Client`

```go
type Client struct {
    ID          string      // Unique identifier for this client
    Channel     chan []byte  // Channel for sending events to this client
    LastEventID string      // Last event ID received, for reconnection
    CreatedAt   time.Time   // When the client connected
}
```

#### `Config`

```go
type Config struct {
    BufferSize        int            // Channel buffer size per client (default: 100)
    HeartbeatInterval time.Duration  // Heartbeat event interval (default: 30s)
    MaxClients        int            // Maximum concurrent clients (default: 1000)
}
```

#### `Broker`

Manages SSE client connections and broadcasts events. Implements `http.Handler`.

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | Returns a Config with defaults: BufferSize=100, HeartbeatInterval=30s, MaxClients=1000. |
| `NewBroker` | `NewBroker(config *Config) *Broker` | Creates a new Broker. If config is nil, DefaultConfig is used. Starts the heartbeat goroutine. |

### Broker Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `AddClient` | `(b *Broker) AddClient(w http.ResponseWriter, r *http.Request) (*Client, error)` | Upgrades an HTTP connection to SSE and registers a new client. Returns error if streaming is unsupported or max clients reached. Sets SSE headers (`text/event-stream`, `no-cache`, `keep-alive`). Reads `X-Client-ID` and `Last-Event-ID` headers. |
| `RemoveClient` | `(b *Broker) RemoveClient(id string)` | Disconnects and removes a client by ID. Closes the client channel. No-op if client does not exist. |
| `Broadcast` | `(b *Broker) Broadcast(event *Event)` | Sends an event to all connected clients. Skips clients with full buffers (non-blocking). |
| `SendTo` | `(b *Broker) SendTo(clientID string, event *Event) error` | Sends an event to a specific client by ID. Returns error if client not found or buffer full. |
| `ClientCount` | `(b *Broker) ClientCount() int` | Returns the number of connected clients. |
| `Close` | `(b *Broker) Close()` | Stops the broker, waits for the heartbeat goroutine to exit, and disconnects all clients. |
| `ServeHTTP` | `(b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request)` | Implements `http.Handler`. Calls AddClient and blocks until the client disconnects. Returns HTTP 503 on error. |

---

## Package `websocket`

**Import**: `digital.vasic.streaming/pkg/websocket`

WebSocket hub with support for rooms, read/write pumps, ping/pong, and message broadcasting.

### Types

#### `Message`

```go
type Message struct {
    Type   string          `json:"type"`            // Message type identifier
    Room   string          `json:"room,omitempty"`  // Optional room target
    Data   json.RawMessage `json:"data,omitempty"`  // Message payload
    Sender string          `json:"sender,omitempty"` // Client ID of sender
}
```

#### `Config`

```go
type Config struct {
    ReadBufferSize  int           // WebSocket read buffer (default: 1024)
    WriteBufferSize int           // WebSocket write buffer (default: 1024)
    PingInterval    time.Duration // Ping interval (default: 54s)
    PongWait        time.Duration // Pong wait timeout (default: 60s)
    WriteWait       time.Duration // Write deadline (default: 10s)
    MaxMessageSize  int64         // Max message size in bytes (default: 512KB)
    AllowedOrigins  []string      // Allowed origins for CORS (default: ["*"])
}
```

#### `Client`

Represents a WebSocket client connection. Fields are unexported; use methods to interact.

##### Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `ID` | `(c *Client) ID() string` | Returns the client identifier. |
| `Send` | `(c *Client) Send(data []byte) error` | Queues data to be sent to the client. Returns error if client is closed or send buffer is full. |
| `Close` | `(c *Client) Close() error` | Closes the client connection. Idempotent -- safe to call multiple times. |

#### `Room`

Represents a group of clients. Created internally; not directly constructed by users.

#### `Hub`

Manages WebSocket connections, rooms, and broadcasting.

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | Returns a Config with sensible defaults. |
| `NewRoom` | `NewRoom(name string) *Room` | Creates a new Room. Used internally by the Hub. |
| `NewHub` | `NewHub(config *Config) *Hub` | Creates a new Hub. If config is nil, DefaultConfig is used. Starts the run goroutine. |

### Hub Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `OnMessage` | `(h *Hub) OnMessage(handler func(*Client, *Message))` | Sets the handler for incoming messages from any client. |
| `ServeWS` | `(h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) (*Client, error)` | Handles WebSocket connection upgrade. Reads `X-Client-ID` header for custom IDs. Starts read and write pumps. |
| `Broadcast` | `(h *Hub) Broadcast(msg *Message) error` | Sends a message to all connected clients. Returns error if JSON marshaling fails. |
| `SendToRoom` | `(h *Hub) SendToRoom(roomName string, msg *Message) error` | Sends a message to all clients in a room. Returns error if room not found or JSON marshaling fails. |
| `JoinRoom` | `(h *Hub) JoinRoom(clientID, roomName string) error` | Adds a client to a room. Creates the room if it does not exist. Returns error if client not found. |
| `LeaveRoom` | `(h *Hub) LeaveRoom(clientID, roomName string) error` | Removes a client from a room. Returns error if room not found. |
| `ClientCount` | `(h *Hub) ClientCount() int` | Returns the number of connected clients. |
| `RoomCount` | `(h *Hub) RoomCount() int` | Returns the number of rooms. |
| `RoomClientCount` | `(h *Hub) RoomClientCount(roomName string) int` | Returns the number of clients in a room. Returns 0 if room does not exist. |
| `Close` | `(h *Hub) Close()` | Stops the hub, waits for the run goroutine to exit, and closes all client connections. |

---

## Package `grpc`

**Import**: `digital.vasic.streaming/pkg/grpc`

gRPC streaming utilities including server interfaces, stream interceptors, and health check server.

### Types

#### `Config`

```go
type Config struct {
    Address              string  // Listen address (default: ":50051")
    TLSCertFile          string  // TLS certificate file path
    TLSKeyFile           string  // TLS private key file path
    MaxRecvMsgSize       int     // Max receive message size (default: 4MB)
    MaxSendMsgSize       int     // Max send message size (default: 4MB)
    MaxConcurrentStreams uint32   // Max concurrent streams (default: 100)
}
```

#### `StreamServer` (interface)

```go
type StreamServer interface {
    Unary(ctx context.Context, req []byte) ([]byte, error)
    ServerStream(ctx context.Context, req []byte, stream chan<- []byte) error
    ClientStream(ctx context.Context, stream <-chan []byte) ([]byte, error)
    BidirectionalStream(ctx context.Context, recv <-chan []byte, send chan<- []byte) error
}
```

#### `StreamInterceptor` (function type)

```go
type StreamInterceptor func(
    srv interface{},
    ss grpc.ServerStream,
    info *grpc.StreamServerInfo,
    handler grpc.StreamHandler,
) error
```

#### `HealthServer`

Implements the gRPC health check protocol (`grpc.health.v1.Health`).

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | Returns a Config with defaults: Address=":50051", 4MB messages, 100 streams. |
| `ChainStreamInterceptors` | `ChainStreamInterceptors(interceptors ...StreamInterceptor) StreamInterceptor` | Chains multiple StreamInterceptors into a single interceptor. First argument executes first. |
| `MetadataInterceptor` | `MetadataInterceptor() StreamInterceptor` | Creates an interceptor that extracts gRPC metadata and injects it into the stream context. |
| `LoggingInterceptor` | `LoggingInterceptor(logFunc func(method string, err error)) StreamInterceptor` | Creates an interceptor that calls logFunc with the method name and any error after each stream handler completes. |
| `NewHealthServer` | `NewHealthServer() *HealthServer` | Creates a new HealthServer with an empty service map. |
| `RegisterHealthServer` | `RegisterHealthServer(s *grpc.Server, hs *HealthServer)` | Registers the HealthServer with a gRPC server. |
| `ServerOptions` | `ServerOptions(config *Config) []grpc.ServerOption` | Returns gRPC server options (MaxRecvMsgSize, MaxSendMsgSize, MaxConcurrentStreams) from the Config. |

### HealthServer Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `SetServiceStatus` | `(h *HealthServer) SetServiceStatus(service string, status grpc_health_v1.HealthCheckResponse_ServingStatus)` | Sets the serving status for a named service. |
| `Check` | `(h *HealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error)` | Unary health check RPC. Empty service name returns SERVING. Unknown service returns NotFound. |
| `Watch` | `(h *HealthServer) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error` | Streaming health check RPC. Sends the current status and returns. |

---

## Package `webhook`

**Import**: `digital.vasic.streaming/pkg/webhook`

Webhook dispatch with retry, exponential backoff, HMAC-SHA256 signing, and a subscription registry.

### Types

#### `Webhook`

```go
type Webhook struct {
    URL    string   `json:"url"`              // Endpoint URL
    Secret string   `json:"secret,omitempty"` // HMAC signing key
    Events []string `json:"events,omitempty"` // Subscribed event types (empty = all)
    Active bool     `json:"active"`           // Whether webhook is enabled
}
```

#### `Payload`

```go
type Payload struct {
    ID        string      `json:"id"`                  // Unique delivery ID
    Event     string      `json:"event"`               // Event type
    Data      interface{} `json:"data"`                 // Event payload
    Timestamp time.Time   `json:"timestamp"`            // Generation time
    Signature string      `json:"signature,omitempty"`  // HMAC-SHA256 signature
}
```

#### `DispatcherConfig`

```go
type DispatcherConfig struct {
    MaxRetries      int           // Max retry attempts (default: 5)
    Timeout         time.Duration // HTTP request timeout (default: 30s)
    BackoffBase     time.Duration // Backoff base duration (default: 1s)
    BackoffMax      time.Duration // Max backoff duration (default: 5m)
    SignatureHeader string        // Signature header name (default: "X-Signature-256")
    UserAgent       string        // User-Agent header (default: "Streaming-Webhook/1.0")
}
```

#### `Dispatcher`

Handles webhook delivery with retry logic.

#### `Registry`

Manages webhook subscriptions.

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `DefaultDispatcherConfig` | `DefaultDispatcherConfig() *DispatcherConfig` | Returns defaults: 5 retries, 30s timeout, 1s backoff, 5m max backoff. |
| `Sign` | `Sign(payload []byte, secret string) string` | Computes HMAC-SHA256 signature. Returns `"sha256=<hex>"`. |
| `Verify` | `Verify(payload []byte, signature, secret string) bool` | Checks that the signature matches the payload and secret using constant-time comparison. |
| `NewDispatcher` | `NewDispatcher(config *DispatcherConfig) *Dispatcher` | Creates a new Dispatcher. If config is nil, DefaultDispatcherConfig is used. |
| `NewRegistry` | `NewRegistry() *Registry` | Creates a new empty webhook Registry. |

### Dispatcher Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Send` | `(d *Dispatcher) Send(ctx context.Context, webhook *Webhook, payload *Payload) error` | Delivers a payload to a webhook with retry and backoff. Signs the payload if webhook has a secret. Returns error if webhook is inactive, context cancelled, or all attempts fail. |
| `Stats` | `(d *Dispatcher) Stats() (delivered, failed int64)` | Returns cumulative delivery and failure counts. |

### Registry Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Register` | `(r *Registry) Register(id string, webhook *Webhook)` | Adds a webhook subscription with the given ID. Overwrites if ID exists. |
| `Unregister` | `(r *Registry) Unregister(id string)` | Removes a webhook subscription by ID. No-op if not found. |
| `Get` | `(r *Registry) Get(id string) (*Webhook, bool)` | Returns a webhook by ID and whether it was found. |
| `List` | `(r *Registry) List() map[string]*Webhook` | Returns a copy of all registered webhooks. |
| `Count` | `(r *Registry) Count() int` | Returns the number of registered webhooks. |
| `MatchingWebhooks` | `(r *Registry) MatchingWebhooks(event string) []*Webhook` | Returns all active webhooks subscribed to the given event. Webhooks with empty Events match all events. Supports wildcard `"*"`. |

---

## Package `http`

**Import**: `digital.vasic.streaming/pkg/http`

HTTP client with automatic retry, exponential backoff, and circuit breaker.

### Types

#### `Config`

```go
type Config struct {
    MaxRetries              int           // Max retry attempts (default: 3)
    Timeout                 time.Duration // Per-request timeout (default: 30s)
    BackoffBase             time.Duration // Backoff base (default: 1s)
    BackoffMax              time.Duration // Max backoff (default: 30s)
    CircuitBreakerThreshold int           // Failures to open circuit (default: 5)
    CircuitBreakerTimeout   time.Duration // Open circuit duration (default: 60s)
}
```

#### `Response`

```go
type Response struct {
    *http.Response  // Embedded standard response
}
```

##### Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `BodyBytes` | `(r *Response) BodyBytes() ([]byte, error)` | Reads and returns the entire response body. Closes the body after reading. |
| `IsSuccess` | `(r *Response) IsSuccess() bool` | Returns true if status code is 2xx. |
| `IsRetryable` | `(r *Response) IsRetryable() bool` | Returns true for 429, 502, 503, 504 status codes. |

#### `CircuitState`

```go
type CircuitState int

const (
    CircuitClosed   CircuitState = iota  // Requests allowed
    CircuitOpen                          // Requests blocked
    CircuitHalfOpen                      // Single test request allowed
)
```

#### `Client`

HTTP client with retry, timeout, and circuit breaker.

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `DefaultConfig` | `DefaultConfig() *Config` | Returns defaults: 3 retries, 30s timeout, 1s backoff, 5 failure threshold, 60s circuit timeout. |
| `NewClient` | `NewClient(config *Config) *Client` | Creates a new Client. If config is nil, DefaultConfig is used. |

### Client Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Request` | `(c *Client) Request(ctx context.Context, method, url string, body io.Reader) (*Response, error)` | Sends an HTTP request with automatic retry and circuit breaker. Retries on retryable status codes. Non-retryable errors (4xx except 429) are returned immediately. Returns error if all attempts fail or context is cancelled. |
| `CircuitState` | `(c *Client) CircuitState() CircuitState` | Returns the current circuit breaker state (Closed, Open, or HalfOpen). |

---

## Package `transport`

**Import**: `digital.vasic.streaming/pkg/transport`

Unified transport abstraction layer supporting multiple protocols.

### Types

#### `Type`

```go
type Type string

const (
    TypeHTTP      Type = "http"
    TypeWebSocket Type = "websocket"
    TypeGRPC      Type = "grpc"
)
```

#### `Config`

```go
type Config struct {
    Type    Type                    // Transport protocol type
    Address string                  // Connection address
    Options map[string]interface{}  // Transport-specific options
}
```

#### `Transport` (interface)

```go
type Transport interface {
    Send(ctx context.Context, data []byte) error
    Receive(ctx context.Context) ([]byte, error)
    Close() error
}
```

#### `Factory`

Creates Transport instances by type.

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewFactory` | `NewFactory() *Factory` | Creates a new transport Factory with built-in creators for HTTP, WebSocket, and gRPC types (all use channel-based transport by default). |

### Factory Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Register` | `(f *Factory) Register(t Type, creator func(config *Config) (Transport, error))` | Registers a transport creator for a given type. Overwrites existing creators. |
| `Create` | `(f *Factory) Create(config *Config) (Transport, error)` | Creates a new Transport for the given configuration. Returns error if config is nil or type is unsupported. |
| `SupportedTypes` | `(f *Factory) SupportedTypes() []Type` | Returns all registered transport types. |
