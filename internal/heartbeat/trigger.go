package heartbeat

import "context"

type TriggerPayload struct {
	BotID       string
	Interval    int
	OwnerUserID string
}

type TriggerResult struct {
	Status     string
	Text       string
	Usage      any
	UsageBytes []byte
}

type Triggerer interface {
	TriggerHeartbeat(ctx context.Context, botID string, payload TriggerPayload, token string) (TriggerResult, error)
}
