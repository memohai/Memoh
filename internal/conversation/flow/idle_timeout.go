package flow

import (
	"context"
	"sync"
	"time"
)

// idleCancel wraps a resettable idle timer. If Reset() is not called before
// the timer fires, the underlying context is cancelled.
type idleCancel struct {
	cancel      context.CancelFunc
	timer       *time.Timer
	mu          sync.Mutex
	state       idleTimerState
	baseTimeout time.Duration
	toolTimeout time.Duration
	toolCalls   int
}

type idleTimerState uint8

const (
	idleTimerActive idleTimerState = iota
	idleTimerFinished
	idleTimerFired
)

func (ic *idleCancel) Reset() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if ic.state == idleTimerActive {
		ic.timer.Stop()
		ic.timer.Reset(ic.currentTimeout())
	}
}

// RecordToolCall increments the tool call counter and extends the idle timeout.
func (ic *idleCancel) RecordToolCall() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.toolCalls++
}

func (ic *idleCancel) Stop() {
	ic.Finish()
	ic.cancel()
}

func (ic *idleCancel) Finish() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if ic.state == idleTimerActive {
		ic.state = idleTimerFinished
		ic.timer.Stop()
	}
}

func (ic *idleCancel) fire() {
	ic.mu.Lock()
	if ic.state != idleTimerActive {
		ic.mu.Unlock()
		return
	}
	ic.state = idleTimerFired
	ic.mu.Unlock()
	ic.cancel()
}

func (ic *idleCancel) DidFire() bool {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.state == idleTimerFired
}

// ToolCalls returns the number of tool calls recorded.
func (ic *idleCancel) ToolCalls() int {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.toolCalls
}

// currentTimeout returns the adaptive timeout: base + 60s per tool call, capped at 600s.
// Tool calls (especially spawn/subagent) can take minutes to complete, so the
// extension per tool call is generous to avoid interrupting active work.
func (ic *idleCancel) currentTimeout() time.Duration {
	extra := time.Duration(ic.toolCalls) * ic.toolTimeout
	timeout := ic.baseTimeout + extra
	if timeout > 600*time.Second {
		timeout = 600 * time.Second
	}
	return timeout
}

const (
	defaultIdleTimeout     = 90 * time.Second
	defaultIdleToolTimeout = 60 * time.Second
)

type idleTimeoutOptions struct {
	baseTimeout time.Duration
	toolTimeout time.Duration
}

// withIdleTimeout returns a context that is cancelled if no Reset() call is
// made within the adaptive idle timeout. The returned idleCancel must have
// Reset() called for each meaningful event to prevent the timeout from firing.
// The timeout adapts: base + 60s per tool call, capped at 600s.
func withIdleTimeout(parent context.Context, options *idleTimeoutOptions) (context.Context, *idleCancel) {
	bt := defaultIdleTimeout
	toolTimeout := defaultIdleToolTimeout
	if options != nil {
		if options.baseTimeout > 0 {
			bt = options.baseTimeout
		}
		toolTimeout = options.toolTimeout
	}

	ctx, cancel := context.WithCancel(parent)
	ic := &idleCancel{
		cancel:      cancel,
		baseTimeout: bt,
		toolTimeout: toolTimeout,
	}
	ic.timer = time.AfterFunc(bt, ic.fire)
	return ctx, ic
}
