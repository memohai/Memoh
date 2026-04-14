package tui

import (
	"fmt"
	"io/fs"
	"os"

	dbembed "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/config"
)

func ProvideConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func MigrationsFS() fs.FS {
	sub, err := fs.Sub(dbembed.MigrationsFS, "migrations")
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err))
	}
	return sub
}
