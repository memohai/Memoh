package pipeline

import (
	"context"
	"log/slog"
	"strings"
)

func (d *DiscussDriver) loadDiscussCursor(ctx context.Context, cfg DiscussSessionConfig, log *slog.Logger) DiscussCursorPosition {
	if d.deps.CursorStore == nil {
		return DiscussCursorPosition{}
	}
	position, err := d.deps.CursorStore.GetDiscussCursor(ctx, cfg.SessionID, discussCursorScope(cfg))
	if err != nil {
		log.Warn("discuss cursor load failed", slog.Any("error", err))
		return DiscussCursorPosition{}
	}
	return position
}

func (d *DiscussDriver) advanceDiscussCursor(ctx context.Context, sess *discussSession, cfg DiscussSessionConfig, position DiscussCursorPosition, log *slog.Logger) bool {
	if position.EventCursor <= sess.lastProcessedCursor {
		return sess.pendingCursor == nil
	}
	if err := d.persistDiscussCursor(ctx, cfg, position, log); err != nil {
		if persisted, ok := d.durableDiscussCursorCovers(ctx, cfg, position, log); ok {
			sess.lastProcessedCursor = persisted.EventCursor
			sess.pendingCursor = nil
			return true
		}
		sess.pendingCursor = &pendingDiscussCursor{config: cfg, position: position}
		return false
	}
	sess.lastProcessedCursor = position.EventCursor
	sess.pendingCursor = nil
	return true
}

type pendingDiscussCursorRetryResult uint8

const (
	pendingDiscussCursorRetryPending pendingDiscussCursorRetryResult = iota
	pendingDiscussCursorRetryCommitted
	pendingDiscussCursorRetryReconciled
	pendingDiscussCursorRetryAbandoned
)

func (d *DiscussDriver) retryPendingDiscussCursor(
	ctx context.Context,
	sess *discussSession,
	log *slog.Logger,
) pendingDiscussCursorRetryResult {
	pending := sess.pendingCursor
	if pending == nil {
		return pendingDiscussCursorRetryCommitted
	}
	retryCtx, cancel, active := bindDiscussEventDeliveries(ctx, pending.deliveries)
	if !active {
		cancel()
		if persisted, ok := d.durableDiscussCursorCovers(ctx, pending.config, pending.position, log); ok {
			sess.lastProcessedCursor = persisted.EventCursor
			sess.pendingCursor = nil
			return pendingDiscussCursorRetryReconciled
		}
		sess.pendingCursor = nil
		return pendingDiscussCursorRetryAbandoned
	}
	err := d.persistDiscussCursor(retryCtx, pending.config, pending.position, log)
	deliveriesActive := discussEventDeliveriesActive(pending.deliveries)
	if err != nil {
		reconcileCtx := retryCtx
		if !deliveriesActive || retryCtx.Err() != nil {
			reconcileCtx = ctx
		}
		if persisted, ok := d.durableDiscussCursorCovers(reconcileCtx, pending.config, pending.position, log); ok {
			cancel()
			sess.lastProcessedCursor = persisted.EventCursor
			sess.pendingCursor = nil
			if deliveriesActive {
				return pendingDiscussCursorRetryCommitted
			}
			return pendingDiscussCursorRetryReconciled
		}
		cancel()
		if !deliveriesActive {
			sess.pendingCursor = nil
			return pendingDiscussCursorRetryAbandoned
		}
		return pendingDiscussCursorRetryPending
	}
	cancel()
	sess.lastProcessedCursor = pending.position.EventCursor
	sess.pendingCursor = nil
	return pendingDiscussCursorRetryCommitted
}

func (d *DiscussDriver) durableDiscussCursorCovers(
	ctx context.Context,
	cfg DiscussSessionConfig,
	target DiscussCursorPosition,
	log *slog.Logger,
) (DiscussCursorPosition, bool) {
	if d.deps.CursorStore == nil {
		return DiscussCursorPosition{}, false
	}
	persisted, err := d.deps.CursorStore.GetDiscussCursor(ctx, cfg.SessionID, discussCursorScope(cfg))
	if err != nil {
		log.Warn("discuss cursor reconciliation failed", slog.Any("error", err))
		return DiscussCursorPosition{}, false
	}
	return persisted, persisted.EventCursor >= target.EventCursor
}

func (d *DiscussDriver) persistDiscussCursor(ctx context.Context, cfg DiscussSessionConfig, position DiscussCursorPosition, log *slog.Logger) error {
	if d.deps.CursorStore == nil {
		return nil
	}
	if err := d.deps.CursorStore.UpsertDiscussCursor(ctx,
		cfg.SessionID,
		discussCursorScope(cfg),
		strings.TrimSpace(cfg.RouteID),
		strings.TrimSpace(cfg.CurrentPlatform),
		position,
	); err != nil {
		log.Warn("discuss cursor persist failed",
			slog.Any("error", err),
			slog.Int64("event_cursor", position.EventCursor),
			slog.Int64("source_cursor", position.SourceCursor))
		return err
	}
	return nil
}

func discussCursorScope(cfg DiscussSessionConfig) string {
	if routeID := strings.TrimSpace(cfg.RouteID); routeID != "" {
		return "route:" + routeID
	}
	platform := strings.TrimSpace(cfg.CurrentPlatform)
	identityID := strings.TrimSpace(cfg.ChannelIdentityID)
	switch {
	case platform != "" && identityID != "":
		return "source:" + platform + ":" + identityID
	case platform != "":
		return "source:" + platform
	default:
		return "default"
	}
}
