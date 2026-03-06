package realtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---------------------------------------------------------------

// collectHandler returns an EventHandler that appends received batches
// to a shared slice, and a function to retrieve all collected batches.
func collectHandler() (EventHandler, func() [][]ChangeEvent) {
	var (
		mu      sync.Mutex
		batches [][]ChangeEvent
	)

	handler := func(events []ChangeEvent) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]ChangeEvent, len(events))
		copy(cp, events)
		batches = append(batches, cp)
	}

	getter := func() [][]ChangeEvent {
		mu.Lock()
		defer mu.Unlock()
		out := make([][]ChangeEvent, len(batches))
		copy(out, batches)
		return out
	}

	return handler, getter
}

// waitForBatches polls until at least n batches have been collected or
// the timeout elapses.
func waitForBatches(
	t *testing.T,
	getter func() [][]ChangeEvent,
	n int,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(getter()) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected at least %d batch(es), got %d", n, len(getter()))
}

// --- ChangeType tests ------------------------------------------------------

func TestChangeType_String(t *testing.T) {
	tests := []struct {
		name     string
		ct       ChangeType
		expected string
	}{
		{"Created", Created, "created"},
		{"Modified", Modified, "modified"},
		{"Deleted", Deleted, "deleted"},
		{"Renamed", Renamed, "renamed"},
		{"Unknown", ChangeType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.ct.String())
		})
	}
}

// --- Debouncer creation tests ----------------------------------------------

func TestNewDebouncer(t *testing.T) {
	handler := func(_ []ChangeEvent) {}
	d := NewDebouncer(100*time.Millisecond, handler, nil)
	require.NotNil(t, d)
	assert.Equal(t, 100*time.Millisecond, d.window)
	assert.NotNil(t, d.logger) // nil logger replaced with noop
}

func TestNewDebouncer_WithLogger(t *testing.T) {
	logger := &testLogger{}
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, logger)
	require.NotNil(t, d)
	assert.Equal(t, logger, d.logger)
}

// --- Start / Stop tests ----------------------------------------------------

func TestDebouncer_Start_Stop(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)
	ctx := context.Background()

	err := d.Start(ctx)
	require.NoError(t, err)

	d.Stop()
	// Idempotent stop should not panic.
	d.Stop()
}

func TestDebouncer_Start_AlreadyStarted(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)
	ctx := context.Background()

	err := d.Start(ctx)
	require.NoError(t, err)
	defer d.Stop()

	err = d.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestDebouncer_Notify_NotStarted(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)

	err := d.Notify(ChangeEvent{Path: "/tmp/a.txt", Type: Created})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestDebouncer_Notify_AfterStop(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)
	err := d.Start(context.Background())
	require.NoError(t, err)

	d.Stop()

	err = d.Notify(ChangeEvent{Path: "/tmp/a.txt", Type: Created})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// --- Aggregation tests -----------------------------------------------------

func TestDebouncer_Aggregation_MultipleBecomeOneBatch(t *testing.T) {
	handler, getter := collectHandler()
	d := NewDebouncer(100*time.Millisecond, handler, nil)

	err := d.Start(context.Background())
	require.NoError(t, err)
	defer d.Stop()

	now := time.Now()
	events := []ChangeEvent{
		{Path: "/a.txt", Type: Created, Timestamp: now},
		{Path: "/b.txt", Type: Modified, Timestamp: now},
		{Path: "/c.txt", Type: Deleted, Timestamp: now},
	}

	for _, e := range events {
		err := d.Notify(e)
		require.NoError(t, err)
	}

	// All three should arrive as a single batch.
	waitForBatches(t, getter, 1, 500*time.Millisecond)
	batches := getter()
	assert.Len(t, batches, 1)
	assert.Len(t, batches[0], 3)

	// Verify event contents.
	assert.Equal(t, "/a.txt", batches[0][0].Path)
	assert.Equal(t, Created, batches[0][0].Type)
	assert.Equal(t, "/b.txt", batches[0][1].Path)
	assert.Equal(t, Modified, batches[0][1].Type)
	assert.Equal(t, "/c.txt", batches[0][2].Path)
	assert.Equal(t, Deleted, batches[0][2].Type)
}

