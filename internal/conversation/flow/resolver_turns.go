package flow

import (
	"context"
	"strings"
	"sync"
)

func sessionTurnKey(botID, sessionID string) string {
	return strings.TrimSpace(botID) + ":" + strings.TrimSpace(sessionID)
}

func (r *Resolver) enterSessionTurn(_ context.Context, botID, sessionID string) func() {
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

	return r.makeSessionTurnReleaser(key)
}

func (r *Resolver) makeSessionTurnReleaser(key string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			r.sessionTurnMu.Lock()
			switch refs := r.sessionTurnRefs[key] - 1; {
			case refs > 0:
				r.sessionTurnRefs[key] = refs
			default:
				delete(r.sessionTurnRefs, key)
			}
			r.sessionTurnMu.Unlock()
		})
	}
}
