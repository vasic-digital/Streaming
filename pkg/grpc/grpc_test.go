package grpc

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx      context.Context
	sentMsgs []interface{}
	recvMsgs []interface{}
	recvIdx  int
	recvErr  error
	sendErr  error
}

func newMockServerStream(ctx context.Context) *mockServerStream {
	return &mockServerStream{
		ctx:      ctx,
		sentMsgs: make([]interface{}, 0),
		recvMsgs: make([]interface{}, 0),
	}
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func (m *mockServerStream) SendMsg(msg interface{}) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func (m *mockServerStream) RecvMsg(msg interface{}) error {
	if m.recvErr != nil {
		return m.recvErr
	}
	if m.recvIdx >= len(m.recvMsgs) {
		return errors.New("no more messages")
	}
	m.recvIdx++
	return nil
}

func (m *mockServerStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockServerStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockServerStream) SetTrailer(metadata.MD) {
}

// mockHealthWatchServer implements grpc_health_v1.Health_WatchServer for testing.
type mockHealthWatchServer struct {
	grpc_health_v1.Health_WatchServer
	ctx       context.Context
	responses []*grpc_health_v1.HealthCheckResponse
	sendErr   error
}

func newMockHealthWatchServer(ctx context.Context) *mockHealthWatchServer {
	return &mockHealthWatchServer{
		ctx:       ctx,
		responses: make([]*grpc_health_v1.HealthCheckResponse, 0),
	}
}

func (m *mockHealthWatchServer) Send(resp *grpc_health_v1.HealthCheckResponse) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockHealthWatchServer) Context() context.Context {
	return m.ctx
}

// =============================================================================
// Config Tests
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, ":50051", config.Address)
	assert.Equal(t, 4*1024*1024, config.MaxRecvMsgSize)
	assert.Equal(t, 4*1024*1024, config.MaxSendMsgSize)
	assert.Equal(t, uint32(100), config.MaxConcurrentStreams)
}

func TestConfig_Fields(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "with TLS",
			config: Config{
				Address:        ":443",
				TLSCertFile:    "/path/to/cert.pem",
				TLSKeyFile:     "/path/to/key.pem",
				MaxRecvMsgSize: 1024,
				MaxSendMsgSize: 1024,
			},
		},
		{
			name: "without TLS",
			config: Config{
				Address:              ":8080",
				MaxRecvMsgSize:       2048,
				MaxSendMsgSize:       2048,
				MaxConcurrentStreams: 50,
			},
		},
		{
			name: "minimal config",
			config: Config{
				Address: ":9090",
			},
		},
		{
			name: "max values",
			config: Config{
				Address:              ":50051",
				MaxRecvMsgSize:       100 * 1024 * 1024,
				MaxSendMsgSize:       100 * 1024 * 1024,
				MaxConcurrentStreams: 10000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.config.Address)
		})
	}
}

// =============================================================================
// ServerOptions Tests
// =============================================================================

func TestServerOptions(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &Config{
				Address:              ":9090",
				MaxRecvMsgSize:       8 * 1024 * 1024,
				MaxSendMsgSize:       8 * 1024 * 1024,
				MaxConcurrentStreams: 200,
			},
		},
		{
			name: "zero values config",
			config: &Config{
				Address:              ":0",
				MaxRecvMsgSize:       0,
				MaxSendMsgSize:       0,
				MaxConcurrentStreams: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServerOptions(tt.config)
			assert.NotEmpty(t, opts)
			assert.Len(t, opts, 3)
		})
	}
}

// =============================================================================
// HealthServer Tests
// =============================================================================

func TestHealthServer_SetServiceStatus(t *testing.T) {
	tests := []struct {
		name    string
		service string
		status  grpc_health_v1.HealthCheckResponse_ServingStatus
	}{
		{
			name:    "set serving",
			service: "svc1",
			status:  grpc_health_v1.HealthCheckResponse_SERVING,
		},
		{
			name:    "set not serving",
			service: "svc2",
			status:  grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
		{
			name:    "set service unknown",
			service: "svc3",
			status:  grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN,
		},
		{
			name:    "empty service name",
			service: "",
			status:  grpc_health_v1.HealthCheckResponse_SERVING,
		},
		{
			name:    "service with special characters",
			service: "my-service.v1",
			status:  grpc_health_v1.HealthCheckResponse_SERVING,
		},
	}

	hs := NewHealthServer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs.SetServiceStatus(tt.service, tt.status)

			hs.mu.RLock()
			s, ok := hs.services[tt.service]
			hs.mu.RUnlock()

			assert.True(t, ok)
			assert.Equal(t, tt.status, s)
		})
	}
}

