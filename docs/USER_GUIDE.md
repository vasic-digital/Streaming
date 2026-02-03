# User Guide

## Installation

```bash
go get digital.vasic.streaming
```

Requires Go 1.24 or later.

## Package Overview

| Package | Import Path | Purpose |
|---------|-------------|---------|
| sse | `digital.vasic.streaming/pkg/sse` | Server-Sent Events broker |
| websocket | `digital.vasic.streaming/pkg/websocket` | WebSocket hub with rooms |
| grpc | `digital.vasic.streaming/pkg/grpc` | gRPC streaming utilities |
| webhook | `digital.vasic.streaming/pkg/webhook` | Webhook dispatch with retry |
| http | `digital.vasic.streaming/pkg/http` | HTTP client with circuit breaker |
| transport | `digital.vasic.streaming/pkg/transport` | Unified transport abstraction |

---

## Server-Sent Events (SSE)

The `sse` package provides a broker that manages client connections, broadcasts events, and sends automatic heartbeats.

### Creating a Broker

```go
import "digital.vasic.streaming/pkg/sse"

// With default configuration (100 buffer, 30s heartbeat, 1000 max clients)
broker := sse.NewBroker(nil)
defer broker.Close()

// With custom configuration
broker := sse.NewBroker(&sse.Config{
    BufferSize:        256,
    HeartbeatInterval: 15 * time.Second,
    MaxClients:        5000,
})
defer broker.Close()
```

### Handling Client Connections

The broker implements `http.Handler`, so you can mount it directly on a router:

```go
http.Handle("/events", broker)
```

Or use `AddClient` for more control:

```go
http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
    client, err := broker.AddClient(w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusServiceUnavailable)
        return
    }
    // Client is now streaming. Block until disconnect.
    <-r.Context().Done()
    _ = client
})
```

Clients can set `X-Client-ID` header to specify their ID, and `Last-Event-ID` for reconnection tracking.

### Broadcasting Events

```go
// Broadcast to all connected clients
broker.Broadcast(&sse.Event{
    ID:   "1",
    Type: "message",
    Data: []byte(`{"text": "Hello, world!"}`),
})

// Send to a specific client
err := broker.SendTo("client-123", &sse.Event{
    Type: "notification",
    Data: []byte(`{"alert": "You have a new message"}`),
})

// Set retry interval for client reconnection
broker.Broadcast(&sse.Event{
    Type:  "update",
    Data:  []byte(`{"status": "changed"}`),
    Retry: 5000, // 5 seconds
})
```

### Event Wire Format

The `Event.Format()` method serializes events into the SSE wire format:

```
id: 1
event: message
retry: 5000
data: {"text": "Hello, world!"}

```

### Monitoring

```go
count := broker.ClientCount()
fmt.Printf("Connected clients: %d\n", count)
```

---

## WebSocket

The `websocket` package provides a hub for managing WebSocket connections with support for rooms, message broadcasting, and automatic ping/pong.

### Creating a Hub

```go
import "digital.vasic.streaming/pkg/websocket"

// With defaults (1024 buffers, 54s ping, 60s pong, 10s write, 512KB max, all origins)
hub := websocket.NewHub(nil)
defer hub.Close()

// With custom configuration
hub := websocket.NewHub(&websocket.Config{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    PingInterval:    30 * time.Second,
    PongWait:        35 * time.Second,
    WriteWait:       5 * time.Second,
    MaxMessageSize:  1024 * 1024, // 1MB
    AllowedOrigins:  []string{"https://example.com"},
})
defer hub.Close()
```

### Handling Connections

```go
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    client, err := hub.ServeWS(w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    fmt.Printf("Client connected: %s\n", client.ID())
})
```

Clients can set `X-Client-ID` header to specify a custom client ID. Otherwise, an auto-incrementing ID (`ws-1`, `ws-2`, etc.) is assigned.

### Handling Messages

```go
hub.OnMessage(func(client *websocket.Client, msg *websocket.Message) {
    fmt.Printf("Received from %s: type=%s room=%s\n",
        msg.Sender, msg.Type, msg.Room)

    // Echo back to all clients
    hub.Broadcast(msg)
})
```

