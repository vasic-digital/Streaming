# Examples

## Real-Time File Change Debouncing

Aggregate file system events and flush them as batches:

```go
package main

import (
    "context"
    "fmt"
    "time"

    "digital.vasic.streaming/pkg/realtime"
)

func main() {
    debouncer := realtime.NewDebouncer(
        500*time.Millisecond,
        func(events []realtime.ChangeEvent) {
            fmt.Printf("Batch of %d events:\n", len(events))
            for _, e := range events {
                fmt.Printf("  %s: %s\n", e.Type, e.Path)
            }
        },
        nil, // uses no-op logger
    )

    debouncer.Start(context.Background())
    defer debouncer.Stop()

    // Rapid events are batched together
    debouncer.Notify(realtime.ChangeEvent{
        Path: "/data/file1.txt", Type: realtime.Created, Timestamp: time.Now(),
    })
    debouncer.Notify(realtime.ChangeEvent{
        Path: "/data/file2.txt", Type: realtime.Modified, Timestamp: time.Now(),
    })

    time.Sleep(time.Second) // wait for flush
}
```

## Path Filtering for Change Events

Filter events by include/exclude glob patterns:

```go
import "digital.vasic.streaming/pkg/realtime"

filter := realtime.NewPathFilter(
    []string{"*.mkv", "*.mp4", "*.avi"},      // include video files
    []string{"*.tmp", "*.partial", "._*"},     // exclude temp files
)

fmt.Println(filter.Match("movie.mkv"))     // true
fmt.Println(filter.Match("download.tmp"))  // false
fmt.Println(filter.Match("photo.jpg"))     // false (not in includes)
```

## Webhook Registry with Event Matching

Manage webhook subscriptions and dispatch to matching endpoints:

```go
import "digital.vasic.streaming/pkg/webhook"

registry := webhook.NewRegistry()

registry.Register("hook-1", &webhook.Webhook{
    URL:    "https://a.example.com/hook",
    Events: []string{"file.created", "file.deleted"},
    Active: true,
    Secret: "secret-a",
})

registry.Register("hook-2", &webhook.Webhook{
    URL:    "https://b.example.com/hook",
    Events: []string{"*"},  // subscribe to all events
    Active: true,
})

// Find webhooks matching an event
matched := registry.MatchingWebhooks("file.created")
fmt.Printf("Matched %d webhooks for file.created\n", len(matched)) // 2

// Verify webhook signatures
payload := []byte(`{"event":"file.created"}`)
sig := webhook.Sign(payload, "secret-a")
valid := webhook.Verify(payload, sig, "secret-a")
fmt.Println("Signature valid:", valid) // true
```
