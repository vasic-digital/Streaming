package transport_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"digital.vasic.streaming/pkg/transport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTransport_ZeroLengthSend(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	require.NoError(t, err)
	defer tr.Close()

	ctx := context.Background()

	// Send zero-length data should succeed
	err = tr.Send(ctx, []byte{})
	require.NoError(t, err)

	// Send nil data should succeed
	err = tr.Send(ctx, nil)
	require.NoError(t, err)
}

func TestChannelTransport_SendAfterClose(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	require.NoError(t, err)

	err = tr.Close()
	require.NoError(t, err)

	err = tr.Send(context.Background(), []byte("after close"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transport is closed")
}

func TestChannelTransport_ReceiveAfterClose(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	require.NoError(t, err)

	err = tr.Close()
	require.NoError(t, err)

	_, err = tr.Receive(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transport is closed")
}

func TestChannelTransport_SendContextTimeout(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	// Buffer size 0 means unbuffered -- Send will block
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
		Options: map[string]interface{}{
			"buffer_size": 0,
		},
	})
	require.NoError(t, err)
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = tr.Send(ctx, []byte("will block"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "send cancelled")
}

func TestChannelTransport_ReceiveContextTimeout(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	require.NoError(t, err)
	defer tr.Close()

	// No data injected -- Receive should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = tr.Receive(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "receive cancelled")
}

func TestChannelTransport_CloseMultipleTimes(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	require.NoError(t, err)

	err = tr.Close()
	assert.NoError(t, err)

	// Second close should be no-op, not panic
	err = tr.Close()
	assert.NoError(t, err)
}

func TestFactory_CreateUnsupportedType(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	_, err := f.Create(&transport.Config{
		Type:    transport.Type("unknown"),
		Address: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported transport type")
}

func TestFactory_NilConfig(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	_, err := f.Create(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestFactory_ConcurrentRegisterAndCreate(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	var wg sync.WaitGroup

	// Concurrent registration of custom types
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			customType := transport.Type(fmt.Sprintf("custom-%d", idx))
			f.Register(customType, func(config *transport.Config) (transport.Transport, error) {
				return nil, fmt.Errorf("not implemented")
			})
		}(i)
	}

	// Concurrent creation of built-in types
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr, err := f.Create(&transport.Config{
				Type:    transport.TypeHTTP,
				Address: "test",
			})
			if err == nil {
				_ = tr.Close()
			}
		}()
	}

	wg.Wait()
}

func TestFactory_SupportedTypes_AllBuiltIn(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	types := f.SupportedTypes()

	assert.Len(t, types, 3)

	typeSet := make(map[transport.Type]bool)
	for _, tp := range types {
		typeSet[tp] = true
	}
	assert.True(t, typeSet[transport.TypeHTTP])
	assert.True(t, typeSet[transport.TypeWebSocket])
	assert.True(t, typeSet[transport.TypeGRPC])
}

func TestFactory_OverrideBuiltInType(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()

	called := false
	f.Register(transport.TypeHTTP, func(config *transport.Config) (transport.Transport, error) {
		called = true
		return nil, fmt.Errorf("custom HTTP")
	})

	_, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "custom HTTP")
	assert.True(t, called)
}

func TestChannelTransport_ConcurrentSendAndClose(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
		Options: map[string]interface{}{
			"buffer_size": 100,
		},
	})
	require.NoError(t, err)

	var wg sync.WaitGroup

	// Start sending concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = tr.Send(context.Background(), []byte(fmt.Sprintf("msg-%d", idx)))
		}(i)
	}

	// Close while sends are in flight
	time.Sleep(5 * time.Millisecond)
	_ = tr.Close()

	wg.Wait()
	// No panic or deadlock means success
}

func TestChannelTransport_LargeBufferSize(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
		Options: map[string]interface{}{
			"buffer_size": 10000,
		},
	})
	require.NoError(t, err)
	defer tr.Close()

	// Fill the buffer
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		err := tr.Send(ctx, []byte("data"))
		require.NoError(t, err)
	}
}

func TestChannelTransport_DefaultBufferSize(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	// No buffer_size option -- should use default (256)
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
	})
	require.NoError(t, err)
	defer tr.Close()

	// Should be able to send at least 200 messages without blocking
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		err := tr.Send(ctx, []byte("data"))
		require.NoError(t, err)
	}
}

func TestChannelTransport_InvalidBufferSizeOption(t *testing.T) {
	t.Parallel()

	f := transport.NewFactory()
	// Non-int buffer_size should fall back to default
	tr, err := f.Create(&transport.Config{
		Type:    transport.TypeHTTP,
		Address: "test",
		Options: map[string]interface{}{
			"buffer_size": "not-a-number",
		},
	})
	require.NoError(t, err)
	defer tr.Close()

	// Should still work with default buffer
	err = tr.Send(context.Background(), []byte("test"))
	assert.NoError(t, err)
}