The `Message` struct uses JSON serialization:

```go
type Message struct {
    Type   string          `json:"type"`
    Room   string          `json:"room,omitempty"`
    Data   json.RawMessage `json:"data,omitempty"`
    Sender string          `json:"sender,omitempty"`
}
```

### Rooms

```go
// Join a room
hub.JoinRoom("ws-1", "chat-general")

// Send to a specific room
hub.SendToRoom("chat-general", &websocket.Message{
    Type: "chat",
    Data: json.RawMessage(`{"text": "Hello room!"}`),
})

// Leave a room
hub.LeaveRoom("ws-1", "chat-general")
```

Rooms are created automatically when the first client joins. Clients are automatically removed from all rooms when they disconnect.

### Broadcasting

```go
// Broadcast to all connected clients
hub.Broadcast(&websocket.Message{
    Type: "announcement",
    Data: json.RawMessage(`{"text": "Server maintenance in 5 minutes"}`),
})
```

### Monitoring

```go
fmt.Printf("Clients: %d\n", hub.ClientCount())
fmt.Printf("Rooms: %d\n", hub.RoomCount())
fmt.Printf("Clients in 'lobby': %d\n", hub.RoomClientCount("lobby"))
```

### Sending to Individual Clients

```go
err := client.Send([]byte(`{"type":"ping"}`))
if err != nil {
    fmt.Printf("Client send failed: %v\n", err)
}
```

---

## gRPC Streaming

The `grpc` package provides streaming server interfaces, interceptor chaining, metadata extraction, logging, and a health check server.

### StreamServer Interface

Implement the `StreamServer` interface to handle all four gRPC call types:

```go
import streamgrpc "digital.vasic.streaming/pkg/grpc"

type MyServer struct{}

func (s *MyServer) Unary(
    ctx context.Context, req []byte,
) ([]byte, error) {
    // Process single request, return single response
    return []byte("response"), nil
}

func (s *MyServer) ServerStream(
    ctx context.Context, req []byte, stream chan<- []byte,
) error {
    // Send multiple responses for a single request
    for i := 0; i < 10; i++ {
        stream <- []byte(fmt.Sprintf("chunk-%d", i))
    }
    return nil
}

func (s *MyServer) ClientStream(
    ctx context.Context, stream <-chan []byte,
) ([]byte, error) {
    // Receive multiple requests, return single response
    var total int
    for data := range stream {
        total += len(data)
    }
    return []byte(fmt.Sprintf("received %d bytes", total)), nil
}

func (s *MyServer) BidirectionalStream(
    ctx context.Context, recv <-chan []byte, send chan<- []byte,
) error {
    // Bidirectional streaming: receive and send concurrently
    for data := range recv {
        send <- append([]byte("echo: "), data...)
    }
    return nil
}
```

### Server Configuration

```go
import streamgrpc "digital.vasic.streaming/pkg/grpc"

// Default: :50051, 4MB messages, 100 concurrent streams
config := streamgrpc.DefaultConfig()

// Custom configuration
config := &streamgrpc.Config{
    Address:              ":9090",
    TLSCertFile:          "/path/to/cert.pem",
    TLSKeyFile:           "/path/to/key.pem",
    MaxRecvMsgSize:       16 * 1024 * 1024, // 16MB
    MaxSendMsgSize:       16 * 1024 * 1024,
    MaxConcurrentStreams: 500,
}

// Get gRPC server options from config
opts := streamgrpc.ServerOptions(config)
server := grpc.NewServer(opts...)
```

### Stream Interceptors

```go
// Logging interceptor
logger := streamgrpc.LoggingInterceptor(func(method string, err error) {
    if err != nil {
        log.Printf("ERROR %s: %v", method, err)
    } else {
        log.Printf("OK %s", method)
    }
})

// Metadata interceptor (extracts metadata into context)
metadata := streamgrpc.MetadataInterceptor()

// Chain multiple interceptors
chained := streamgrpc.ChainStreamInterceptors(metadata, logger)
```

