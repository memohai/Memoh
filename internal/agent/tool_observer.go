package agent

import (
	"context"
	"time"
)

type ToolCallObservation struct {
	ToolCallID string
	ToolName   string
	Input      any
	Result     any
	Err        error
	StartedAt  time.Time
	FinishedAt time.Time
}

type ToolCallObserver interface {
	OnToolCallStart(ctx context.Context, observation ToolCallObservation) error
	OnToolCallFinish(ctx context.Context, observation ToolCallObservation) error
}
