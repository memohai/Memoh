package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type sessionEventDeliveryLeaseQueries interface {
	sessionEventDeliveryCompletionReader
	ClaimSessionEventDelivery(context.Context, sqlc.ClaimSessionEventDeliveryParams) (pgtype.Timestamptz, error)
	CompleteSessionEventDelivery(context.Context, sqlc.CompleteSessionEventDeliveryParams) (int64, error)
	RenewSessionEventDelivery(context.Context, sqlc.RenewSessionEventDeliveryParams) (pgtype.Timestamptz, error)
	ReleaseSessionEventDelivery(context.Context, sqlc.ReleaseSessionEventDeliveryParams) (int64, error)
}

type EventDeliveryLease struct {
	queries       sessionEventDeliveryLeaseQueries
	logger        *slog.Logger
	eventID       pgtype.UUID
	claimToken    pgtype.UUID
	duration      time.Duration
	renewInterval time.Duration
	stop          chan struct{}
	done          chan struct{}
	lost          context.Context
	markLost      context.CancelFunc
	stopOnce      sync.Once
	completeOnce  sync.Once
	completeErr   error
	releaseOnce   sync.Once
	releaseErr    error
	deadlineMu    sync.Mutex
	deadlineTimer *time.Timer
	deadlineGen   uint64
}

