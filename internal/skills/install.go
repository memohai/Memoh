package skills

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

// InstallArchive atomically replaces a registry Skill with a validated archive.
func InstallArchive(ctx context.Context, client *bridge.Client, workspaceOS, registryID, packageID, skillID string, archive Archive) error {
	if client == nil {
		return errors.New("workspace is not reachable")
	}
	targetDir, err := RegistrySkillDirForIDs(registryID, packageID, skillID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	packageDir, err := RegistryPackageSkillsDirForIDs(registryID, packageID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	suffix, err := randomInstallSuffix()
	if err != nil {
		return err
	}
	stagingRoot := path.Join(ManagedDir(), ".staging")
	tempDir := path.Join(stagingRoot, "install-"+suffix)
	backupDir := path.Join(stagingRoot, "backup-"+suffix)
	_ = client.DeleteFile(ctx, tempDir, true)
	_ = client.DeleteFile(ctx, backupDir, true)
	defer func() { _ = client.DeleteFile(context.WithoutCancel(ctx), tempDir, true) }()
	if err := client.Mkdir(ctx, stagingRoot); err != nil {
		return fmt.Errorf("create Skill staging directory: %w", err)
	}
	if err := client.Mkdir(ctx, tempDir); err != nil {
		return fmt.Errorf("create temporary Skill directory: %w", err)
	}

	executablePaths := make([]string, 0)
	for _, file := range archive.files {
		filePath := path.Join(tempDir, file.path)
		if dir := path.Dir(filePath); dir != tempDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return fmt.Errorf("create Skill directory %q: %w", dir, err)
			}
		}
		if err := client.WriteFile(ctx, filePath, file.content); err != nil {
			return fmt.Errorf("write Skill file %q: %w", file.path, err)
		}
		if file.executable {
			executablePaths = append(executablePaths, filePath)
		}
	}
	if err := applyExecutableModes(ctx, client, workspaceOS, executablePaths); err != nil {
		return err
	}

	registryDir, err := RegistryDirForID(registryID)
	if err != nil {
		return errors.New("registry Skill identity is invalid")
	}
	if err := GuardRegistryInstall(ctx, client, registryID); err != nil {
		return err
	}
	if err := client.Mkdir(ctx, registryDir); err != nil {
		return fmt.Errorf("create registry Skill directory: %w", err)
	}
	if err := client.Mkdir(ctx, packageDir); err != nil {
		return fmt.Errorf("create registry package directory: %w", err)
	}

	targetExists := false
	if _, err := client.Stat(ctx, targetDir); err == nil {
		targetExists = true
	} else if !errors.Is(err, bridge.ErrNotFound) {
		return fmt.Errorf("inspect existing Skill: %w", err)
	}
	if targetExists {
		if err := client.Rename(ctx, targetDir, backupDir); err != nil {
			return fmt.Errorf("prepare existing Skill for replacement: %w", err)
		}
	}
	if err := client.Rename(ctx, tempDir, targetDir); err != nil {
		if targetExists {
			if rollbackErr := client.Rename(context.WithoutCancel(ctx), backupDir, targetDir); rollbackErr != nil {
				return fmt.Errorf("publish installed Skill: %w; restore previous Skill from %q: %w", err, backupDir, rollbackErr)
			}
		}
		return fmt.Errorf("publish installed Skill: %w", err)
	}
	if targetExists {
		_ = client.DeleteFile(ctx, backupDir, true)
	}
	return nil
}

func randomInstallSuffix() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create installation ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func applyExecutableModes(ctx context.Context, client *bridge.Client, workspaceOS string, paths []string) error {
	if len(paths) == 0 || strings.EqualFold(workspaceOS, "windows") || strings.EqualFold(workspaceOS, "win32") {
		return nil
	}
	for start := 0; start < len(paths); start += 100 {
		end := min(start+100, len(paths))
		quoted := make([]string, 0, end-start)
		for _, filePath := range paths[start:end] {
			quoted = append(quoted, shellQuote(filePath))
		}
		result, err := client.ExecWithEnv(ctx, "chmod 755 -- "+strings.Join(quoted, " "), "/", 30, nil)
		if err != nil {
			return fmt.Errorf("preserve executable Skill files: %w", err)
		}
		if result != nil && result.ExitCode != 0 {
			return fmt.Errorf("preserve executable Skill files: chmod exited with code %d", result.ExitCode)
		}
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `"'"'`) + "'"
}
