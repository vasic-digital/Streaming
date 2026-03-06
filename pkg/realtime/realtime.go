// Package realtime provides generic file change notification with
// debouncing and event aggregation. It sits above file system watchers,
// collecting raw change events and flushing them as consolidated
// batches after a configurable debounce window expires.
package realtime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Logger defines a structured logging interface.
type Logger interface {
	// Info logs an informational message.
	Info(msg string, keysAndValues ...interface{})
	// Warn logs a warning message.
	Warn(msg string, keysAndValues ...interface{})
	// Error logs an error message.
	Error(msg string, keysAndValues ...interface{})
	// Debug logs a debug message.
	Debug(msg string, keysAndValues ...interface{})
}

// ChangeType represents the kind of file system change.
type ChangeType int

const (
	// Created indicates a new file or directory was created.
	Created ChangeType = iota
	// Modified indicates an existing file or directory was modified.
	Modified
	// Deleted indicates a file or directory was removed.
	Deleted
	// Renamed indicates a file or directory was renamed.
	Renamed
)

// String returns the human-readable name of a ChangeType.
func (ct ChangeType) String() string {
	switch ct {
	case Created:
		return "created"
	case Modified:
		return "modified"
	case Deleted:
		return "deleted"
	case Renamed:
		return "renamed"
	default:
		return fmt.Sprintf("unknown(%d)", int(ct))
	}
}

// ChangeEvent represents a single file system change event.
type ChangeEvent struct {
	// Path is the absolute path of the affected file or directory.
	Path string
	// Type is the kind of change that occurred.
	Type ChangeType
	// Timestamp is when the change was detected.
	Timestamp time.Time
	// IsDir indicates whether the path is a directory.
	IsDir bool
	// OldPath contains the previous path for rename events.
	OldPath string
}

// EventHandler is a callback that receives a batch of aggregated
// change events after the debounce window expires.
type EventHandler func(events []ChangeEvent)

// Debouncer aggregates file change events over a configurable time
// window and flushes them as a single batch to the registered handler.
// Multiple rapid events are coalesced so downstream consumers receive
// fewer, larger batches instead of a flood of individual notifications.
type Debouncer struct {
	window  time.Duration
	handler EventHandler
	logger  Logger

	mu      sync.Mutex
	pending []ChangeEvent
	timer   *time.Timer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
	stopped bool

	// flushCh is used internally to signal the run loop that a flush
	// is due. This avoids races between the timer goroutine and the
	// run loop.
	flushCh chan struct{}
}

// NewDebouncer creates a new Debouncer.
//
// Parameters:
//   - window: the debounce duration; events arriving within this
//     period are batched together. Must be > 0.
//   - handler: callback invoked with the aggregated batch. Must not
//     be nil.
//   - logger: structured logger for diagnostics. May be nil, in which
//     case a no-op logger is used.
func NewDebouncer(
	window time.Duration,
	handler EventHandler,
	logger Logger,
) *Debouncer {
	if logger == nil {
		logger = &noopLogger{}
	}

	return &Debouncer{
		window:  window,
		handler: handler,
		logger:  logger,
		flushCh: make(chan struct{}, 1),
	}
}

// Start begins the debounce loop. The debouncer runs until Stop is
// called or the provided context is cancelled. Start may only be
// called once; subsequent calls return an error.
func (d *Debouncer) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return fmt.Errorf("debouncer already started")
	}
	d.started = true
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.mu.Unlock()

	d.logger.Info("debouncer started",
		"window", d.window.String())

	d.wg.Add(1)
	go d.run()

	return nil
}

// Stop gracefully shuts down the debouncer. Any pending events are
// flushed before Stop returns. Stop is idempotent.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	if d.stopped || !d.started {
		d.mu.Unlock()
		return
	}
	d.stopped = true
	d.cancel()
	d.mu.Unlock()

	d.wg.Wait()
	d.logger.Info("debouncer stopped")
}

// Notify submits a change event to the debouncer. The event is added
// to the pending batch and the debounce timer is (re)started. If the
// debouncer is not running, Notify returns an error.
func (d *Debouncer) Notify(event ChangeEvent) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.started || d.stopped {
		return fmt.Errorf("debouncer is not running")
	}

	d.pending = append(d.pending, event)
	d.logger.Debug("event queued",
		"path", event.Path,
		"type", event.Type.String(),
		"pending", len(d.pending))

	d.resetTimerLocked()
	return nil
}

// Pending returns the number of events currently waiting to be
// flushed. This is primarily useful for testing and monitoring.
func (d *Debouncer) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// run is the main event loop. It waits for flush signals or context
// cancellation.
func (d *Debouncer) run() {
	defer d.wg.Done()

	for {
		select {
		case <-d.flushCh:
			d.flush()
		case <-d.ctx.Done():
			// Drain any remaining events before exiting.
			d.flush()
			return
		}
	}
}

// resetTimerLocked resets or starts the debounce timer. The caller
// must hold d.mu.
func (d *Debouncer) resetTimerLocked() {
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.window, func() {
		select {
		case d.flushCh <- struct{}{}:
		default:
			// A flush is already pending; no need to signal again.
		}
	})
}

// flush drains the pending events and delivers them to the handler.
func (d *Debouncer) flush() {
	d.mu.Lock()
	if len(d.pending) == 0 {
		d.mu.Unlock()
		return
	}

	batch := d.pending
	d.pending = nil
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.mu.Unlock()

	d.logger.Info("flushing events", "count", len(batch))
	d.handler(batch)
}

// noopLogger is a Logger that discards all messages.
type noopLogger struct{}

func (n *noopLogger) Info(_ string, _ ...interface{})  {}
func (n *noopLogger) Warn(_ string, _ ...interface{})  {}
func (n *noopLogger) Error(_ string, _ ...interface{}) {}
func (n *noopLogger) Debug(_ string, _ ...interface{}) {}
