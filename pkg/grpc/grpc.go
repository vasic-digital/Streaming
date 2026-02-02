// Package grpc provides gRPC streaming utilities including server interfaces,
// stream interceptors, configuration, and health check server.
package grpc

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Config holds gRPC server configuration.
type Config struct {
	// Address is the server listen address (e.g., ":50051").
	Address string
	// TLSCertFile is the path to the TLS certificate file.
	TLSCertFile string
	// TLSKeyFile is the path to the TLS private key file.
	TLSKeyFile string
	// MaxRecvMsgSize is the maximum message size the server can receive.
	MaxRecvMsgSize int
	// MaxSendMsgSize is the maximum message size the server can send.
	MaxSendMsgSize int
	// MaxConcurrentStreams is the maximum number of concurrent streams.
	MaxConcurrentStreams uint32
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Address:              ":50051",
		MaxRecvMsgSize:       4 * 1024 * 1024, // 4MB
		MaxSendMsgSize:       4 * 1024 * 1024, // 4MB
		MaxConcurrentStreams: 100,
	}
}

// StreamServer defines the interface for gRPC streaming servers.
type StreamServer interface {
	// Unary handles a unary (single request, single response) RPC.
	Unary(ctx context.Context, req []byte) ([]byte, error)
	// ServerStream handles a server-streaming RPC.
	ServerStream(ctx context.Context, req []byte, stream chan<- []byte) error
	// ClientStream handles a client-streaming RPC.
	ClientStream(ctx context.Context, stream <-chan []byte) ([]byte, error)
	// BidirectionalStream handles a bidirectional-streaming RPC.
	BidirectionalStream(
		ctx context.Context,
		recv <-chan []byte,
		send chan<- []byte,
	) error
}

// StreamInterceptor is a function that intercepts gRPC stream operations.
type StreamInterceptor func(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error

// ChainStreamInterceptors chains multiple StreamInterceptors into one.
func ChainStreamInterceptors(
	interceptors ...StreamInterceptor,
) StreamInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(srv interface{}, ss grpc.ServerStream) error {
				return interceptor(srv, ss, info, next)
			}
		}
		return chain(srv, ss)
	}
}

// MetadataInterceptor creates an interceptor that extracts metadata
// and injects it into the context.
func MetadataInterceptor() StreamInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			md = metadata.MD{}
		}
		ctx := metadata.NewIncomingContext(ss.Context(), md)
		wrapped := &wrappedStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// LoggingInterceptor creates an interceptor that logs stream method calls.
// The logFunc receives the method name and any error.
func LoggingInterceptor(
	logFunc func(method string, err error),
) StreamInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := handler(srv, ss)
		if logFunc != nil {
			logFunc(info.FullMethod, err)
		}
		return err
	}
}

// wrappedStream wraps grpc.ServerStream with a custom context.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context.
func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

// HealthServer implements the gRPC health check protocol.
type HealthServer struct {
	grpc_health_v1.UnimplementedHealthServer

	mu       sync.RWMutex
	services map[string]grpc_health_v1.HealthCheckResponse_ServingStatus
}

// NewHealthServer creates a new HealthServer.
func NewHealthServer() *HealthServer {
	return &HealthServer{
		services: make(
			map[string]grpc_health_v1.HealthCheckResponse_ServingStatus,
		),
	}
}

// SetServiceStatus sets the serving status for a service.
func (h *HealthServer) SetServiceStatus(
	service string,
	status grpc_health_v1.HealthCheckResponse_ServingStatus,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.services[service] = status
}

// Check implements the health check RPC.
func (h *HealthServer) Check(
	ctx context.Context,
	req *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	service := req.GetService()

	// Empty service name means overall health
	if service == "" {
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_SERVING,
		}, nil
	}

	s, ok := h.services[service]
	if !ok {
		return nil, status.Error(
			codes.NotFound,
			fmt.Sprintf("unknown service: %s", service),
		)
	}

	return &grpc_health_v1.HealthCheckResponse{Status: s}, nil
}

// Watch implements the streaming health check RPC.
func (h *HealthServer) Watch(
	req *grpc_health_v1.HealthCheckRequest,
	stream grpc_health_v1.Health_WatchServer,
) error {
	service := req.GetService()

	h.mu.RLock()
	s, ok := h.services[service]
	h.mu.RUnlock()

	if !ok && service != "" {
		return status.Error(
			codes.NotFound,
			fmt.Sprintf("unknown service: %s", service),
		)
	}

	if service == "" {
		s = grpc_health_v1.HealthCheckResponse_SERVING
	}

	return stream.Send(&grpc_health_v1.HealthCheckResponse{Status: s})
}

// RegisterHealthServer registers the HealthServer with a gRPC server.
func RegisterHealthServer(s *grpc.Server, hs *HealthServer) {
	grpc_health_v1.RegisterHealthServer(s, hs)
}

// ServerOptions returns gRPC server options derived from the Config.
func ServerOptions(config *Config) []grpc.ServerOption {
	if config == nil {
		config = DefaultConfig()
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(config.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(config.MaxSendMsgSize),
		grpc.MaxConcurrentStreams(config.MaxConcurrentStreams),
	}

	return opts
}
