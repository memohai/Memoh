package inbound

import (
	"context"
	"log/slog"
	"strings"
	"time"

	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

const (
	queuedRetryBaseDelay = 100 * time.Millisecond
	queuedRetryMaxDelay  = 5 * time.Second
)

func defaultQueuedRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	delay := queuedRetryBaseDelay
	for i := 1; i < attempt && delay < queuedRetryMaxDelay; i++ {
		delay *= 2
	}
	if delay > queuedRetryMaxDelay {
		return queuedRetryMaxDelay
	}
	return delay
}

func (p *ChannelInboundProcessor) Close() {
	if p == nil {
		return
	}
	p.queueRetryMu.Lock()
	if p.queueRetryCancel != nil {
		p.queueRetryCancel()
		p.queueRetryCancel = nil
	}
	p.queueRetryMu.Unlock()
	p.queueRetryWG.Wait()
}

func finalizeQueueTransition(ctx context.Context, result MarkDoneResult, streamErr error) {
	for _, fn := range result.PendingPersists {
		fn(ctx)
	}
	for _, message := range result.InjectedMessages {
		if message.OnStreamFinished != nil {
			message.OnStreamFinished(streamErr)
		}
	}
}

func (p *ChannelInboundProcessor) processQueuedTaskChain(
	ctx context.Context,
	routeID string,
	task QueuedTask,
	async bool,
	retryAttempt int,
) {
	for {
		if retryAttempt > 0 && !p.waitForQueuedRetry(retryAttempt) {
			p.releaseQueuedRouteOnShutdown(ctx, routeID, task)
			return
		}
		streamErr := p.handleQueuedTask(ctx, routeID, task)
		if streamErr != nil {
			failedAttempts := retryAttempt + 1
			p.logQueuedTaskFailure(routeID, streamErr, failedAttempts)
			if !async {
				p.startQueuedRetry(ctx, routeID, task)
				return
			}
			retryAttempt = failedAttempts
			continue
		}

		retryAttempt = 0
		result := p.dispatcher.FinishActive(routeID)
		finalizeQueueTransition(ctx, result, nil)
		if len(result.QueuedTasks) == 0 {
			return
		}
		task = result.QueuedTasks[0]
	}
}

func (p *ChannelInboundProcessor) runQueuedTaskChain(
	ctx context.Context,
	routeID string,
	task QueuedTask,
	async bool,
	retryAttempt int,
) {
	if !p.beginQueuedTaskChain() {
		p.releaseQueuedRouteOnShutdown(ctx, routeID, task)
		return
	}
	defer p.queueRetryWG.Done()
	p.processQueuedTaskChain(ctx, routeID, task, async, retryAttempt)
}

func (p *ChannelInboundProcessor) handleQueuedTask(ctx context.Context, routeID string, task QueuedTask) error {
	if p.logger != nil {
		p.logger.Info("processing queued task",
			slog.String("route_id", routeID),
			slog.String("query", strings.TrimSpace(task.Text)),
		)
	}
	taskCtx := task.Ctx //nolint:contextcheck // queued work must retain its own enqueue-time values across retries
	if taskCtx == nil {
		taskCtx = ctx
	}
	taskCtx, cancel := context.WithCancel(context.WithoutCancel(taskCtx))
	stopCancel := func() bool { return true }
	if p.queueRetryContext != nil {
		stopCancel = context.AfterFunc(p.queueRetryContext, cancel)
	}
	defer func() {
		stopCancel()
		cancel()
	}()
	queuedCtx := withQueuedReplayState(taskCtx, task)
	return p.HandleInbound(queuedCtx, task.Cfg, task.Msg, task.Sender) //nolint:contextcheck // queued work intentionally survives its enqueue caller
}

func (p *ChannelInboundProcessor) startQueuedRetry(ctx context.Context, routeID string, task QueuedTask) {
	if !p.beginQueuedTaskChain() {
		p.releaseQueuedRouteOnShutdown(ctx, routeID, task)
		return
	}
	go func() {
		defer p.queueRetryWG.Done()
		p.processQueuedTaskChain(ctx, routeID, task, true, 1)
	}()
}

func (p *ChannelInboundProcessor) beginQueuedTaskChain() bool {
	p.queueRetryMu.Lock()
	defer p.queueRetryMu.Unlock()
	if p.queueRetryContext == nil || p.queueRetryContext.Err() != nil {
		return false
	}
	p.queueRetryWG.Add(1)
	return true
}

func (p *ChannelInboundProcessor) waitForQueuedRetry(attempt int) bool {
	delay := defaultQueuedRetryDelay(attempt)
	if p.queueRetryDelay != nil {
		delay = p.queueRetryDelay(attempt)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-p.queueRetryContext.Done():
		return false
	}
}

func (p *ChannelInboundProcessor) releaseQueuedRouteOnShutdown(ctx context.Context, routeID string, task QueuedTask) {
	shutdownCtx := context.WithoutCancel(ctx)
	p.releaseQueuedTaskDelivery(shutdownCtx, task)
	if p.dispatcher == nil {
		return
	}
	result := p.dispatcher.MarkDone(routeID)
	finalizeQueueTransition(shutdownCtx, result, context.Canceled)
	for _, queued := range result.QueuedTasks {
		p.releaseQueuedTaskDelivery(shutdownCtx, queued)
	}
}

func (p *ChannelInboundProcessor) releaseQueuedTaskDelivery(ctx context.Context, task QueuedTask) {
	eventID := strings.TrimSpace(task.EventID)
	if eventID == "" {
		return
	}
	value, loaded := p.inflightEventDelivery.Load(eventID)
	if !loaded {
		return
	}
	lease, ok := value.(*pipelinepkg.EventDeliveryLease)
	if !ok {
		return
	}
	if err := p.releaseEventDeliveryClaim(ctx, eventID, lease); err != nil && p.logger != nil {
		p.logger.Error("release queued event delivery on shutdown failed",
			slog.String("event_id", eventID),
			slog.Any("error", err))
	}
}

func (p *ChannelInboundProcessor) logQueuedTaskFailure(routeID string, err error, attempt int) {
	if p.logger == nil {
		return
	}
	p.logger.Error("queued task processing failed",
		slog.String("route_id", routeID),
		slog.Int("attempt", attempt),
		slog.Any("error", err))
}
