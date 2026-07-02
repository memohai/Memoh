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
	lock := r.sessionTurnLockLocked(key)
	r.sessionTurnRefs[key]++
	r.sessionTurnMu.Unlock()

	lock.Lock()
	return r.makeSessionTurnReleaser(key, lock)
}

func (r *Resolver) tryEnterIdleSessionTurn(_ context.Context, botID, sessionID string) (func(), bool) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return func() {}, true
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	lock := r.sessionTurnLockLocked(key)
	if r.sessionTurnRefs[key] > 0 {
		r.sessionTurnMu.Unlock()
		return func() {}, false
	}
	r.sessionTurnRefs[key] = 1
	lock.Lock()
	r.sessionTurnMu.Unlock()
	return r.makeSessionTurnReleaser(key, lock), true
}

func (r *Resolver) sessionTurnLockLocked(key string) *sync.Mutex {
	if r.sessionTurnRefs == nil {
		r.sessionTurnRefs = make(map[string]int)
	}
	if r.sessionTurnLocks == nil {
		r.sessionTurnLocks = make(map[string]*sync.Mutex)
	}
	lock := r.sessionTurnLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		r.sessionTurnLocks[key] = lock
	}
	return lock
}

func (r *Resolver) makeSessionTurnReleaser(key string, lock *sync.Mutex) func() {
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
			lock.Unlock()
		})
	}
}
