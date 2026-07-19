package inbound

import "sync"

type pipelineSessionLock struct {
	mu   sync.Mutex
	refs int
}

type pipelineSessionLockSet struct {
	mu    sync.Mutex
	locks map[string]*pipelineSessionLock
}

func (p *ChannelInboundProcessor) lockPipelineSession(sessionID string) func() {
	return p.pipelineSessionLocks.lock(sessionID)
}

func (s *pipelineSessionLockSet) lock(sessionID string) func() {
	s.mu.Lock()
	if s.locks == nil {
		s.locks = make(map[string]*pipelineSessionLock)
	}
	entry := s.locks[sessionID]
	if entry == nil {
		entry = &pipelineSessionLock{}
		s.locks[sessionID] = entry
	}
	entry.refs++
	s.mu.Unlock()

	entry.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			s.mu.Lock()
			entry.refs--
			if entry.refs == 0 {
				delete(s.locks, sessionID)
			}
			s.mu.Unlock()
		})
	}
}

func (s *pipelineSessionLockSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.locks)
}
