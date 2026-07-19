package flow

import (
	"context"
	"sync"
)

type scheduledCompaction struct {
	ctx                context.Context
	botID              string
	sessionID          string
	userID             string
	inputTokens        int
	contextTokenBudget int
}

type compactionScheduleState struct {
	pending *scheduledCompaction
}

type compactionScheduler struct {
	mu     sync.Mutex
	active map[string]*compactionScheduleState
	run    func(scheduledCompaction) bool
}

func newCompactionScheduler(run func(scheduledCompaction) bool) *compactionScheduler {
	return &compactionScheduler{
		active: make(map[string]*compactionScheduleState),
		run:    run,
	}
}

func (s *compactionScheduler) Schedule(key string, request scheduledCompaction) {
	s.mu.Lock()
	state := s.active[key]
	if state != nil {
		state.pending = &request
		s.mu.Unlock()
		return
	}
	state = &compactionScheduleState{}
	s.active[key] = state
	s.mu.Unlock()

	go s.runLoop(key, state, request)
}

func (s *compactionScheduler) runLoop(key string, state *compactionScheduleState, request scheduledCompaction) {
	for {
		compacted := s.run(request)

		s.mu.Lock()
		if compacted || state.pending == nil {
			delete(s.active, key)
			s.mu.Unlock()
			return
		}
		request = *state.pending
		state.pending = nil
		s.mu.Unlock()
	}
}

func (s *compactionScheduler) Active() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active)
}
