package flow

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

func sessionTurnKey(botID, sessionID string) string {
	return strings.TrimSpace(botID) + ":" + strings.TrimSpace(sessionID)
}

func (r *Resolver) enterSessionTurn(ctx context.Context, botID, sessionID string) func() {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return func() {}
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	if r.sessionTurnRefs == nil {
		r.sessionTurnRefs = make(map[string]int)
	}
	r.sessionTurnRefs[key]++
	r.sessionTurnMu.Unlock()

	return r.makeSessionTurnReleaser(ctx, key, botID, sessionID)
}

func (r *Resolver) tryEnterIdleSessionTurn(ctx context.Context, botID, sessionID string) (func(), bool) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return nil, false
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	if r.sessionTurnRefs == nil {
		r.sessionTurnRefs = make(map[string]int)
	}
	if r.sessionTurnRefs[key] > 0 {
		r.sessionTurnMu.Unlock()
		return nil, false
	}
	r.sessionTurnRefs[key] = 1
	r.sessionTurnMu.Unlock()

	return r.makeSessionTurnReleaser(ctx, key, botID, sessionID), true
}

func (r *Resolver) makeSessionTurnReleaser(ctx context.Context, key, botID, sessionID string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			becameIdle := false

			r.sessionTurnMu.Lock()
			switch refs := r.sessionTurnRefs[key] - 1; {
			case refs > 0:
				r.sessionTurnRefs[key] = refs
			default:
				delete(r.sessionTurnRefs, key)
				becameIdle = true
			}
			r.sessionTurnMu.Unlock()

			if becameIdle {
				r.maybeTriggerDeferredBackgroundNotifications(ctx, botID, sessionID)
			}
		})
	}
}

func (r *Resolver) markDeferredBackgroundNotification(botID, sessionID string) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return
	}
	r.bgNotifDeferred.Store(sessionTurnKey(botID, sessionID), true)
}

func (r *Resolver) takeDeferredBackgroundNotification(botID, sessionID string) bool {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return false
	}
	_, loaded := r.bgNotifDeferred.LoadAndDelete(sessionTurnKey(botID, sessionID))
	return loaded
}

func (r *Resolver) maybeTriggerDeferredBackgroundNotifications(ctx context.Context, botID, sessionID string) {
	if !r.takeDeferredBackgroundNotification(botID, sessionID) {
		return
	}
	if r.bgManager == nil || !r.bgManager.HasNotifications(botID, sessionID) {
		return
	}

	r.logger.Info("background notification trigger queued after session became idle",
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
	)
	if ctx == nil {
		return
	}
	go r.TriggerBackgroundNotification(context.WithoutCancel(ctx), botID, sessionID)
}
