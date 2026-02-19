package main

import (
	"log/slog"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/memohai/memoh/cmd/agent/modules"
)

func migrationsFS() fs.FS {
	sub, err := fs.Sub(dbembed.MigrationsFS, "migrations")
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err))
	}
	return sub
}

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "migrate":
		runMigrate(os.Args[2:])
	case "version":
		fmt.Printf("memoh-server %s\n", version.GetInfo())
	default:
		fmt.Fprintf(os.Stderr, "Usage: memoh-server <command>\n\nCommands:\n  serve     Start the server (default)\n  migrate   Run database migrations (up|down|version|force)\n  version   Print version information\n")
		os.Exit(1)
	}
}

func runMigrate(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: memoh-server migrate <up|down|version|force N>\n")
		os.Exit(1)
	}

	cfg, err := provideConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format)
	log := logger.L

	migrateCmd := args[0]
	var migrateArgs []string
	if len(args) > 1 {
		migrateArgs = args[1:]
	}

	if err := db.RunMigrate(log, cfg.Postgres, migrationsFS(), migrateCmd, migrateArgs); err != nil {
		log.Error("migration failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func runServe() {
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