func TestHealthServer_SetServiceStatus_Update(t *testing.T) {
	hs := NewHealthServer()

	// Set initial status
	hs.SetServiceStatus("svc", grpc_health_v1.HealthCheckResponse_SERVING)

	hs.mu.RLock()
	s1, _ := hs.services["svc"]
	hs.mu.RUnlock()
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, s1)

	// Update status
	hs.SetServiceStatus("svc", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	hs.mu.RLock()
	s2, _ := hs.services["svc"]
	hs.mu.RUnlock()
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, s2)
}

func TestHealthServer_SetServiceStatus_Concurrent(t *testing.T) {
	hs := NewHealthServer()
	var wg sync.WaitGroup

	// Concurrently set statuses
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status := grpc_health_v1.HealthCheckResponse_SERVING
			if i%2 == 0 {
				status = grpc_health_v1.HealthCheckResponse_NOT_SERVING
			}
			hs.SetServiceStatus("concurrent-svc", status)
		}(i)
	}

	wg.Wait()

	// Should have a valid status
	hs.mu.RLock()
	_, ok := hs.services["concurrent-svc"]
	hs.mu.RUnlock()
	assert.True(t, ok)
}

func TestHealthServer_Check(t *testing.T) {
	tests := []struct {
		name           string
		service        string
		setup          func(hs *HealthServer)
		expectedStatus grpc_health_v1.HealthCheckResponse_ServingStatus
		expectErr      bool
		expectedCode   codes.Code
	}{
		{
			name:    "empty service returns SERVING",
			service: "",
			setup:   func(hs *HealthServer) {},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_SERVING,
			expectErr: false,
		},
		{
			name:    "known service SERVING",
			service: "myservice",
			setup: func(hs *HealthServer) {
				hs.SetServiceStatus(
					"myservice",
					grpc_health_v1.HealthCheckResponse_SERVING,
				)
			},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_SERVING,
			expectErr: false,
		},
		{
			name:    "known service NOT_SERVING",
			service: "degraded",
			setup: func(hs *HealthServer) {
				hs.SetServiceStatus(
					"degraded",
					grpc_health_v1.HealthCheckResponse_NOT_SERVING,
				)
			},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_NOT_SERVING,
			expectErr: false,
		},
		{
			name:    "known service SERVICE_UNKNOWN",
			service: "unknown-status",
			setup: func(hs *HealthServer) {
				hs.SetServiceStatus(
					"unknown-status",
					grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN,
				)
			},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_SERVICE_UNKNOWN,
			expectErr: false,
		},
		{
			name:         "unknown service returns NOT_FOUND",
			service:      "unknown",
			setup:        func(hs *HealthServer) {},
			expectErr:    true,
			expectedCode: codes.NotFound,
		},
		{
			name:         "service with special chars not found",
			service:      "service.v1.beta",
			setup:        func(hs *HealthServer) {},
			expectErr:    true,
			expectedCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := NewHealthServer()
			tt.setup(hs)

			resp, err := hs.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{
				Service: tt.service,
			})

			if tt.expectErr {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, resp.Status)
			}
		})
	}
}

func TestHealthServer_Check_Concurrent(t *testing.T) {
	hs := NewHealthServer()
	hs.SetServiceStatus("concurrent", grpc_health_v1.HealthCheckResponse_SERVING)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := hs.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{
				Service: "concurrent",
			})
			assert.NoError(t, err)
			assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
		}()
	}
	wg.Wait()
}

