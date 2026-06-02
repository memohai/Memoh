package workspace

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/*
var bridgeTemplates embed.FS

const (
	agentsFileName         = "AGENTS.md"
	legacyIdentityFileName = "IDENTITY.md"
)

func seedBridgeTemplates(dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return err
	}
	if err := migrateLegacyIdentityFile(dstDir); err != nil {
		return err
	}
	entries, err := fs.ReadDir(bridgeTemplates, "templates")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		dst := filepath.Join(dstDir, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := bridgeTemplates.ReadFile("templates/" + entry.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyIdentityFile(dstDir string) error {
	agentsPath := filepath.Join(dstDir, agentsFileName)
	if _, err := os.Stat(agentsPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	identityPath := filepath.Join(dstDir, legacyIdentityFileName)
	info, err := os.Stat(identityPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	return os.Rename(identityPath, agentsPath)
}