### Health Check Server

```go
healthServer := streamgrpc.NewHealthServer()

// Register with gRPC server
streamgrpc.RegisterHealthServer(grpcServer, healthServer)

// Update service status
healthServer.SetServiceStatus(
    "my.service.v1",
    grpc_health_v1.HealthCheckResponse_SERVING,
)

// Mark a service as not serving
healthServer.SetServiceStatus(
    "my.service.v1",
    grpc_health_v1.HealthCheckResponse_NOT_SERVING,
)
```

The health server supports both unary `Check` and streaming `Watch` RPCs. An empty service name returns `SERVING` for overall health.

---

## Webhooks

The `webhook` package provides webhook dispatch with retry, exponential backoff, and HMAC-SHA256 signing, plus a subscription registry.

### Webhook Registry

```go
import "digital.vasic.streaming/pkg/webhook"

registry := webhook.NewRegistry()

// Register a webhook
registry.Register("wh-1", &webhook.Webhook{
    URL:    "https://example.com/hooks/events",
    Secret: "my-secret-key",
    Events: []string{"user.created", "user.deleted"},
    Active: true,
})

// Register a webhook that receives all events
registry.Register("wh-2", &webhook.Webhook{
    URL:    "https://logging.example.com/hooks",
    Active: true,
    // Empty Events means all events
})

// Look up a webhook
wh, exists := registry.Get("wh-1")

// List all webhooks
all := registry.List()

// Find matching webhooks for an event
matched := registry.MatchingWebhooks("user.created")

// Unregister
registry.Unregister("wh-1")

// Count registered webhooks
count := registry.Count()
```

### Dispatching Events

```go
dispatcher := webhook.NewDispatcher(nil) // uses defaults

// Or with custom config
dispatcher := webhook.NewDispatcher(&webhook.DispatcherConfig{
    MaxRetries:      3,
    Timeout:         10 * time.Second,
    BackoffBase:     500 * time.Millisecond,
    BackoffMax:      2 * time.Minute,
    SignatureHeader: "X-Hub-Signature-256",
    UserAgent:       "MyApp-Webhook/1.0",
})

// Dispatch to a specific webhook
err := dispatcher.Send(ctx, wh, &webhook.Payload{
    ID:        "evt-123",
    Event:     "user.created",
    Data:      map[string]string{"user_id": "42", "name": "Alice"},
    Timestamp: time.Now(),
})
```

### HMAC-SHA256 Signing and Verification

```go
payload := []byte(`{"event":"user.created","data":{"id":"42"}}`)
secret := "my-secret"

// Sign
signature := webhook.Sign(payload, secret)
// Returns: "sha256=abc123..."

// Verify
valid := webhook.Verify(payload, signature, secret)
```

### Delivery Statistics

```go
delivered, failed := dispatcher.Stats()
fmt.Printf("Delivered: %d, Failed: %d\n", delivered, failed)
```

### Complete Dispatch Flow

```go
registry := webhook.NewRegistry()
dispatcher := webhook.NewDispatcher(nil)

// Register webhooks
registry.Register("slack", &webhook.Webhook{
    URL:    "https://hooks.slack.com/...",
    Secret: "slack-secret",
    Events: []string{"deploy.success", "deploy.failure"},
    Active: true,
})

// When an event occurs, dispatch to all matching webhooks
event := "deploy.success"
payload := &webhook.Payload{
    ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
    Event:     event,
    Data:      map[string]string{"version": "1.2.3"},
    Timestamp: time.Now(),
}

for _, wh := range registry.MatchingWebhooks(event) {
    if err := dispatcher.Send(ctx, wh, payload); err != nil {
        log.Printf("Webhook delivery failed: %v", err)
    }
}
```

---

## HTTP Client

The `http` package provides an HTTP client with automatic retry, exponential backoff, and a circuit breaker.

### Creating a Client