func TestHealthServer_Watch(t *testing.T) {
	tests := []struct {
		name           string
		service        string
		setup          func(hs *HealthServer)
		expectedStatus grpc_health_v1.HealthCheckResponse_ServingStatus
		expectErr      bool
		expectedCode   codes.Code
	}{
		{
			name:    "empty service returns SERVING",
			service: "",
			setup:   func(hs *HealthServer) {},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_SERVING,
			expectErr: false,
		},
		{
			name:    "known service SERVING",
			service: "watch-svc",
			setup: func(hs *HealthServer) {
				hs.SetServiceStatus(
					"watch-svc",
					grpc_health_v1.HealthCheckResponse_SERVING,
				)
			},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_SERVING,
			expectErr: false,
		},
		{
			name:    "known service NOT_SERVING",
			service: "watch-degraded",
			setup: func(hs *HealthServer) {
				hs.SetServiceStatus(
					"watch-degraded",
					grpc_health_v1.HealthCheckResponse_NOT_SERVING,
				)
			},
			expectedStatus: grpc_health_v1.
				HealthCheckResponse_NOT_SERVING,
			expectErr: false,
		},
		{
			name:         "unknown service returns NOT_FOUND",
			service:      "watch-unknown",
			setup:        func(hs *HealthServer) {},
			expectErr:    true,
			expectedCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := NewHealthServer()
			tt.setup(hs)

			mockStream := newMockHealthWatchServer(context.Background())

			err := hs.Watch(
				&grpc_health_v1.HealthCheckRequest{Service: tt.service},
				mockStream,
			)

			if tt.expectErr {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, st.Code())
				assert.Empty(t, mockStream.responses)
			} else {
				require.NoError(t, err)
				require.Len(t, mockStream.responses, 1)
				assert.Equal(t, tt.expectedStatus, mockStream.responses[0].Status)
			}
		})
	}
}

func TestHealthServer_Watch_SendError(t *testing.T) {
	hs := NewHealthServer()
	hs.SetServiceStatus("svc", grpc_health_v1.HealthCheckResponse_SERVING)

	mockStream := newMockHealthWatchServer(context.Background())
	mockStream.sendErr = errors.New("send failed")

	err := hs.Watch(
		&grpc_health_v1.HealthCheckRequest{Service: "svc"},
		mockStream,
	)

	require.Error(t, err)
	assert.Equal(t, "send failed", err.Error())
}

// =============================================================================
// Interceptor Tests
// =============================================================================

func TestChainStreamInterceptors_Empty(t *testing.T) {
	chained := ChainStreamInterceptors()
	assert.NotNil(t, chained)

	// Test that it calls the handler directly
	handlerCalled := false
	handler := func(srv interface{}, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	mockStream := newMockServerStream(context.Background())
	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)

	assert.NoError(t, err)
	assert.True(t, handlerCalled)
}

func TestChainStreamInterceptors_Single(t *testing.T) {
	var order []string

	interceptor := func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		order = append(order, "interceptor-before")
		err := handler(srv, ss)
		order = append(order, "interceptor-after")
		return err
	}

	chained := ChainStreamInterceptors(interceptor)

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		order = append(order, "handler")
		return nil
	}

	mockStream := newMockServerStream(context.Background())
	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)

	assert.NoError(t, err)
	assert.Equal(t, []string{"interceptor-before", "handler", "interceptor-after"}, order)
}

func TestChainStreamInterceptors_Multiple(t *testing.T) {
	var order []string

	makeInterceptor := func(name string) StreamInterceptor {
		return func(
			srv interface{},
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) error {
			order = append(order, name+"-before")
			err := handler(srv, ss)
			order = append(order, name+"-after")
			return err
		}
	}

	chained := ChainStreamInterceptors(
		makeInterceptor("first"),
		makeInterceptor("second"),
		makeInterceptor("third"),
	)

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		order = append(order, "handler")
		return nil
	}

	mockStream := newMockServerStream(context.Background())
	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)

	assert.NoError(t, err)
	// Interceptors execute in order: first -> second -> third -> handler -> third -> second -> first
	expected := []string{
		"first-before", "second-before", "third-before",
		"handler",
		"third-after", "second-after", "first-after",
	}
	assert.Equal(t, expected, order)
}

