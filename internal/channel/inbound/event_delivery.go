package inbound

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

var ErrEventDeliveryInFlight = errors.New("event delivery is already in flight")

func currentEventDeliveryClaim(eventID string, lease *pipelinepkg.EventDeliveryLease) (*conversation.DeliveryClaim, error) {
	if lease == nil {
		return nil, errors.New("pipeline event delivery lease is missing")
	}
	claim, ok := lease.DeliveryClaim()
	if !ok {
		return nil, errors.New("pipeline event delivery claim is missing")
	}
	return checkedEventDeliveryClaim(eventID, claim)
}

func checkedEventDeliveryClaim(eventID string, claim pipelinepkg.DeliveryClaim) (*conversation.DeliveryClaim, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || !strings.EqualFold(eventID, strings.TrimSpace(claim.EventID)) {
		return nil, fmt.Errorf("pipeline event delivery claim does not match event %q", eventID)
	}
	if strings.TrimSpace(claim.ClaimToken) == "" {
		return nil, fmt.Errorf("pipeline event delivery claim for event %q has no token", eventID)
	}
	return &conversation.DeliveryClaim{
		EventID:    strings.TrimSpace(claim.EventID),
		ClaimToken: strings.TrimSpace(claim.ClaimToken),
	}, nil
}

type injectedEventDeliverySignals struct {
	persisted      chan error
	streamFinished chan error
	persistOnce    sync.Once
	streamOnce     sync.Once
}

func newInjectedEventDeliverySignals() *injectedEventDeliverySignals {
	return &injectedEventDeliverySignals{
		persisted:      make(chan error, 1),
		streamFinished: make(chan error, 1),
	}
}

func (s *injectedEventDeliverySignals) reportPersisted(err error) {
	s.persistOnce.Do(func() { s.persisted <- err })
}

func (s *injectedEventDeliverySignals) reportStreamFinished(err error) {
	s.streamOnce.Do(func() { s.streamFinished <- err })
}

func (p *ChannelInboundProcessor) claimEventDelivery(
	ctx context.Context,
	eventID string,
	reuseExisting bool,
) (*pipelinepkg.EventDeliveryLease, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || p.eventStore == nil {
		return nil, errors.New("pipeline event delivery has no durable event store")
	}
	if existing, loaded := p.inflightEventDelivery.Load(eventID); loaded {
		lease, ok := existing.(*pipelinepkg.EventDeliveryLease)
		if !ok {
			return nil, errors.New("pipeline event delivery has invalid in-flight state")
		}
		if !lease.Active() {
			p.inflightEventDelivery.CompareAndDelete(eventID, lease)
		} else {
			if reuseExisting {
				return lease, nil
			}
			return nil, ErrEventDeliveryInFlight
		}
	}
	lease, err := p.eventStore.ClaimEventDelivery(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, ErrEventDeliveryInFlight
	}
	if existing, loaded := p.inflightEventDelivery.LoadOrStore(eventID, lease); loaded {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		releaseErr := lease.Release(releaseCtx)
		cancel()
		if releaseErr != nil && p.logger != nil {
			p.logger.Warn("release redundant event delivery lease failed",
				slog.String("event_id", eventID),
				slog.Any("error", releaseErr))
		}
		if reuseExisting {
			existingLease, ok := existing.(*pipelinepkg.EventDeliveryLease)
			if !ok {
				return nil, errors.New("pipeline event delivery has invalid in-flight state")
			}
			return existingLease, nil
		}
		return nil, ErrEventDeliveryInFlight
	}
	go func() {
		<-lease.Done()
		p.inflightEventDelivery.CompareAndDelete(eventID, lease)
	}()
	return lease, nil
}

func (p *ChannelInboundProcessor) releaseEventDeliveryClaim(ctx context.Context, eventID string, lease *pipelinepkg.EventDeliveryLease) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || lease == nil {
		return nil
	}
	p.inflightEventDelivery.CompareAndDelete(eventID, lease)
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := lease.Release(releaseCtx); err != nil {
		return fmt.Errorf("release pipeline event delivery: %w", err)
	}
	return nil
}

func (p *ChannelInboundProcessor) completeEventDeliveryClaim(ctx context.Context, eventID string, lease *pipelinepkg.EventDeliveryLease) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || lease == nil {
		return nil
	}
	p.inflightEventDelivery.CompareAndDelete(eventID, lease)
	completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	completeErr := lease.Complete(completeCtx)
	cancel()
	if completeErr == nil {
		return nil
	}
	releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	releaseErr := lease.Release(releaseCtx)
	releaseCancel()
	return errors.Join(fmt.Errorf("complete pipeline event delivery: %w", completeErr), releaseErr)
}

func (p *ChannelInboundProcessor) completePendingEventHistory(ctx context.Context, pending bool, messageID string) error {
	if !pending {
		return nil
	}
	if strings.TrimSpace(messageID) == "" {
		return errors.New("pending event delivery has no durable message")
	}
	completer, ok := p.message.(pendingDeliveryCompleter)
	if !ok {
		return errors.New("message writer does not support pending delivery completion")
	}
	if err := completer.CompletePendingDelivery(ctx, messageID); err != nil {
		return fmt.Errorf("complete pending event delivery: %w", err)
	}
	return nil
}

func (p *ChannelInboundProcessor) completeQueuedReplayWithoutResponse(ctx context.Context, replay queuedReplayState) error {
	if strings.TrimSpace(replay.eventID) == "" {
		return errors.New("queued replay has no durable event")
	}
	return p.completePendingEventHistory(ctx, true, replay.persistedUserMessageID)
}

func (p *ChannelInboundProcessor) finalizeInjectedEventDelivery(
	ctx context.Context,
	botID string,
	routeID string,
	eventID string,
	lease *pipelinepkg.EventDeliveryLease,
	signals *injectedEventDeliverySignals,
) {
	if lease == nil || signals == nil {
		return
	}
	var persistedErr error
	var streamErr error
	persisted := false
	streamFinished := false
	for !persisted || !streamFinished {
		select {
		case err := <-signals.persisted:
			persisted = true
			persistedErr = err
			if err != nil {
				p.cancelActiveStreamForRoute(botID, routeID, "injected message persistence failed")
			}
		case err := <-signals.streamFinished:
			streamFinished = true
			streamErr = err
		case <-lease.Done():
			p.cancelActiveStreamForRoute(botID, routeID, "injected event delivery lease lost")
			return
		}
	}
	if persistedErr == nil && streamErr == nil {
		if err := p.completeEventDeliveryClaim(ctx, eventID, lease); err != nil && p.logger != nil {
			p.logger.Error("complete injected event delivery failed",
				slog.String("event_id", eventID),
				slog.Any("error", err))
		}
		return
	}
	if err := p.releaseEventDeliveryClaim(ctx, eventID, lease); err != nil && p.logger != nil {
		p.logger.Error("release injected event delivery failed",
			slog.String("event_id", eventID),
			slog.Any("error", err))
	}
}
