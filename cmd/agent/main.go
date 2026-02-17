package main

import (
	"log/slog"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/memohai/memoh/cmd/agent/modules"
)

func main() {
	fx.New(
		modules.InfraModule,
		modules.DomainModule,
		modules.MemoryModule,
		modules.ChannelModule,
		modules.ConversationModule,
		modules.ContainerdModule,
		modules.HandlersModule,
		modules.ServerModule,

		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger.With(slog.String("component", "fx"))}
		}),
	).Run()
}
