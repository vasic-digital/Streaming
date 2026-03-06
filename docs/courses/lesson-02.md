# Lesson 2: gRPC Streaming and Webhook Dispatch

## Learning Objectives

- Define gRPC streaming server interfaces (unary, server-stream, client-stream, bidirectional)
- Chain stream interceptors for logging and metadata extraction
- Dispatch webhooks with HMAC-SHA256 signing, retry, and exponential backoff

## Key Concepts

- **StreamServer Interface**: Four RPC patterns via channel-based Go interfaces: `Unary(ctx, req) (resp, error)`, `ServerStream(ctx, req, stream)`, `ClientStream(ctx, stream) (resp, error)`, and `BidirectionalStream(ctx, recv, send)`.
- **Interceptor Chaining**: `ChainStreamInterceptors(a, b, c)` composes interceptors similar to middleware chaining. `MetadataInterceptor()` extracts gRPC metadata into context. `LoggingInterceptor(logFunc)` logs method names and errors.
- **Health Server**: Implements the gRPC health check protocol (`grpc_health_v1`) with per-service status tracking and both Check (unary) and Watch (streaming) RPCs.
- **Webhook Signing**: `Sign(payload, secret)` computes `sha256=<hex>` HMAC. `Verify(payload, signature, secret)` uses `hmac.Equal` for constant-time comparison.
- **Retry with Backoff**: The dispatcher retries failed deliveries up to `MaxRetries` with `BackoffBase * 2^(attempt-1)`, capped at `BackoffMax`.

## Code Walkthrough

### Source: `pkg/grpc/grpc.go`

`ChainStreamInterceptors` wraps handlers in reverse order, similar to middleware chaining. `ServerOptions` converts config into `grpc.ServerOption` values for max message sizes and concurrent streams.

### Source: `pkg/webhook/webhook.go`

The `Dispatcher.Send` method marshals the payload, signs it if a secret is configured, then retries delivery with backoff. The `Registry.MatchingWebhooks` method filters active webhooks by event type, supporting wildcard (`*`) subscriptions.

## Practice Exercise

1. Implement a `StreamServer` that echoes requests back in a server-stream RPC. Register it with a gRPC server using `ServerOptions` from config.
2. Chain a logging interceptor and a metadata interceptor. Verify that the logger receives method names for each RPC call.
3. Create a webhook dispatcher, register a webhook with a secret, and send a payload. Verify the request includes the `X-Signature-256` header with a valid HMAC.
