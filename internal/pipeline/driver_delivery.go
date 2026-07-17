package pipeline

import (
	"context"
	"log/slog"
	"time"
)

const discussEventDeliveryFinalizeTimeout = 5 * time.Second

func discussEventDeliveriesActive(deliveries []DiscussEventDelivery) bool {
	for _, delivery := range deliveries {
		if delivery.Lease != nil && !delivery.Lease.Active() {
			return false
		}
	}
	return true
}

func mergeDiscussEventDeliveries(groups ...[]DiscussEventDelivery) ([]DiscussEventDelivery, []DiscussEventDelivery) {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	if count == 0 {
		return nil, nil
	}
	merged := make([]DiscussEventDelivery, 0, count)
	discarded := make([]DiscussEventDelivery, 0)
	seen := make(map[string]int, count)
	for _, group := range groups {
		for _, delivery := range group {
			if delivery.EventID != "" {
				if index, ok := seen[delivery.EventID]; ok {
					existing := merged[index]
					if existing.Lease == delivery.Lease {
						continue
					}
					existingActive := existing.Lease != nil && existing.Lease.Active()
					deliveryActive := delivery.Lease != nil && delivery.Lease.Active()
					if deliveryActive || !existingActive {
						merged[index] = delivery
						discarded = append(discarded, existing)
					} else {
						discarded = append(discarded, delivery)
					}
					continue
				}
				seen[delivery.EventID] = len(merged)
			}
			merged = append(merged, delivery)
		}
	}
	return merged, discarded
}

func bindDiscussEventDeliveries(parent context.Context, deliveries []DiscussEventDelivery) (context.Context, context.CancelFunc, bool) {
	ctx, cancel := context.WithCancel(parent)
	stops := make([]func() bool, 0, len(deliveries))
	active := true
	for _, delivery := range deliveries {
		if delivery.Lease == nil {
			continue
		}
		if !delivery.Lease.Active() {
			active = false
			break
		}
		stops = append(stops, context.AfterFunc(delivery.Lease.lost, cancel))
		if !delivery.Lease.Active() {
			active = false
			break
		}
	}
	cancelAll := func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
		cancel()
	}
	if ctx.Err() != nil {
		active = false
	}
	return ctx, cancelAll, active
}

func completeDiscussEventDeliveries(parent context.Context, deliveries []DiscussEventDelivery, log *slog.Logger) {
	for _, delivery := range deliveries {
		if delivery.Lease == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), discussEventDeliveryFinalizeTimeout)
		err := delivery.Lease.Complete(ctx)
		cancel()
		if err == nil {
			continue
		}
		log.Error("discuss event delivery completion failed",
			slog.String("event_id", delivery.EventID),
			slog.Any("error", err))
		releaseDiscussEventDeliveries(parent, []DiscussEventDelivery{delivery}, log)
	}
}

func releaseDiscussEventDeliveries(parent context.Context, deliveries []DiscussEventDelivery, log *slog.Logger) {
	for _, delivery := range deliveries {
		if delivery.Lease == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), discussEventDeliveryFinalizeTimeout)
		err := delivery.Lease.Release(ctx)
		cancel()
		if err != nil {
			log.Error("discuss event delivery release failed",
				slog.String("event_id", delivery.EventID),
				slog.Any("error", err))
		}
	}
}

func (d *DiscussDriver) releaseDiscussSessionDeliveries(ctx context.Context, sess *discussSession) {
	var deliveries []DiscussEventDelivery
	if sess.pendingCursor != nil {
		var discarded []DiscussEventDelivery
		deliveries, discarded = mergeDiscussEventDeliveries(deliveries, sess.pendingCursor.deliveries)
		releaseDiscussEventDeliveries(ctx, discarded, d.logger)
		sess.pendingCursor = nil
	}
	for {
		select {
		case notification := <-sess.rcCh:
			var discarded []DiscussEventDelivery
			deliveries, discarded = mergeDiscussEventDeliveries(deliveries, notification.deliveries)
			releaseDiscussEventDeliveries(ctx, discarded, d.logger)
		default:
			releaseDiscussEventDeliveries(ctx, deliveries, d.logger)
			return
		}
	}
}