func TestChainStreamInterceptors_WithError(t *testing.T) {
	expectedErr := errors.New("interceptor error")

	interceptor := func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		return expectedErr
	}

	chained := ChainStreamInterceptors(interceptor)

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		return nil
	}

	mockStream := newMockServerStream(context.Background())
	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)

	assert.Equal(t, expectedErr, err)
}

func TestChainStreamInterceptors_HandlerError(t *testing.T) {
	var interceptorSawError error
	expectedErr := errors.New("handler error")

	interceptor := func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := handler(srv, ss)
		interceptorSawError = err
		return err
	}

	chained := ChainStreamInterceptors(interceptor)

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		return expectedErr
	}

	mockStream := newMockServerStream(context.Background())
	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)

	assert.Equal(t, expectedErr, err)
	assert.Equal(t, expectedErr, interceptorSawError)
}

func TestMetadataInterceptor(t *testing.T) {
	interceptor := MetadataInterceptor()
	assert.NotNil(t, interceptor)

	t.Run("with existing metadata", func(t *testing.T) {
		md := metadata.Pairs("key1", "value1", "key2", "value2")
		ctx := metadata.NewIncomingContext(context.Background(), md)
		mockStream := newMockServerStream(ctx)

		var capturedStream grpc.ServerStream
		handler := func(srv interface{}, ss grpc.ServerStream) error {
			capturedStream = ss
			return nil
		}

		err := interceptor(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)
		assert.NoError(t, err)
		assert.NotNil(t, capturedStream)

		// Verify metadata is accessible
		md2, ok := metadata.FromIncomingContext(capturedStream.Context())
		assert.True(t, ok)
		assert.Equal(t, []string{"value1"}, md2.Get("key1"))
		assert.Equal(t, []string{"value2"}, md2.Get("key2"))
	})

	t.Run("without metadata", func(t *testing.T) {
		mockStream := newMockServerStream(context.Background())

		var capturedStream grpc.ServerStream
		handler := func(srv interface{}, ss grpc.ServerStream) error {
			capturedStream = ss
			return nil
		}

		err := interceptor(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)
		assert.NoError(t, err)
		assert.NotNil(t, capturedStream)

		// Should have empty metadata
		md, ok := metadata.FromIncomingContext(capturedStream.Context())
		assert.True(t, ok)
		assert.Empty(t, md)
	})

	t.Run("handler error propagated", func(t *testing.T) {
		mockStream := newMockServerStream(context.Background())
		expectedErr := errors.New("handler failed")

		handler := func(srv interface{}, ss grpc.ServerStream) error {
			return expectedErr
		}

		err := interceptor(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)
		assert.Equal(t, expectedErr, err)
	})
}

