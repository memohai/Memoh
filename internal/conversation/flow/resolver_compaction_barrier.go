package flow

import (
	"strings"
	"sync"
)

type sessionCompactionGate struct {
	lock sync.RWMutex
	refs int
}

func (r *Resolver) DeferSessionCompaction(botID, sessionID string) func() {
	gate, key := r.acquireSessionCompactionGate(botID, sessionID)
	if gate == nil {
		return func() {}
	}
	gate.lock.RLock()
	var once sync.Once
	return func() {
		once.Do(func() {
			gate.lock.RUnlock()
			r.releaseSessionCompactionGate(key, gate)
		})
	}
}

func (r *Resolver) enterSessionCompaction(botID, sessionID string) func() {
	gate, key := r.acquireSessionCompactionGate(botID, sessionID)
	if gate == nil {
		return func() {}
	}
	gate.lock.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			gate.lock.Unlock()
			r.releaseSessionCompactionGate(key, gate)
		})
	}
}

func (r *Resolver) acquireSessionCompactionGate(botID, sessionID string) (*sessionCompactionGate, string) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if r == nil || botID == "" || sessionID == "" {
		return nil, ""
	}
	key := sessionTurnKey(botID, sessionID)
	r.sessionCompactionMu.Lock()
	defer r.sessionCompactionMu.Unlock()
	if r.sessionCompactions == nil {
		r.sessionCompactions = make(map[string]*sessionCompactionGate)
	}
	gate := r.sessionCompactions[key]
	if gate == nil {
		gate = &sessionCompactionGate{}
		r.sessionCompactions[key] = gate
	}
	gate.refs++
	return gate, key
}

func (r *Resolver) releaseSessionCompactionGate(key string, gate *sessionCompactionGate) {
	r.sessionCompactionMu.Lock()
	defer r.sessionCompactionMu.Unlock()
	gate.refs--
	if gate.refs == 0 && r.sessionCompactions[key] == gate {
		delete(r.sessionCompactions, key)
	}
}
