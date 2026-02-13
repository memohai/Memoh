package flow

import (
	"context"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/schedule"
)

// Runner defines conversation execution behavior for sync, stream, and scheduled flows.
type Runner interface {
	Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error)
	StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
	TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) error
}