func TestDebouncer_Aggregation_RenameEvent(t *testing.T) {
	handler, getter := collectHandler()
	d := NewDebouncer(80*time.Millisecond, handler, nil)

	err := d.Start(context.Background())
	require.NoError(t, err)
	defer d.Stop()

	err = d.Notify(ChangeEvent{
		Path:      "/new_name.txt",
		Type:      Renamed,
		Timestamp: time.Now(),
		OldPath:   "/old_name.txt",
	})
	require.NoError(t, err)

	waitForBatches(t, getter, 1, 500*time.Millisecond)
	batches := getter()
	require.Len(t, batches, 1)
	require.Len(t, batches[0], 1)
	assert.Equal(t, Renamed, batches[0][0].Type)
	assert.Equal(t, "/old_name.txt", batches[0][0].OldPath)
	assert.Equal(t, "/new_name.txt", batches[0][0].Path)
}

func TestDebouncer_Aggregation_DirectoryEvent(t *testing.T) {
	handler, getter := collectHandler()
	d := NewDebouncer(80*time.Millisecond, handler, nil)

	err := d.Start(context.Background())
	require.NoError(t, err)
	defer d.Stop()

	err = d.Notify(ChangeEvent{
		Path:      "/mydir",
		Type:      Created,
		Timestamp: time.Now(),
		IsDir:     true,
	})
	require.NoError(t, err)

	waitForBatches(t, getter, 1, 500*time.Millisecond)
	batches := getter()
	require.Len(t, batches, 1)
	assert.True(t, batches[0][0].IsDir)
}

// --- Debounce window timing tests ------------------------------------------

