//nolint:sloglint // aux retry ops logs use inline key/value pairs for readability
package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// auxUpserter is the minimal contract the aux-dense retry queue depends on.
// The graphRuntime's auxiliary dense wrapper implements this; embedding of the
// node body into a vector happens inside the concrete upserter, not in the
// queue itself.
type auxUpserter interface {
	UpsertDense(ctx context.Context, botID, nodeID, body string, payload map[string]string) error
}

// retryItem is a single failed auxiliary index upsert awaiting re-attempt.
type retryItem struct {
	botID       string
	nodeID      string
	body        string
	hash        string
	payload     map[string]string // qdrant payload, passed through to the upserter
	attempts    int               // number of failed attempts so far
	nextAttempt time.Time         // when the item becomes due again
}

// retryQueue is the per-bot queue. It is guarded by the parent auxIndexRetry.mu
// (keyed by botID). Only one processor goroutine runs for a given queue at a
// time (inFlight serializes that).
type retryQueue struct {
	items    []retryItem
	inFlight bool
	cancel   context.CancelFunc // cancels the processor goroutine for this bot
}

// auxIndexRetry is a per-botID serial retry queue for failed auxiliary dense
// index upserts. Postgres remains the source of truth; these upserts are
// best-effort and must never fail the memory write. Failed upserts are retried
// with exponential backoff up to maxAttempts.
//
// A single auxUpserter is bound at construction (the graphRuntime's auxDense
// wrapper). This avoids package-level state and keeps the queue self-contained.
type auxIndexRetry struct {
	mu          sync.Mutex
	queues      map[string]*retryQueue // keyed by botID
	upserter    auxUpserter            // bound at construction; nil when no aux dense configured
	logger      *slog.Logger
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// newAuxIndexRetry constructs an auxIndexRetry. upserter may be nil when no
// auxiliary dense index is configured (Enqueue then logs and drops). A nil
// logger falls back to slog.Default().
func newAuxIndexRetry(logger *slog.Logger, upserter auxUpserter) *auxIndexRetry {
	if logger == nil {
		logger = slog.Default()
	}
	return &auxIndexRetry{
		queues:      make(map[string]*retryQueue),
		upserter:    upserter,
		logger:      logger,
		maxAttempts: 5,
		baseDelay:   2 * time.Second,
		maxDelay:    2 * time.Minute,
	}
}

// Enqueue adds (or refreshes, deduped by nodeID) a failed auxiliary upsert for
// the given bot and, if no processor is currently running for that bot, spawns
// one. The payload map is passed verbatim to the upserter.
func (r *auxIndexRetry) Enqueue(botID, nodeID, body, hash string, payload map[string]string) {
	if r.upserter == nil {
		// No auxiliary index configured; nothing to retry.
		return
	}

	r.mu.Lock()
	q := r.queues[botID]
	if q == nil {
		q = &retryQueue{}
		r.queues[botID] = q
	}

	// Dedup by nodeID: replace any existing pending entry so we retry the
	// freshest body/payload, but preserve the attempt count so we still make
	// forward progress toward maxAttempts.
	attempts := 0
	for i, it := range q.items {
		if it.nodeID == nodeID {
			attempts = it.attempts
			q.items = append(q.items[:i], q.items[i+1:]...)
			break
		}
	}

	q.items = append(q.items, retryItem{
		botID:       botID,
		nodeID:      nodeID,
		body:        body,
		hash:        hash,
		payload:     payload,
		attempts:    attempts,
		nextAttempt: time.Now().Add(r.baseDelay),
	})

	// Spawn a processor only if none is running for this bot.
	if !q.inFlight {
		q.inFlight = true
		ctx, cancel := context.WithCancel(context.Background())
		q.cancel = cancel
		bot := botID
		go r.process(ctx, bot)
	}
	r.mu.Unlock()
}

// process drains the queue for a single bot serially. It loops while items
// remain, re-attempting due items with exponential backoff and removing
// successes. It always exits cleanly when the queue empties or the context is
// cancelled, and it recovers from any panic so a buggy upserter can never
// crash the server.
func (r *auxIndexRetry) process(ctx context.Context, botID string) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("aux index retry processor panicked", "bot_id", botID, "panic", rec)
		}
		r.mu.Lock()
		if q := r.queues[botID]; q != nil {
			q.inFlight = false
			q.cancel = nil
			// If items piled up while we were tearing down, respawn so they
			// are not stranded.
			if len(q.items) > 0 && ctx.Err() == nil {
				q.inFlight = true
				newCtx, cancel := context.WithCancel(context.Background()) //nolint:contextcheck // background retry queue owns its lifecycle
				q.cancel = cancel
				go r.process(newCtx, botID) //nolint:contextcheck // background retry queue owns its lifecycle
			}
		}
		r.mu.Unlock()
	}()

	for {
		r.mu.Lock()
		q := r.queues[botID]
		if q == nil || len(q.items) == 0 {
			r.mu.Unlock()
			return
		}

		now := time.Now()
		idx := -1
		var wait time.Duration
		for i, it := range q.items {
			if !it.nextAttempt.After(now) {
				idx = i
				break
			}
			d := it.nextAttempt.Sub(now)
			if idx == -1 || d < wait {
				wait = d
			}
		}

		if idx == -1 {
			// Nothing due yet. Sleep until the earliest nextAttempt (or until
			// cancelled), then loop.
			r.mu.Unlock()
			if wait <= 0 {
				wait = r.baseDelay
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				continue
			}
		}

		item := q.items[idx]
		// Remove optimistically; re-insert on failure.
		q.items = append(q.items[:idx], q.items[idx+1:]...)
		r.mu.Unlock()

		var err error
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("upserter panic: %v", rec)
					r.logger.Error("aux upserter panicked", "bot_id", botID, "node_id", item.nodeID, "panic", rec)
				}
			}()
			err = r.upserter.UpsertDense(ctx, item.botID, item.nodeID, item.body, item.payload)
		}()

		if err == nil {
			r.logger.Debug("aux index upsert succeeded on retry", "bot_id", botID, "node_id", item.nodeID, "attempts", item.attempts)
			continue
		}

		item.attempts++
		if item.attempts >= r.maxAttempts {
			r.logger.Warn("aux index upsert giving up after attempts", "bot_id", botID, "node_id", item.nodeID, "attempts", item.attempts, "err", err.Error())
			continue
		}

		// Exponential backoff capped at maxDelay; <=0 guards overflow.
		shift := item.attempts
		if shift < 0 || shift > 30 { //nolint:gomnd // 30 caps 2^shift within int64 range
			shift = 30
		}
		var backoff time.Duration
		//nolint:gosec // shift is bounded to <=30 above, no overflow possible
		backoff = r.baseDelay * time.Duration(int64(1)<<uint(shift))
		if backoff > r.maxDelay || backoff <= 0 {
			backoff = r.maxDelay
		}
		item.nextAttempt = time.Now().Add(backoff)

		r.logger.Warn("aux index upsert failed, will retry", "bot_id", botID, "node_id", item.nodeID, "attempts", item.attempts, "backoff", backoff.String(), "err", err.Error())

		r.mu.Lock()
		if q := r.queues[botID]; q != nil {
			q.items = append(q.items, item)
		}
		r.mu.Unlock()
	}
}

// Status reports the health of the retry queue for a bot.
// pending is the number of items waiting; degraded is true when pending > 0.
func (r *auxIndexRetry) Status(botID string) (pending int, degraded bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if q := r.queues[botID]; q != nil {
		pending = len(q.items)
	}
	degraded = pending > 0
	return pending, degraded
}

// Stop cancels every in-flight processor goroutine. Items already queued
// remain in memory and will be retried if more are enqueued.
func (r *auxIndexRetry) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.queues {
		if q.cancel != nil {
			q.cancel()
			q.cancel = nil
		}
		q.inFlight = false
	}
}
