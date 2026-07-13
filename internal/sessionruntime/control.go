package sessionruntime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

func (m *Manager) localControl(streamID string) *runControl {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.controls[strings.TrimSpace(streamID)]
}

func (m *Manager) forgetLocalControl(ctx context.Context, streamID string) {
	m.mu.Lock()
	ctrl := m.controls[strings.TrimSpace(streamID)]
	delete(m.controls, strings.TrimSpace(streamID))
	m.mu.Unlock()
	_ = waitRunControlReady(ctx, ctrl)
	m.stopLeaseRenewal(ctrl)
}

func (m *Manager) removeLocalControl(streamID string, expected *runControl) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.TrimSpace(streamID)
	if m.controls[key] == expected {
		delete(m.controls, key)
	}
}

func (m *Manager) stopAllLocalControls(ctx context.Context) {
	m.mu.Lock()
	controls := make([]*runControl, 0, len(m.controls))
	for streamID, ctrl := range m.controls {
		controls = append(controls, ctrl)
		delete(m.controls, streamID)
	}
	m.mu.Unlock()
	for _, ctrl := range controls {
		select {
		case ctrl.abortCh <- struct{}{}:
		default:
		}
		if ctrl.cancel != nil {
			ctrl.cancel()
		}
		_ = waitRunControlReady(ctx, ctrl)
		m.stopLeaseRenewal(ctrl)
	}
}

func (c *runControl) markReady() {
	if c == nil {
		return
	}
	c.readyOnce.Do(func() { close(c.ready) })
}

func waitRunControlReady(ctx context.Context, ctrl *runControl) error {
	if ctrl == nil || ctrl.ready == nil {
		return nil
	}
	select {
	case <-ctrl.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) startLeaseRenewal(ctx context.Context, ctrl *runControl) {
	if m == nil || ctrl == nil || m.distributed == nil || m.ownerLeaseTTL <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	ctrl.leaseStop = cancel
	ctrl.leaseDone = done
	interval := leaseRenewInterval(m.ownerLeaseTTL)
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !ctrl.leaseIsValidAt(time.Now()) {
					m.cancelRunControl(ctrl)
					return
				}
				now, err := m.backend.Now(ctx)
				if err == nil {
					err = m.distributed.RenewLease(ctx, Key{BotID: ctrl.botID, SessionID: ctrl.sessionID}, ctrl.streamID, m.ownerID, now, now.Add(m.ownerLeaseTTL))
				}
				if err == nil {
					ctrl.setLeaseValidUntil(time.Now().Add(m.ownerLeaseTTL))
					continue
				}
				if ctx.Err() != nil {
					return
				}
				m.logger.Warn("renew runtime owner lease failed", slog.Any("error", err), slog.String("stream_id", ctrl.streamID))
				if errors.Is(err, ErrRunOwnershipLost) || !ctrl.leaseIsValidAt(time.Now()) {
					m.cancelRunControl(ctrl)
					return
				}
			}
		}
	}()
}

func (c *runControl) setLeaseValidUntil(deadline time.Time) {
	if c == nil {
		return
	}
	c.leaseMu.Lock()
	c.leaseValidUntil = deadline
	c.leaseMu.Unlock()
}

func (c *runControl) leaseIsValidAt(now time.Time) bool {
	if c == nil {
		return false
	}
	c.leaseMu.RLock()
	deadline := c.leaseValidUntil
	c.leaseMu.RUnlock()
	return !deadline.IsZero() && now.Before(deadline)
}

func (*Manager) cancelRunControl(ctrl *runControl) {
	if ctrl == nil {
		return
	}
	select {
	case ctrl.abortCh <- struct{}{}:
	default:
	}
	if ctrl.cancel != nil {
		ctrl.cancel()
	}
}

func (*Manager) stopLeaseRenewal(ctrl *runControl) {
	if ctrl == nil || ctrl.leaseStop == nil {
		return
	}
	ctrl.leaseStop()
	if ctrl.leaseDone != nil {
		<-ctrl.leaseDone
	}
	ctrl.leaseStop = nil
	ctrl.leaseDone = nil
}
