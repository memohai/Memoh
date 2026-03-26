// Package tools provides enhanced skill parsing and validation for DeerFlow-aligned skill system.
package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillV2 represents an enhanced skill with DeerFlow-compatible metadata.
type SkillV2 struct {
	// Core fields (from frontmatter)
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`

	// Extended metadata (DeerFlow aligned)
	Version       string   `json:"version,omitempty" yaml:"version,omitempty"`
	Author        string   `json:"author,omitempty" yaml:"author,omitempty"`
	License       string   `json:"license,omitempty" yaml:"license,omitempty"`
	AllowedTools  []string `json:"allowed-tools,omitempty" yaml:"allowed-tools,omitempty"`
	Compatibility string   `json:"compatibility,omitempty" yaml:"compatibility,omitempty"`
	Category      string   `json:"category,omitempty" yaml:"category,omitempty"`

	// Content (body after frontmatter)
	Content string `json:"content" yaml:"-"`
	Raw     string `json:"raw" yaml:"-"`

	// Runtime state
	Enabled      bool      `json:"enabled" yaml:"-"`
	AutoLoad     bool      `json:"auto_load" yaml:"-"`
	InstalledAt  time.Time `json:"installed_at" yaml:"-"`
	UpdatedAt    time.Time `json:"updated_at" yaml:"-"`
	SkillDir     string    `json:"-" yaml:"-"`
	CategoryDir  string    `json:"category_dir,omitempty" yaml:"-"` // "public" or "custom"
}

// ParsedSkill represents the raw parsed result from SKILL.md file.
type ParsedSkill struct {
	Frontmatter map[string]any `json:"frontmatter"`
	Content     string         `json:"content"`
	Raw         string         `json:"raw"`
}

// SkillValidationError represents a validation error with field context.
type SkillValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e SkillValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validation patterns (DeerFlow aligned)
var (
	// hyphen-case: lowercase letters, digits, and hyphens only
	validSkillNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	// Semantic versioning
	validVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
)

// Constants
const (
	MaxSkillNameLength        = 64
	MaxSkillDescriptionLength = 1024
	MaxSkillContentLength     = 10 * 1024 * 1024 // 10MB
)

// ParseSkillV2 parses a SKILL.md file with extended DeerFlow-compatible frontmatter.
func ParseSkillV2(raw string, fallbackName string, category string) (*SkillV2, error) {
	trimmed := strings.TrimSpace(raw)

	skill := &SkillV2{
		Name:        strings.TrimSpace(fallbackName),
		Raw:         raw,
		CategoryDir: category,
		Enabled:     true,
		AutoLoad:    false,
	}

	// No frontmatter case
	if !strings.HasPrefix(trimmed, "---") {
		skill.Content = trimmed
		if skill.Name == "" {
			skill.Name = "default"
		}
		return skill, nil
	}

	// Parse frontmatter
	parsed, err := parseFrontmatter(trimmed)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	skill.Content = parsed.Content
	skill.Raw = raw

	// Extract frontmatter fields
	if err := extractFrontmatter(skill, parsed.Frontmatter); err != nil {
		return nil, err
	}

	// Normalize
	if skill.Name == "" {
		skill.Name = fallbackName
	}
	if skill.Name == "" {
		skill.Name = "default"
	}

	return skill, nil
}

// parseFrontmatter extracts YAML frontmatter from markdown content.
func parseFrontmatter(content string) (*ParsedSkill, error) {
	// Find frontmatter boundaries
	rest := content[3:] // Skip initial "---"
	rest = strings.TrimLeft(rest, " \t")

	// Handle newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	// Find closing ---
	closingIdx := strings.Index(rest, "\n---")
	if closingIdx < 0 {
		return nil, errors.New("no closing frontmatter delimiter found")
	}

	frontmatterRaw := rest[:closingIdx]
	body := rest[closingIdx+4:]
	body = strings.TrimLeft(body, "\r\n")

	// Parse YAML frontmatter
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &frontmatter); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return &ParsedSkill{
		Frontmatter: frontmatter,
		Content:     strings.TrimSpace(body),
		Raw:         content,
	}, nil
}

// extractFrontmatter extracts fields from parsed frontmatter into SkillV2.
func extractFrontmatter(skill *SkillV2, fm map[string]any) error {
	// Required fields
	if name, ok := fm["name"].(string); ok {
		skill.Name = strings.TrimSpace(name)
	}
	if desc, ok := fm["description"].(string); ok {
		skill.Description = strings.TrimSpace(desc)
	}

	// Optional metadata fields
	if version, ok := fm["version"].(string); ok {
		skill.Version = strings.TrimSpace(version)
	}
	if author, ok := fm["author"].(string); ok {
		skill.Author = strings.TrimSpace(author)
	}
	if license, ok := fm["license"].(string); ok {
		skill.License = strings.TrimSpace(license)
	}
	if compat, ok := fm["compatibility"].(string); ok {
		skill.Compatibility = strings.TrimSpace(compat)
	}
	if cat, ok := fm["category"].(string); ok {
		skill.Category = strings.TrimSpace(cat)
	}

	// allowed-tools: can be string or array
	switch v := fm["allowed-tools"].(type) {
	case []any:
		skill.AllowedTools = make([]string, 0, len(v))
		for _, tool := range v {
			if toolStr, ok := tool.(string); ok {
				skill.AllowedTools = append(skill.AllowedTools, toolStr)
			}
		}
	case string:
		// Single tool as string
		if v != "" {
			skill.AllowedTools = []string{v}
		}
	}

	return nil
}

// Validate validates the skill according to DeerFlow rules.
func (s *SkillV2) Validate() []SkillValidationError {
	var errors []SkillValidationError

	// Validate name (required)
	if s.Name == "" {
		errors = append(errors, SkillValidationError{
			Field:   "name",
			Message: "name is required",
		})
	} else {
		if errs := validateSkillName(s.Name); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}

	// Validate description (required)
	if s.Description == "" {
		errors = append(errors, SkillValidationError{
			Field:   "description",
			Message: "description is required",
		})
	} else {
		if errs := validateSkillDescription(s.Description); len(errs) > 0 {
			errors = append(errors, errs...)
		}
	}

	// Validate version (optional)
	if s.Version != "" {
		if !validVersionPattern.MatchString(s.Version) {
			errors = append(errors, SkillValidationError{
				Field:   "version",
				Message: fmt.Sprintf("invalid semantic version: %s", s.Version),
			})
		}
	}

	// Validate content size
	if len(s.Raw) > MaxSkillContentLength {
		errors = append(errors, SkillValidationError{
			Field:   "content",
			Message: fmt.Sprintf("content exceeds maximum size of %d bytes", MaxSkillContentLength),
		})
	}

	return errors
}

// IsValid returns true if the skill has no validation errors.
func (s *SkillV2) IsValid() bool {
	return len(s.Validate()) == 0
}

// validateSkillName validates skill name according to DeerFlow rules:
// - hyphen-case: lowercase letters, digits, and hyphens only
// - cannot start/end with hyphen
// - cannot contain consecutive hyphens
// - max 64 characters
func validateSkillName(name string) []SkillValidationError {
	var errors []SkillValidationError

	if len(name) > MaxSkillNameLength {
		errors = append(errors, SkillValidationError{
			Field:   "name",
			Message: fmt.Sprintf("name is too long (%d characters), maximum is %d", len(name), MaxSkillNameLength),
		})
	}

	if !validSkillNamePattern.MatchString(name) {
		errors = append(errors, SkillValidationError{
			Field:   "name",
			Message: "name must be hyphen-case (lowercase letters, digits, and hyphens only)",
		})
	}

	if strings.HasPrefix(name, "-") {
		errors = append(errors, SkillValidationError{
			Field:   "name",
			Message: "name cannot start with a hyphen",
		})
	}

	if strings.HasSuffix(name, "-") {
		errors = append(errors, SkillValidationError{
			Field:   "name",
			Message: "name cannot end with a hyphen",
		})
	}

	if strings.Contains(name, "--") {
		errors = append(errors, SkillValidationError{
			Field:   "name",
			Message: "name cannot contain consecutive hyphens",
		})
	}

	return errors
}

// validateSkillDescription validates skill description:
// - max 1024 characters
// - cannot contain angle brackets
func validateSkillDescription(desc string) []SkillValidationError {
	var errors []SkillValidationError

	if len(desc) > MaxSkillDescriptionLength {
		errors = append(errors, SkillValidationError{
			Field:   "description",
			Message: fmt.Sprintf("description is too long (%d characters), maximum is %d", len(desc), MaxSkillDescriptionLength),
		})
	}

	if strings.Contains(desc, "<") || strings.Contains(desc, ">") {
		errors = append(errors, SkillValidationError{
			Field:   "description",
			Message: "description cannot contain angle brackets (< or >)",
		})
	}

	return errors
}

// ToSystemPrompt returns the skill content formatted for system prompt injection.
func (s *SkillV2) ToSystemPrompt() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Skill: %s\n\n", s.Name))

	if s.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n\n", s.Description))
	}

	if s.Version != "" {
		b.WriteString(fmt.Sprintf("Version: %s\n", s.Version))
	}

	if len(s.AllowedTools) > 0 {
		b.WriteString(fmt.Sprintf("Allowed Tools: %s\n", strings.Join(s.AllowedTools, ", ")))
	}

	b.WriteString("\n---\n\n")
	b.WriteString(s.Content)

	return b.String()
}

// IsToolAllowed checks if a tool is in the skill's allowed-tools list.
// If allowed-tools is empty, all tools are permitted.
func (s *SkillV2) IsToolAllowed(toolName string) bool {
	if len(s.AllowedTools) == 0 {
		return true
	}

	for _, allowed := range s.AllowedTools {
		if strings.EqualFold(allowed, toolName) {
			return true
		}
	}

	return false
}

// SkillStore defines the interface for skill storage operations.
type SkillStore interface {
	// LoadSkills loads all skills from storage
	LoadSkills(ctx context.Context, category string) ([]*SkillV2, error)

	// LoadSkill loads a specific skill by name
	LoadSkill(ctx context.Context, name string, category string) (*SkillV2, error)

	// SaveSkill saves a skill to storage
	SaveSkill(ctx context.Context, skill *SkillV2) error

	// DeleteSkill removes a skill from storage
	DeleteSkill(ctx context.Context, name string, category string) error

	// SkillExists checks if a skill exists
	SkillExists(ctx context.Context, name string, category string) (bool, error)
}
