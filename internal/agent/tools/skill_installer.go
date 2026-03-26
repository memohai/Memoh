package tools

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// MaxSkillArchiveSize is the maximum allowed size for a .skill file (512MB)
	MaxSkillArchiveSize = 512 * 1024 * 1024
	// MaxSkillExtractSize is the maximum total extracted size (512MB)
	MaxSkillExtractSize = 512 * 1024 * 1024
	// SkillFileName is the main skill filename
	SkillFileName = "SKILL.md"
	// SkillFileExtension is the skill archive extension
	SkillFileExtension = ".skill"
)

// SkillInstaller handles installation of .skill packages.
type SkillInstaller struct {
	publicDir  string
	customDir  string
	configPath string
}

// NewSkillInstaller creates a new skill installer.
func NewSkillInstaller(skillsRoot, configPath string) *SkillInstaller {
	return &SkillInstaller{
		publicDir:  filepath.Join(skillsRoot, DefaultPublicCategory),
		customDir:  filepath.Join(skillsRoot, DefaultCustomCategory),
		configPath: configPath,
	}
}

// InstallResult contains the result of a skill installation.
type InstallResult struct {
	Success   bool      `json:"success"`
	SkillName string    `json:"skill_name"`
	Message   string    `json:"message"`
	InstalledAt time.Time `json:"installed_at"`
}

// InstallFromArchive installs a skill from a .skill ZIP archive.
func (si *SkillInstaller) InstallFromArchive(archivePath string) (*InstallResult, error) {
	// Validate file extension
	if filepath.Ext(archivePath) != SkillFileExtension {
		return nil, fmt.Errorf("file must have %s extension", SkillFileExtension)
	}

	// Open archive
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open skill archive: %w", err)
	}
	defer zr.Close()

	// Validate archive size
	var totalSize int64
	for _, f := range zr.File {
		totalSize += int64(f.UncompressedSize64)
		if totalSize > MaxSkillExtractSize {
			return nil, fmt.Errorf("skill archive exceeds maximum extract size of %d bytes", MaxSkillExtractSize)
		}
	}

	// Create temp directory for extraction
	tempDir, err := os.MkdirTemp("", "skill-install-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract archive safely
	skillDir, err := si.extractArchive(zr, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract skill archive: %w", err)
	}

	// Validate skill
	skill, err := si.validateSkill(skillDir)
	if err != nil {
		return nil, fmt.Errorf("skill validation failed: %w", err)
	}

	// Check if skill already exists
	targetDir := filepath.Join(si.customDir, skill.Name)
	if _, err := os.Stat(targetDir); err == nil {
		return nil, fmt.Errorf("skill '%s' already exists", skill.Name)
	}

	// Create custom directory if needed
	if err := os.MkdirAll(si.customDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create custom directory: %w", err)
	}

	// Move skill to target location
	if err := os.Rename(skillDir, targetDir); err != nil {
		return nil, fmt.Errorf("failed to install skill: %w", err)
	}

	// Update extensions config
	if err := si.updateConfig(skill); err != nil {
		// Attempt to rollback
		_ = os.RemoveAll(targetDir)
		return nil, fmt.Errorf("failed to update config: %w", err)
	}

	return &InstallResult{
		Success:     true,
		SkillName:   skill.Name,
		Message:     fmt.Sprintf("Skill '%s' installed successfully", skill.Name),
		InstalledAt: time.Now().UTC(),
	}, nil
}

// InstallFromDirectory installs a skill from a local directory.
func (si *SkillInstaller) InstallFromDirectory(sourceDir string) (*InstallResult, error) {
	// Validate skill
	skill, err := si.validateSkill(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("skill validation failed: %w", err)
	}

	// Check if skill already exists
	targetDir := filepath.Join(si.customDir, skill.Name)
	if _, err := os.Stat(targetDir); err == nil {
		return nil, fmt.Errorf("skill '%s' already exists", skill.Name)
	}

	// Create custom directory if needed
	if err := os.MkdirAll(si.customDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create custom directory: %w", err)
	}

	// Copy skill directory
	if err := copyDir(sourceDir, targetDir); err != nil {
		return nil, fmt.Errorf("failed to copy skill: %w", err)
	}

	// Update extensions config
	if err := si.updateConfig(skill); err != nil {
		// Attempt to rollback
		_ = os.RemoveAll(targetDir)
		return nil, fmt.Errorf("failed to update config: %w", err)
	}

	return &InstallResult{
		Success:     true,
		SkillName:   skill.Name,
		Message:     fmt.Sprintf("Skill '%s' installed successfully", skill.Name),
		InstalledAt: time.Now().UTC(),
	}, nil
}

