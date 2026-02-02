# Streaming

Generic, reusable Go module for streaming and transport abstractions.

## Packages

- **pkg/sse** - Server-Sent Events with broker, heartbeat, and reconnection support
- **pkg/websocket** - WebSocket hub with rooms, read/write pumps, and ping/pong
- **pkg/grpc** - gRPC streaming utilities, interceptors, and health checking
- **pkg/webhook** - Webhook dispatch with retry, backoff, and HMAC-SHA256 signing
- **pkg/http** - HTTP client with retry, timeout, and circuit breaker
- **pkg/transport** - Unified transport abstraction layer

## Installation

```bash
go get digital.vasic.streaming
```

## Quick Start

```go
import (
    "digital.vasic.streaming/pkg/sse"
    "digital.vasic.streaming/pkg/websocket"
    "digital.vasic.streaming/pkg/webhook"
)
```

## Testing

```bash
go test ./... -count=1 -race
```

## License

Proprietary.