func TestDebouncer_WindowTiming_ResetOnNewEvent(t *testing.T) {
	handler, getter := collectHandler()
	// Use a 150ms window.
	d := NewDebouncer(150*time.Millisecond, handler, nil)

	err := d.Start(context.Background())
	require.NoError(t, err)
	defer d.Stop()

	// Send first event.
	err = d.Notify(ChangeEvent{
		Path: "/first.txt", Type: Created, Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Wait 80ms (within window), send second event — this resets
	// the timer.
	time.Sleep(80 * time.Millisecond)
	err = d.Notify(ChangeEvent{
		Path: "/second.txt", Type: Modified, Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// At this point ~80ms have elapsed. The timer was reset, so we
	// need another ~150ms before flush. Verify no batch yet at 100ms
	// after second event.
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, getter(), 0, "should not have flushed yet")

	// Wait for the window to expire.
	waitForBatches(t, getter, 1, 300*time.Millisecond)
	batches := getter()
	require.Len(t, batches, 1)
	assert.Len(t, batches[0], 2, "both events in one batch")
}

func TestDebouncer_WindowTiming_SeparateBatches(t *testing.T) {
	handler, getter := collectHandler()
	d := NewDebouncer(60*time.Millisecond, handler, nil)

	err := d.Start(context.Background())
	require.NoError(t, err)
	defer d.Stop()

	// Send first event.
	err = d.Notify(ChangeEvent{
		Path: "/a.txt", Type: Created, Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Wait long enough for the first batch to flush.
	waitForBatches(t, getter, 1, 300*time.Millisecond)

	// Send second event after the first batch flushed.
	err = d.Notify(ChangeEvent{
		Path: "/b.txt", Type: Modified, Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Wait for the second batch.
	waitForBatches(t, getter, 2, 300*time.Millisecond)
	batches := getter()
	require.Len(t, batches, 2)
	assert.Len(t, batches[0], 1)
	assert.Len(t, batches[1], 1)
	assert.Equal(t, "/a.txt", batches[0][0].Path)
	assert.Equal(t, "/b.txt", batches[1][0].Path)
}

// --- Context cancellation tests --------------------------------------------

func TestDebouncer_ContextCancellation_FlushesRemaining(t *testing.T) {
	handler, getter := collectHandler()
	// Long window so the timer would not fire on its own.
	d := NewDebouncer(5*time.Second, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())

	err := d.Start(ctx)
	require.NoError(t, err)

	// Queue events.
	err = d.Notify(ChangeEvent{
		Path: "/pending.txt", Type: Created, Timestamp: time.Now(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, d.Pending())

	// Cancel the context — should cause a final flush.
	cancel()

	// Wait for the debouncer to finish.
	d.Stop()

	batches := getter()
	require.Len(t, batches, 1, "remaining events should flush on cancel")
	assert.Equal(t, "/pending.txt", batches[0][0].Path)
}

func TestDebouncer_ContextCancellation_StopsDebouncer(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)
	ctx, cancel := context.WithCancel(context.Background())

	err := d.Start(ctx)
	require.NoError(t, err)

	cancel()
	// Stop should return promptly because context was cancelled.
	done := make(chan struct{})
	go func() {
		d.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after context cancellation")
	}
}

// --- Pending tests ---------------------------------------------------------

func TestDebouncer_Pending(t *testing.T) {
	d := NewDebouncer(5*time.Second, func(_ []ChangeEvent) {}, nil)

	err := d.Start(context.Background())
	require.NoError(t, err)
	defer d.Stop()

	assert.Equal(t, 0, d.Pending())

	_ = d.Notify(ChangeEvent{Path: "/a", Type: Created})
	_ = d.Notify(ChangeEvent{Path: "/b", Type: Modified})
	assert.Equal(t, 2, d.Pending())
}

// --- Stress test -----------------------------------------------------------

func TestDebouncer_ManyEvents(t *testing.T) {
	var (
		mu    sync.Mutex
		total int
	)

	handler := func(events []ChangeEvent) {
		mu.Lock()
		defer mu.Unlock()
		total += len(events)
	}

	d := NewDebouncer(80*time.Millisecond, handler, nil)
	err := d.Start(context.Background())
	require.NoError(t, err)

	const eventCount = 500
	for i := 0; i < eventCount; i++ {
		_ = d.Notify(ChangeEvent{
			Path:      "/file.txt",
			Type:      Modified,
			Timestamp: time.Now(),
		})
	}

	// Allow time for flush.
	time.Sleep(300 * time.Millisecond)
	d.Stop()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, eventCount, total,
		"all events should be delivered across batches")
}

// --- Stop without Start ----------------------------------------------------

func TestDebouncer_Stop_WithoutStart(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)
	// Should not panic.
	d.Stop()
}

// --- Nil logger replaced ---------------------------------------------------

func TestDebouncer_NilLogger(t *testing.T) {
	d := NewDebouncer(50*time.Millisecond, func(_ []ChangeEvent) {}, nil)
	require.NotNil(t, d.logger)

	// Verify noop logger methods do not panic.
	d.logger.Info("test")
	d.logger.Warn("test")
	d.logger.Error("test")
	d.logger.Debug("test")
}

// --- testLogger for verifying logger wiring --------------------------------

type testLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *testLogger) Info(msg string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "INFO: "+msg)
}

func (l *testLogger) Warn(msg string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "WARN: "+msg)
}

func (l *testLogger) Error(msg string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "ERROR: "+msg)
}

func (l *testLogger) Debug(msg string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "DEBUG: "+msg)
}

func TestDebouncer_LoggerCalled(t *testing.T) {
	logger := &testLogger{}
	handler, getter := collectHandler()
	d := NewDebouncer(50*time.Millisecond, handler, logger)

	err := d.Start(context.Background())
	require.NoError(t, err)

	_ = d.Notify(ChangeEvent{
		Path: "/logged.txt", Type: Created, Timestamp: time.Now(),
	})

	waitForBatches(t, getter, 1, 300*time.Millisecond)
	d.Stop()

	logger.mu.Lock()
	defer logger.mu.Unlock()
	assert.True(t, len(logger.messages) > 0, "logger should have received messages")
}