```go
import streamhttp "digital.vasic.streaming/pkg/http"

// Default: 3 retries, 30s timeout, 1s backoff base, 5 failures to open circuit
client := streamhttp.NewClient(nil)

// Custom configuration
client := streamhttp.NewClient(&streamhttp.Config{
    MaxRetries:              5,
    Timeout:                 10 * time.Second,
    BackoffBase:             500 * time.Millisecond,
    BackoffMax:              15 * time.Second,
    CircuitBreakerThreshold: 3,
    CircuitBreakerTimeout:   30 * time.Second,
})
```

### Making Requests

```go
ctx := context.Background()

// GET request
resp, err := client.Request(ctx, http.MethodGet, "https://api.example.com/data", nil)
if err != nil {
    log.Fatal(err)
}

// Check success
if resp.IsSuccess() {
    body, err := resp.BodyBytes()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(body))
}

// POST request with body
body := strings.NewReader(`{"key": "value"}`)
resp, err := client.Request(ctx, http.MethodPost, "https://api.example.com/data", body)
```

### Response Helpers

```go
resp, err := client.Request(ctx, "GET", url, nil)
if err != nil {
    log.Fatal(err)
}

// Check if the request was successful (2xx)
if resp.IsSuccess() {
    data, _ := resp.BodyBytes()
    fmt.Println(string(data))
}

// Check if the error is retryable (429, 502, 503, 504)
if resp.IsRetryable() {
    log.Println("Server is having issues, will retry")
}
```

### Circuit Breaker

The circuit breaker automatically opens after consecutive failures, preventing requests to a failing service:

```go
// Check circuit breaker state
state := client.CircuitState()
switch state {
case streamhttp.CircuitClosed:
    fmt.Println("Circuit is closed, requests allowed")
case streamhttp.CircuitOpen:
    fmt.Println("Circuit is open, requests blocked")
case streamhttp.CircuitHalfOpen:
    fmt.Println("Circuit is half-open, testing with one request")
}
```

Circuit breaker state transitions:
- **Closed** -> **Open**: After `CircuitBreakerThreshold` consecutive failures
- **Open** -> **Half-Open**: After `CircuitBreakerTimeout` elapses
- **Half-Open** -> **Closed**: On a successful request
- **Half-Open** -> **Open**: On a failed request

---

## Transport Abstraction

The `transport` package provides a unified interface for sending and receiving data across different protocols.

### Transport Interface

```go
type Transport interface {
    Send(ctx context.Context, data []byte) error
    Receive(ctx context.Context) ([]byte, error)
    Close() error
}
```

### Using the Factory

```go
import "digital.vasic.streaming/pkg/transport"

factory := transport.NewFactory()

// Create an HTTP transport
t, err := factory.Create(&transport.Config{
    Type:    transport.TypeHTTP,
    Address: "https://api.example.com",
    Options: map[string]interface{}{
        "buffer_size": 512,
    },
})
if err != nil {
    log.Fatal(err)
}
defer t.Close()

// Send data
err = t.Send(ctx, []byte("hello"))

// Receive data (blocks until available or context cancelled)
data, err := t.Receive(ctx)
```

### Supported Transport Types

```go
const (
    TypeHTTP      Type = "http"
    TypeWebSocket Type = "websocket"
    TypeGRPC      Type = "grpc"
)
```

### Registering Custom Transports

```go
factory := transport.NewFactory()

factory.Register(transport.Type("custom"), func(config *transport.Config) (transport.Transport, error) {
    // Create and return your custom transport
    return myCustomTransport(config)
})

// List all supported types
types := factory.SupportedTypes()
for _, t := range types {
    fmt.Println(t)
}
```

### Transport Configuration

The `Config` struct carries protocol-agnostic configuration:

```go
config := &transport.Config{
    Type:    transport.TypeWebSocket,
    Address: "ws://localhost:8080/ws",
    Options: map[string]interface{}{
        "buffer_size": 1024,
        "timeout":     "30s",
    },
}
```

The built-in channel-based transport reads the `buffer_size` option (default 256) from `Options` to configure its internal channel buffer size.
