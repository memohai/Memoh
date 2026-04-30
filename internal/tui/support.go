package tui

import (
	"fmt"
	"io/fs"
	"os"

	dbembed "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/config"
	dbpkg "github.com/memohai/memoh/internal/db"
)

func ProvideConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func MigrationsFS(cfg config.Config) fs.FS {
	sub, err := dbpkg.MigrationsFSForConfig(cfg, dbembed.MigrationsFS)
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err))
	}
	return sub
}
