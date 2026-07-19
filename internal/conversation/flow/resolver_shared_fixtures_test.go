package flow

import (
	"context"

	messagepkg "github.com/memohai/memoh/internal/message"
)

// nthPersistFailureMessageService fails Persist on its failOnCall-th
// invocation (1-based) with err, then falls back to recordingMessageService.
// Shared by the ACP persistence and retry/edit atomic persistence suites,
// which each fail a different call in the same durable-projection sequence.
type nthPersistFailureMessageService struct {
	recordingMessageService
	failOnCall   int
	err          error
	persistCalls int
}

func (s *nthPersistFailureMessageService) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persistCalls++
	if s.persistCalls == s.failOnCall {
		return messagepkg.Message{}, s.err
	}
	return s.recordingMessageService.Persist(ctx, input)
}

func (s *nthPersistFailureMessageService) PersistTurnResponseTail(_ context.Context, inputs []messagepkg.PersistInput) ([]messagepkg.Message, error) {
	s.persisted = append(s.persisted, inputs...)
	return recordedMessages(inputs), nil
}
