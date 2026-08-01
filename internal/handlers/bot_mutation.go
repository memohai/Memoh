package handlers

import "context"

type botMutationCoordinator interface {
	WithBotMutation(context.Context, string, func(context.Context) error) error
}

func withBotMutation(
	ctx context.Context,
	botID string,
	coordinator botMutationCoordinator,
	fn func(context.Context) error,
) error {
	if coordinator == nil {
		return fn(ctx)
	}
	return coordinator.WithBotMutation(ctx, botID, fn)
}
