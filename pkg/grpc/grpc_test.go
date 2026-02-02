package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, ":50051", config.Address)
	assert.Equal(t, 4*1024*1024, config.MaxRecvMsgSize)
	assert.Equal(t, 4*1024*1024, config.MaxSendMsgSize)
	assert.Equal(t, uint32(100), config.MaxConcurrentStreams)
}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServerOptions(tt.config)
			assert.NotEmpty(t, opts)
			assert.Len(t, opts, 3)
		})
	}
}

func TestNewHealthServer(t *testing.T) {
	hs := NewHealthServer()
	require.NotNil(t, hs)
	assert.NotNil(t, hs.services)
	assert.Empty(t, hs.services)
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
			name:         "unknown service returns NOT_FOUND",
			service:      "unknown",
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

func TestHealthServer_SetServiceStatus(t *testing.T) {
	hs := NewHealthServer()

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
			name:    "update existing",
			service: "svc1",
			status:  grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
	}

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

func TestChainStreamInterceptors(t *testing.T) {
	var order []int

	i1 := func(
		srv interface{},
		ss interface{ Context() context.Context },
		info interface{},
		handler func(interface{}, interface{ Context() context.Context }) error,
	) error {
		order = append(order, 1)
		// We test the concept without a real grpc.ServerStream
		return nil
	}
	_ = i1

	// Basic validation that ChainStreamInterceptors returns a non-nil function
	chained := ChainStreamInterceptors()
	assert.NotNil(t, chained)
}

func TestLoggingInterceptor(t *testing.T) {
	tests := []struct {
		name    string
		logFunc func(method string, err error)
	}{
		{
			name:    "nil log func",
			logFunc: nil,
		},
		{
			name: "with log func",
			logFunc: func(method string, err error) {
				// no-op for test
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := LoggingInterceptor(tt.logFunc)
			assert.NotNil(t, interceptor)
		})
	}
}

func TestMetadataInterceptor(t *testing.T) {
	interceptor := MetadataInterceptor()
	assert.NotNil(t, interceptor)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.config.Address)
		})
	}
}