func (s *EventStore) ClaimEventDelivery(ctx context.Context, eventID string) (*EventDeliveryLease, error) {
	queries, ok := s.queries.(sessionEventDeliveryLeaseQueries)
	if !ok {
		return nil, errors.New("session event delivery lease store is not configured")
	}
	pgEventID, err := dbpkg.ParseUUID(eventID)
	if err != nil {
		return nil, fmt.Errorf("invalid event id: %w", err)
	}
	claimToken, err := dbpkg.ParseUUID(uuid.NewString())
	if err != nil {
		return nil, fmt.Errorf("create event delivery claim token: %w", err)
	}
	duration := s.deliveryLeaseDuration
	if duration <= 0 {
		duration = 2 * time.Minute
	}
	renewInterval := s.deliveryRenewInterval
	if renewInterval <= 0 || renewInterval >= duration {
		renewInterval = duration / 4
	}
	claimStartedAt := time.Now()
	claimedUntil, err := queries.ClaimSessionEventDelivery(ctx, sqlc.ClaimSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: claimToken, LeaseMs: duration.Milliseconds(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim session event delivery: %w", err)
	}
	if !claimedUntil.Valid {
		return nil, errors.New("claim session event delivery returned no expiry")
	}
	lifetimeCtx, markLost := context.WithCancel(context.WithoutCancel(ctx))
	lease := &EventDeliveryLease{
		queries:       queries,
		logger:        s.logger,
		eventID:       pgEventID,
		claimToken:    claimToken,
		duration:      duration,
		renewInterval: renewInterval,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
		lost:          lifetimeCtx,
		markLost:      markLost,
	}
	if !lease.armDeadline(claimStartedAt.Add(duration)) {
		return nil, errors.New("claim session event delivery returned an expired lease")
	}
	go lease.keepalive(lifetimeCtx)
	return lease, nil
}

func (l *EventDeliveryLease) Context(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(l.lost, cancel)
	if !l.Active() {
		cancel()
	}
	return ctx, func() {
		stop()
		cancel()
	}
}

func (l *EventDeliveryLease) Active() bool {
	select {
	case <-l.lost.Done():
		return false
	default:
		return true
	}
}

func (l *EventDeliveryLease) Done() <-chan struct{} {
	return l.done
}

func (l *EventDeliveryLease) DeliveryClaim() (DeliveryClaim, bool) {
	if l == nil || !l.eventID.Valid || !l.claimToken.Valid {
		return DeliveryClaim{}, false
	}
	return DeliveryClaim{
		EventID:    l.eventID.String(),
		ClaimToken: l.claimToken.String(),
	}, true
}

func (l *EventDeliveryLease) Complete(ctx context.Context) error {
	l.completeOnce.Do(func() {
		l.stopKeepalive()
		rows, err := l.queries.CompleteSessionEventDelivery(ctx, sqlc.CompleteSessionEventDeliveryParams{
			EventID: l.eventID, ClaimToken: l.claimToken,
		})
		if err != nil {
			l.completeErr = fmt.Errorf("complete session event delivery: %w", err)
			return
		}
		if rows == 0 {
			completed, readErr := l.queries.IsSessionEventDeliveryCompleted(ctx, l.eventID)
			if readErr != nil {
				l.completeErr = fmt.Errorf("read session event delivery completion: %w", readErr)
				return
			}
			if completed {
				return
			}
		}
		if rows != 1 {
			l.completeErr = errors.New("session event delivery has no durable completion evidence")
		}
	})
	return l.completeErr
}

func (l *EventDeliveryLease) Release(ctx context.Context) error {
	l.releaseOnce.Do(func() {
		l.stopKeepalive()
		rows, err := l.queries.ReleaseSessionEventDelivery(ctx, sqlc.ReleaseSessionEventDeliveryParams{
			EventID: l.eventID, ClaimToken: l.claimToken,
		})
		if err != nil {
			l.releaseErr = fmt.Errorf("release session event delivery: %w", err)
			return
		}
		if rows != 1 {
			l.releaseErr = errors.New("session event delivery lease was lost before release")
		}
	})
	return l.releaseErr
}

func (l *EventDeliveryLease) stopKeepalive() {
	l.stopOnce.Do(func() {
		close(l.stop)
		l.stopDeadline()
		l.markLost()
	})
	<-l.done
}

func (l *EventDeliveryLease) armDeadline(expiry time.Time) bool {
	l.deadlineMu.Lock()
	defer l.deadlineMu.Unlock()
	if l.lost.Err() != nil {
		return false
	}
	l.deadlineGen++
	generation := l.deadlineGen
	if l.deadlineTimer != nil {
		l.deadlineTimer.Stop()
	}
	delay := time.Until(expiry)
	if delay <= 0 {
		l.deadlineTimer = nil
		l.markLost()
		return false
	}
	l.deadlineTimer = time.AfterFunc(delay, func() {
		l.deadlineMu.Lock()
		defer l.deadlineMu.Unlock()
		if l.deadlineGen != generation {
			return
		}
		l.deadlineTimer = nil
		l.markLost()
	})
	return true
}

func conservativeLocalLeaseDeadline(now, claimedUntil time.Time, duration time.Duration) time.Time {
	latest := now.Add(duration)
	if claimedUntil.Before(latest) {
		return claimedUntil
	}
	return latest
}

func (l *EventDeliveryLease) stopDeadline() {
	l.deadlineMu.Lock()
	defer l.deadlineMu.Unlock()
	l.deadlineGen++
	if l.deadlineTimer != nil {
		l.deadlineTimer.Stop()
		l.deadlineTimer = nil
	}
}

func (l *EventDeliveryLease) keepalive(ctx context.Context) {
	defer close(l.done)
	defer l.stopDeadline()
	ticker := time.NewTicker(l.renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(ctx, l.renewInterval)
			claimedUntil, err := l.queries.RenewSessionEventDelivery(renewCtx, sqlc.RenewSessionEventDeliveryParams{
				EventID: l.eventID, ClaimToken: l.claimToken, LeaseMs: l.duration.Milliseconds(),
			})
			cancel()
			renewedAt := time.Now()
			if l.lost.Err() != nil {
				return
			}
			if err == nil && !claimedUntil.Valid {
				err = errors.New("renew session event delivery returned no expiry")
			}
			if err == nil && l.armDeadline(conservativeLocalLeaseDeadline(renewedAt, claimedUntil.Time, l.duration)) {
				continue
			}
			if err == nil {
				return
			}
			l.markLost()
			if l.logger != nil {
				l.logger.Error("event delivery lease renewal failed",
					slog.String("event_id", l.eventID.String()),
					slog.Any("error", err))
			}
			return
		}
	}
}
