# Getting Started

## Installation

```bash
go get digital.vasic.streaming
```

## SSE Broker

Create an SSE broker and serve events to HTTP clients:

```go
package main

import (
    "net/http"

    "digital.vasic.streaming/pkg/sse"
)

func main() {
    broker := sse.NewBroker(&sse.Config{
        BufferSize:        100,
        HeartbeatInterval: 30 * time.Second,
        MaxClients:        1000,
    })
    defer broker.Close()

    // Serve SSE endpoint
    http.Handle("/events", broker)

    // Broadcast events from elsewhere
    go func() {
        broker.Broadcast(&sse.Event{
            Type: "update",
            Data: []byte(`{"status":"ready"}`),
        })
    }()

    http.ListenAndServe(":8080", nil)
}
```

## WebSocket Hub

Create a WebSocket hub with room support:

```go
import "digital.vasic.streaming/pkg/websocket"

hub := websocket.NewHub(nil) // uses DefaultConfig
defer hub.Close()

// Handle incoming messages
hub.OnMessage(func(client *websocket.Client, msg *websocket.Message) {
    fmt.Printf("Received from %s: %s\n", client.ID(), msg.Type)
})

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    client, err := hub.ServeWS(w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    hub.JoinRoom(client.ID(), "general")
})
```

## Webhook Dispatch

Send webhooks with HMAC-SHA256 signing and retry:

```go
import "digital.vasic.streaming/pkg/webhook"

dispatcher := webhook.NewDispatcher(nil) // uses DefaultDispatcherConfig

wh := &webhook.Webhook{
    URL:    "https://api.example.com/webhook",
    Secret: "my-signing-secret",
    Active: true,
}

payload := &webhook.Payload{
    ID:        "evt-001",
    Event:     "file.created",
    Data:      map[string]string{"path": "/media/movie.mkv"},
    Timestamp: time.Now(),
}

err := dispatcher.Send(context.Background(), wh, payload)
```

## HTTP Client with Circuit Breaker

Make resilient HTTP requests:

```go
import streamhttp "digital.vasic.streaming/pkg/http"

client := streamhttp.NewClient(&streamhttp.Config{
    MaxRetries:              3,
    Timeout:                 10 * time.Second,
    CircuitBreakerThreshold: 5,
    CircuitBreakerTimeout:   60 * time.Second,
})

resp, err := client.Request(ctx, "GET", "https://api.example.com/data", nil)
if err != nil {
    log.Fatal(err)
}
body, _ := resp.BodyBytes()
```