// ExportSkill exports a skill to a .skill archive.
func (si *SkillInstaller) ExportSkill(skillName string, category string, outputPath string) error {
	// Determine source directory
	var sourceDir string
	switch category {
	case DefaultPublicCategory:
		sourceDir = filepath.Join(si.publicDir, skillName)
	case DefaultCustomCategory:
		sourceDir = filepath.Join(si.customDir, skillName)
	default:
		// Try both directories
		publicPath := filepath.Join(si.publicDir, skillName)
		customPath := filepath.Join(si.customDir, skillName)
		if _, err := os.Stat(publicPath); err == nil {
			sourceDir = publicPath
		} else if _, err := os.Stat(customPath); err == nil {
			sourceDir = customPath
		} else {
			return fmt.Errorf("skill '%s' not found", skillName)
		}
	}

	// Verify source exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("skill '%s' not found in %s", skillName, category)
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create zip writer
	zw := zip.NewWriter(outFile)
	defer zw.Close()

	// Walk source directory and add files to archive
	err = filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		// Create zip header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		// Create file in archive
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		// Open source file
		srcFile, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Copy content
		_, err = io.Copy(w, srcFile)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to create skill archive: %w", err)
	}

	return nil
}

// UninstallSkill removes an installed skill.
func (si *SkillInstaller) UninstallSkill(skillName string) error {
	// Try custom directory first (user-installed skills)
	skillDir := filepath.Join(si.customDir, skillName)
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		// Try public directory
		skillDir = filepath.Join(si.publicDir, skillName)
		if _, err := os.Stat(skillDir); os.IsNotExist(err) {
			return fmt.Errorf("skill '%s' not found", skillName)
		}
	}

	// Remove skill directory
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("failed to remove skill: %w", err)
	}

	// Remove from config
	config, err := LoadExtensionsConfig(si.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	config.RemoveSkill(skillName)

	if err := config.Save(); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	return nil
}

// extractArchive safely extracts a zip archive to the destination directory.
func (si *SkillInstaller) extractArchive(zr *zip.ReadCloser, destDir string) (string, error) {
	var skillRoot string

	for _, f := range zr.File {
		// Security checks
		if err := si.validateZipEntry(f); err != nil {
			return "", err
		}

		// Determine extraction path
		filePath := filepath.Join(destDir, f.Name)

		// Create directory if needed
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, f.Mode()); err != nil {
				return "", err
			}
			continue
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return "", err
		}

		// Extract file
		if err := si.extractFile(f, filePath); err != nil {
			return "", err
		}

		// Track skill root directory
		if skillRoot == "" {
			skillRoot = filepath.Dir(filePath)
		}
	}

	// If archive contains a single top-level directory, return that
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(destDir, entries[0].Name()), nil
	}

	return destDir, nil
}

// validateZipEntry checks if a zip entry is safe to extract.
func (si *SkillInstaller) validateZipEntry(f *zip.File) error {
	// Reject absolute paths
	if filepath.IsAbs(f.Name) {
		return fmt.Errorf("archive contains absolute path: %s", f.Name)
	}

	// Reject directory traversal
	if strings.Contains(f.Name, "..") {
		return fmt.Errorf("archive contains directory traversal: %s", f.Name)
	}

	// Reject symlinks (by checking mode)
	if f.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive contains symlink: %s", f.Name)
	}

	return nil
}

// extractFile extracts a single file from the zip archive.
func (si *SkillInstaller) extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

// validateSkill validates and parses a skill from a directory.
func (si *SkillInstaller) validateSkill(skillDir string) (*SkillV2, error) {
	skillPath := filepath.Join(skillDir, SkillFileName)

	// Check if SKILL.md exists
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SKILL.md not found in skill directory")
	}

	// Read skill file
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}

	// Parse skill
	skill, err := ParseSkillV2(string(content), "", DefaultCustomCategory)
	if err != nil {
		return nil, fmt.Errorf("failed to parse skill: %w", err)
	}

	// Validate skill
	validationErrors := skill.Validate()
	if len(validationErrors) > 0 {
		var errs []string
		for _, e := range validationErrors {
			errs = append(errs, e.Error())
		}
		return nil, fmt.Errorf("validation errors: %s", strings.Join(errs, "; "))
	}

	return skill, nil
}

// updateConfig updates the extensions config with the new skill.
func (si *SkillInstaller) updateConfig(skill *SkillV2) error {
	config, err := LoadExtensionsConfig(si.configPath)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	config.SetSkillState(skill.Name, SkillState{
		Enabled:     true,
		AutoLoad:    false,
		Category:    DefaultCustomCategory,
		InstalledAt: now,
		UpdatedAt:   now,
		Source:      "import",
		Version:     skill.Version,
	})

	return config.Save()
}

// Helper functions

// copyDir copies a directory recursively.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// PackSkill creates a .skill archive from a skill directory.
func PackSkill(sourceDir, outputPath string) error {
	// Validate source is a valid skill
	skillPath := filepath.Join(sourceDir, SkillFileName)
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not contain SKILL.md")
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create zip writer
	zw := zip.NewWriter(outFile)
	defer zw.Close()

	// Get skill name from directory
	skillName := filepath.Base(sourceDir)

	// Walk source directory
	err = filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Calculate relative path with skill name as root
		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}
		relPath = filepath.Join(skillName, relPath)

		// Create zip header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		// Create file in archive
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		// Open source file
		srcFile, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Copy content
		_, err = io.Copy(w, srcFile)
		return err
	})

	return err
}
