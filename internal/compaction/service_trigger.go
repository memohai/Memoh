package compaction

import (
	"context"
	"log/slog"
)

type compactionTriggerRequest struct {
	ctx context.Context
	cfg TriggerConfig
}

// TriggerCompaction queues automatic compaction work by session. One worker
// owns each session and coalesces overlapping demand to the latest config.
func (s *Service) TriggerCompaction(ctx context.Context, cfg TriggerConfig) {
	request := compactionTriggerRequest{ctx: context.WithoutCancel(ctx), cfg: cfg}
	s.inflightMu.Lock()
	if s.asyncRunning == nil {
		s.asyncRunning = make(map[string]struct{})
	}
	if s.asyncPending == nil {
		s.asyncPending = make(map[string]compactionTriggerRequest)
	}
	if _, running := s.asyncRunning[cfg.SessionID]; running {
		s.asyncPending[cfg.SessionID] = request
		s.inflightMu.Unlock()
		return
	}
	s.asyncRunning[cfg.SessionID] = struct{}{}
	s.inflightMu.Unlock()
	go s.runCompactionTriggerWorker(request)
}

func (s *Service) runCompactionTriggerWorker(request compactionTriggerRequest) {
	for {
		_, owner, err := s.runCompaction(request.ctx, request.cfg)
		if err != nil {
			s.logger.Error(
				"compaction failed",
				slog.String("bot_id", request.cfg.BotID),
				slog.String("session_id", request.cfg.SessionID),
				slog.String("error", err.Error()),
			)
		}
		if owner != nil {
			<-owner.done
			request = s.takeLatestCompactionTrigger(request)
			continue
		}
		next, ok := s.nextCompactionTrigger(request.cfg.SessionID)
		if !ok {
			return
		}
		request = next
	}
}

func (s *Service) takeLatestCompactionTrigger(current compactionTriggerRequest) compactionTriggerRequest {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	next, ok := s.asyncPending[current.cfg.SessionID]
	if !ok {
		return current
	}
	delete(s.asyncPending, current.cfg.SessionID)
	return next
}

func (s *Service) nextCompactionTrigger(sessionID string) (compactionTriggerRequest, bool) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	next, ok := s.asyncPending[sessionID]
	if ok {
		delete(s.asyncPending, sessionID)
		return next, true
	}
	delete(s.asyncRunning, sessionID)
	return compactionTriggerRequest{}, false
}
