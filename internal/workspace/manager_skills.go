package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/agent/tools"
	bridgepb "github.com/memohai/memoh/internal/workspace/bridge/pb"
)

const (
	// BuiltinSkillsPath is the path to built-in skills in the host filesystem
	BuiltinSkillsPath = "./skills/public"
	// ContainerSkillsPath is the path where skills are mounted in the container
	ContainerSkillsPath = "/data/.skills"
)

// InitBuiltinSkills initializes built-in skills for a bot container.
// It copies skills from the host's BuiltinSkillsPath to the container's /data/.skills/public/
// and creates the extensions_config.json file.
func (m *Manager) InitBuiltinSkills(ctx context.Context, botID string) error {
	if m.logger != nil {
		m.logger.Info("initializing built-in skills", slog.String("bot_id", botID))
	}

	// Get gRPC client for the container
	client, err := m.MCPClient(ctx, botID)
	if err != nil {
		return fmt.Errorf("failed to get MCP client: %w", err)
	}

	// Create .skills directory structure in container
	skillsDir := ContainerSkillsPath
	publicDir := filepath.Join(skillsDir, "public")
	customDir := filepath.Join(skillsDir, "custom")

	if err := client.Mkdir(ctx, skillsDir); err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to create skills dir (may already exist)", slog.String("path", skillsDir), slog.Any("error", err))
		}
	}
	if err := client.Mkdir(ctx, publicDir); err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to create public skills dir (may already exist)", slog.String("path", publicDir), slog.Any("error", err))
		}
	}
	if err := client.Mkdir(ctx, customDir); err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to create custom skills dir (may already exist)", slog.String("path", customDir), slog.Any("error", err))
		}
	}

	// Check if built-in skills directory exists on host
	if _, err := os.Stat(BuiltinSkillsPath); os.IsNotExist(err) {
		if m.logger != nil {
			m.logger.Warn("built-in skills directory not found, skipping initialization", slog.String("path", BuiltinSkillsPath))
		}
		return nil
	}

	// Read built-in skills from host
	entries, err := os.ReadDir(BuiltinSkillsPath)
	if err != nil {
		return fmt.Errorf("failed to read built-in skills directory: %w", err)
	}

	// Track which skills were copied
	copiedSkills := make([]string, 0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillSourceDir := filepath.Join(BuiltinSkillsPath, skillName)
		skillTargetDir := filepath.Join(publicDir, skillName)

		// Check if skill already exists in container
		_, statErr := client.Stat(ctx, skillTargetDir)
		if statErr == nil {
			if m.logger != nil {
				m.logger.Debug("skill already exists in container, skipping", slog.String("skill", skillName))
			}
			continue
		}

		// Create skill directory in container
		if err := client.Mkdir(ctx, skillTargetDir); err != nil {
			if m.logger != nil {
				m.logger.Error("failed to create skill directory", slog.String("skill", skillName), slog.Any("error", err))
			}
			continue
		}

		// Copy skill files recursively
		if err := m.copySkillToContainer(ctx, client, skillSourceDir, skillTargetDir); err != nil {
			if m.logger != nil {
				m.logger.Error("failed to copy skill files", slog.String("skill", skillName), slog.Any("error", err))
			}
			continue
		}

		copiedSkills = append(copiedSkills, skillName)
		if m.logger != nil {
			m.logger.Debug("copied skill to container", slog.String("skill", skillName))
		}
	}

	// Create or update extensions_config.json
	if err := m.initExtensionsConfig(ctx, client, skillsDir, copiedSkills); err != nil {
		if m.logger != nil {
			m.logger.Error("failed to initialize extensions config", slog.Any("error", err))
		}
		// Non-fatal: skills are copied even if config fails
	}

	if m.logger != nil {
		m.logger.Info("built-in skills initialization completed",
			slog.String("bot_id", botID),
			slog.Int("skills_copied", len(copiedSkills)),
			slog.Any("skills", copiedSkills),
		)
	}

	return nil
}

// copySkillToContainer recursively copies a skill directory to the container
func (m *Manager) copySkillToContainer(ctx context.Context, client interface {
	WriteFile(ctx context.Context, path string, content []byte) error
	Mkdir(ctx context.Context, path string) error
}, sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, relPath)
		// Normalize path separators for container (Unix-style)
		targetPath = strings.ReplaceAll(targetPath, string(filepath.Separator), "/")

		if d.IsDir() {
			if err := client.Mkdir(ctx, targetPath); err != nil {
				// Directory might already exist, continue
				if m.logger != nil {
					m.logger.Debug("mkdir failed (may exist)", slog.String("path", targetPath))
				}
			}
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Write file to container
		if err := client.WriteFile(ctx, targetPath, content); err != nil {
			return fmt.Errorf("failed to write file to container %s: %w", targetPath, err)
		}

		return nil
	})
}

// initExtensionsConfig creates the extensions_config.json file in the container
func (m *Manager) initExtensionsConfig(ctx context.Context, client interface {
	WriteFile(ctx context.Context, path string, content []byte) error
	ReadFile(ctx context.Context, path string, lineOffset, nLines int32) (*bridgepb.ReadFileResponse, error)
}, skillsDir string, copiedSkills []string) error {
	configPath := filepath.Join(skillsDir, tools.DefaultExtensionsConfigFile)
	configPath = strings.ReplaceAll(configPath, string(filepath.Separator), "/")

	// Try to read existing config
	var config *tools.ExtensionsConfig
	existingConfig, err := client.ReadFile(ctx, configPath, 0, 0)
	if err != nil {
		// Config doesn't exist, create new
		config = tools.NewExtensionsConfig()
	} else {
		// Parse existing config
		config, err = tools.LoadExtensionsConfigFromBytes([]byte(existingConfig.Content))
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("failed to parse existing config, creating new", slog.Any("error", err))
			}
			config = tools.NewExtensionsConfig()
		}
	}

	// Add copied skills to config
	now := time.Now().UTC()
	for _, skillName := range copiedSkills {
		config.SetSkillState(skillName, tools.SkillState{
			Enabled:     true,
			AutoLoad:    true,
			Category:    tools.DefaultPublicCategory,
			InstalledAt: now,
			UpdatedAt:   now,
			Source:      "builtin",
			Version:     "1.0.0",
		})
	}

	// Serialize config
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	// Write config to container
	if err := client.WriteFile(ctx, configPath, configData); err != nil {
		return fmt.Errorf("failed to write config to container: %w", err)
	}

	return nil
}