func TestLoggingInterceptor(t *testing.T) {
	t.Run("nil log func", func(t *testing.T) {
		interceptor := LoggingInterceptor(nil)
		assert.NotNil(t, interceptor)

		handlerCalled := false
		handler := func(srv interface{}, ss grpc.ServerStream) error {
			handlerCalled = true
			return nil
		}

		mockStream := newMockServerStream(context.Background())
		err := interceptor(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test"}, handler)

		assert.NoError(t, err)
		assert.True(t, handlerCalled)
	})

	t.Run("with log func success", func(t *testing.T) {
		var loggedMethod string
		var loggedErr error
		logCalled := false

		logFunc := func(method string, err error) {
			logCalled = true
			loggedMethod = method
			loggedErr = err
		}

		interceptor := LoggingInterceptor(logFunc)

		handler := func(srv interface{}, ss grpc.ServerStream) error {
			return nil
		}

		mockStream := newMockServerStream(context.Background())
		err := interceptor(
			nil,
			mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/Method"},
			handler,
		)

		assert.NoError(t, err)
		assert.True(t, logCalled)
		assert.Equal(t, "/test.Service/Method", loggedMethod)
		assert.Nil(t, loggedErr)
	})

	t.Run("with log func error", func(t *testing.T) {
		var loggedMethod string
		var loggedErr error

		logFunc := func(method string, err error) {
			loggedMethod = method
			loggedErr = err
		}

		interceptor := LoggingInterceptor(logFunc)
		expectedErr := errors.New("handler error")

		handler := func(srv interface{}, ss grpc.ServerStream) error {
			return expectedErr
		}

		mockStream := newMockServerStream(context.Background())
		err := interceptor(
			nil,
			mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/FailingMethod"},
			handler,
		)

		assert.Equal(t, expectedErr, err)
		assert.Equal(t, "/test.Service/FailingMethod", loggedMethod)
		assert.Equal(t, expectedErr, loggedErr)
	})
}

// =============================================================================
// wrappedStream Tests
// =============================================================================

func TestWrappedStream_Context(t *testing.T) {
	originalCtx := context.Background()
	newCtx := context.WithValue(originalCtx, "key", "value")

	mockStream := newMockServerStream(originalCtx)
	wrapped := &wrappedStream{ServerStream: mockStream, ctx: newCtx}

	assert.Equal(t, newCtx, wrapped.Context())
	assert.NotEqual(t, originalCtx, wrapped.Context())
	assert.Equal(t, "value", wrapped.Context().Value("key"))
}

func TestWrappedStream_DelegatesMethods(t *testing.T) {
	mockStream := newMockServerStream(context.Background())
	wrapped := &wrappedStream{ServerStream: mockStream, ctx: context.Background()}

	// Test that it delegates to the underlying stream
	err := wrapped.SendMsg("test")
	assert.NoError(t, err)
	assert.Len(t, mockStream.sentMsgs, 1)

	err = wrapped.SetHeader(metadata.MD{})
	assert.NoError(t, err)

	err = wrapped.SendHeader(metadata.MD{})
	assert.NoError(t, err)
}

// =============================================================================
// RegisterHealthServer Tests
// =============================================================================

func TestRegisterHealthServer(t *testing.T) {
	// Create a gRPC server
	s := grpc.NewServer()
	hs := NewHealthServer()

	// Should not panic
	assert.NotPanics(t, func() {
		RegisterHealthServer(s, hs)
	})

	// Clean up
	s.Stop()
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestChainStreamInterceptors_WithLoggingAndMetadata(t *testing.T) {
	var loggedMethod string
	var loggedErr error

	logFunc := func(method string, err error) {
		loggedMethod = method
		loggedErr = err
	}

	chained := ChainStreamInterceptors(
		MetadataInterceptor(),
		LoggingInterceptor(logFunc),
	)

	md := metadata.Pairs("auth", "token123")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	mockStream := newMockServerStream(ctx)

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		// Verify metadata is accessible
		md2, ok := metadata.FromIncomingContext(ss.Context())
		assert.True(t, ok)
		assert.Equal(t, []string{"token123"}, md2.Get("auth"))
		return nil
	}

	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/api/stream"}, handler)

	assert.NoError(t, err)
	assert.Equal(t, "/api/stream", loggedMethod)
	assert.Nil(t, loggedErr)
}

func TestHealthServer_MultipleServices(t *testing.T) {
	hs := NewHealthServer()

	services := map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
		"service-a": grpc_health_v1.HealthCheckResponse_SERVING,
		"service-b": grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		"service-c": grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN,
	}

	// Set all services
	for name, status := range services {
		hs.SetServiceStatus(name, status)
	}

	// Verify all services
	for name, expectedStatus := range services {
		resp, err := hs.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{
			Service: name,
		})
		require.NoError(t, err)
		assert.Equal(t, expectedStatus, resp.Status)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkEvent_Format(b *testing.B) {
	hs := NewHealthServer()
	hs.SetServiceStatus("bench-svc", grpc_health_v1.HealthCheckResponse_SERVING)
	req := &grpc_health_v1.HealthCheckRequest{Service: "bench-svc"}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = hs.Check(ctx, req)
	}
}

func BenchmarkChainStreamInterceptors(b *testing.B) {
	interceptor := func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		return handler(srv, ss)
	}

	chained := ChainStreamInterceptors(interceptor, interceptor, interceptor)

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		return nil
	}

	mockStream := newMockServerStream(context.Background())
	info := &grpc.StreamServerInfo{FullMethod: "/test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chained(nil, mockStream, info, handler)
	}
}
