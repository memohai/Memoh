package flow

import (
	"strings"
	"sync"
)

type sessionCompactionGate struct {
	lock    sync.RWMutex
	refs    int
	readers map[string]map[*sessionCompactionReader]struct{}
}

type sessionCompactionReader struct {
	gate   *sessionCompactionGate
	mu     sync.Mutex
	held   bool
	closed bool
}

func (r *Resolver) DeferSessionCompaction(botID, sessionID, streamID string) func() {
	gate, key := r.acquireSessionCompactionGate(botID, sessionID)
	if gate == nil {
		return func() {}
	}
	gate.lock.RLock()
	reader := &sessionCompactionReader{gate: gate, held: true}
	streamID = strings.TrimSpace(streamID)
	if streamID != "" {
		r.sessionCompactionMu.Lock()
		if gate.readers == nil {
			gate.readers = make(map[string]map[*sessionCompactionReader]struct{})
		}
		if gate.readers[streamID] == nil {
			gate.readers[streamID] = make(map[*sessionCompactionReader]struct{})
		}
		gate.readers[streamID][reader] = struct{}{}
		r.sessionCompactionMu.Unlock()
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			reader.close()
			if streamID != "" {
				r.sessionCompactionMu.Lock()
				delete(gate.readers[streamID], reader)
				if len(gate.readers[streamID]) == 0 {
					delete(gate.readers, streamID)
				}
				r.sessionCompactionMu.Unlock()
			}
			r.releaseSessionCompactionGate(key, gate)
		})
	}
}

func (reader *sessionCompactionReader) suspend() bool {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed || !reader.held {
		return false
	}
	reader.gate.lock.RUnlock()
	reader.held = false
	return true
}

func (reader *sessionCompactionReader) resume() {
	reader.gate.lock.RLock()
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed {
		reader.gate.lock.RUnlock()
		return
	}
	reader.held = true
}

func (reader *sessionCompactionReader) close() {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed {
		return
	}
	reader.closed = true
	if reader.held {
		reader.gate.lock.RUnlock()
		reader.held = false
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

func (r *Resolver) enterSessionCompactionForStream(botID, sessionID, streamID string) func() {
	gate, key := r.acquireSessionCompactionGate(botID, sessionID)
	if gate == nil {
		return func() {}
	}
	readers := r.sessionCompactionReaders(gate, streamID)
	suspended := make([]*sessionCompactionReader, 0, len(readers))
	for _, reader := range readers {
		if reader.suspend() {
			suspended = append(suspended, reader)
		}
	}
	gate.lock.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			gate.lock.Unlock()
			for _, reader := range suspended {
				reader.resume()
			}
			r.releaseSessionCompactionGate(key, gate)
		})
	}
}

func (r *Resolver) sessionCompactionReaders(gate *sessionCompactionGate, streamID string) []*sessionCompactionReader {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil
	}
	r.sessionCompactionMu.Lock()
	defer r.sessionCompactionMu.Unlock()
	readers := make([]*sessionCompactionReader, 0, len(gate.readers[streamID]))
	for reader := range gate.readers[streamID] {
		readers = append(readers, reader)
	}
	return readers
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
