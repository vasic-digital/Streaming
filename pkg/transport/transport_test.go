package transport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	require.NotNil(t, f)

	types := f.SupportedTypes()
	assert.Len(t, types, 3)
}

func TestFactory_Create(t *testing.T) {
	f := NewFactory()

	tests := []struct {
		name      string
		config    *Config
		expectErr bool
		errMsg    string
	}{
		{
			name: "HTTP transport",
			config: &Config{
				Type:    TypeHTTP,
				Address: "http://localhost:8080",
			},
			expectErr: false,
		},
		{
			name: "WebSocket transport",
			config: &Config{
				Type:    TypeWebSocket,
				Address: "ws://localhost:8080",
			},
			expectErr: false,
		},
		{
			name: "gRPC transport",
			config: &Config{
				Type:    TypeGRPC,
				Address: "localhost:50051",
			},
			expectErr: false,
		},
		{
			name:      "nil config",
			config:    nil,
			expectErr: true,
			errMsg:    "config is required",
		},
		{
			name: "unsupported type",
			config: &Config{
				Type:    Type("mqtt"),
				Address: "tcp://localhost:1883",
			},
			expectErr: true,
			errMsg:    "unsupported transport type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := f.Create(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, transport)
			} else {
				require.NoError(t, err)
				require.NotNil(t, transport)
				_ = transport.Close()
			}
		})
	}
}

func TestFactory_Register(t *testing.T) {
	f := NewFactory()

	customType := Type("custom")
	f.Register(customType, func(config *Config) (Transport, error) {
		return newChannelTransport(config)
	})

	transport, err := f.Create(&Config{
		Type:    customType,
		Address: "custom://localhost",
	})
	require.NoError(t, err)
	require.NotNil(t, transport)
	_ = transport.Close()
}

func TestChannelTransport_SendReceive(t *testing.T) {
	f := NewFactory()
	tr, err := f.Create(&Config{
		Type:    TypeHTTP,
		Address: "http://localhost",
		Options: map[string]interface{}{
			"buffer_size": 10,
		},
	})
	require.NoError(t, err)
	defer func() { _ = tr.Close() }()

	ct := tr.(*channelTransport)

	// Send data
	ctx := context.Background()
	err = tr.Send(ctx, []byte("hello"))
	require.NoError(t, err)

	// Read from send channel
	select {
	case data := <-ct.SendChannel():
		assert.Equal(t, "hello", string(data))
	case <-time.After(time.Second):
		t.Fatal("timeout reading from send channel")
	}

	// Inject data into receive channel
	ct.ReceiveChannel() <- []byte("world")

	// Receive data
	data, err := tr.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "world", string(data))
}

func TestChannelTransport_SendClosed(t *testing.T) {
	f := NewFactory()
	tr, err := f.Create(&Config{Type: TypeHTTP, Address: "test"})
	require.NoError(t, err)

	err = tr.Close()
	require.NoError(t, err)

	err = tr.Send(context.Background(), []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transport is closed")
}

func TestChannelTransport_ReceiveClosed(t *testing.T) {
	f := NewFactory()
	tr, err := f.Create(&Config{Type: TypeHTTP, Address: "test"})
	require.NoError(t, err)

	err = tr.Close()
	require.NoError(t, err)

	_, err = tr.Receive(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transport is closed")
}

func TestChannelTransport_SendContextCancelled(t *testing.T) {
	f := NewFactory()
	tr, err := f.Create(&Config{
		Type:    TypeHTTP,
		Address: "test",
		Options: map[string]interface{}{
			"buffer_size": 0,
		},
	})
	require.NoError(t, err)
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = tr.Send(ctx, []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "send cancelled")
}

func TestChannelTransport_ReceiveContextCancelled(t *testing.T) {
	f := NewFactory()
	tr, err := f.Create(&Config{Type: TypeHTTP, Address: "test"})
	require.NoError(t, err)
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tr.Receive(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "receive cancelled")
}

func TestChannelTransport_CloseIdempotent(t *testing.T) {
	f := NewFactory()
	tr, err := f.Create(&Config{Type: TypeHTTP, Address: "test"})
	require.NoError(t, err)

	err = tr.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = tr.Close()
	require.NoError(t, err)
}

func TestType_Constants(t *testing.T) {
	tests := []struct {
		name     string
		typ      Type
		expected string
	}{
		{"HTTP", TypeHTTP, "http"},
		{"WebSocket", TypeWebSocket, "websocket"},
		{"gRPC", TypeGRPC, "grpc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, Type(tt.expected), tt.typ)
		})
	}
}

func TestConfig_Options(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		hasOpts bool
	}{
		{
			name: "with options",
			config: Config{
				Type:    TypeHTTP,
				Address: "http://localhost",
				Options: map[string]interface{}{
					"buffer_size": 100,
					"timeout":     "30s",
				},
			},
			hasOpts: true,
		},
		{
			name: "without options",
			config: Config{
				Type:    TypeGRPC,
				Address: "localhost:50051",
			},
			hasOpts: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.hasOpts {
				assert.NotNil(t, tt.config.Options)
			} else {
				assert.Nil(t, tt.config.Options)
			}
		})
	}
}
