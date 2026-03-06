# Lesson 1: Server-Sent Events and WebSocket Hub

## Learning Objectives

- Build an SSE broker with heartbeat, client management, and targeted delivery
- Create a WebSocket hub with rooms, message routing, and connection lifecycle
- Use the Gin adapters to integrate SSE and WebSocket into Gin applications

## Key Concepts

- **SSE Broker**: Manages a map of connected clients, each with a buffered channel. `Broadcast()` sends formatted SSE data to all clients. `SendTo()` targets a specific client. The broker runs a background heartbeat loop.
- **SSE Wire Format**: Events are formatted as `id: ...\nevent: ...\nretry: ...\ndata: ...\n\n`. The `Event.Format()` method serializes this.
- **WebSocket Hub**: Uses a channel-based register/unregister pattern processed by a single `run()` goroutine. Each client has separate read and write pump goroutines. Rooms are dynamically created on first join.
- **Ping/Pong**: The write pump sends periodic pings. The read pump sets a pong handler that resets the read deadline. If pong is not received within `PongWait`, the connection is considered dead.

## Code Walkthrough

### Source: `pkg/sse/sse.go`

The broker's `AddClient` method sets SSE headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`), registers the client, and spawns goroutines for event streaming and disconnect detection.

### Source: `pkg/websocket/websocket.go`

`ServeWS` upgrades the HTTP connection, creates a client with send channel, registers it with the hub, and starts the read/write pump goroutines. The `OnMessage` callback receives parsed `Message` structs with type, room, data, and sender fields.

### Source: `pkg/gin/gin.go`

`SSEHandler(broker)` delegates to `broker.ServeHTTP`. `WebSocketHandler(hub)` calls `hub.ServeWS`. `HubStatsHandler(hub)` returns JSON with client and room counts.

## Practice Exercise

1. Create an SSE broker, add a test client via `httptest.NewRecorder`, broadcast an event, and verify the client receives the formatted SSE data.
2. Create a WebSocket hub, register an `OnMessage` handler, and verify that messages from one client can be broadcast to all others.
3. Use `hub.JoinRoom` to add clients to rooms. Send a message to a specific room and verify only room members receive it.
