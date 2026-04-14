package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	dbembed "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/version"
)

func provideConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func migrationsFS() fs.FS {
	sub, err := fs.Sub(dbembed.MigrationsFS, "migrations")
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err))
	}
	return sub
}

func runMigrate(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: migrate <up|down|version|force N>")
	}

	cfg, err := provideConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
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
		return err
	}
	return nil
}

func runVersion() error {
	fmt.Printf("memoh %s\n", version.GetInfo())
	return nil
}
