package command

import "context"

// Skill represents a single skill loaded from a bot's container.
type Skill struct {
	Name        string
	Description string
}

// SkillLoader loads skills for a bot.
type SkillLoader interface {
	LoadSkills(ctx context.Context, botID string) ([]Skill, error)
}

// FSEntry represents a file or directory in a container filesystem.
type FSEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// ContainerFS provides read-only access to a bot's container filesystem.
type ContainerFS interface {
	ListDir(ctx context.Context, botID, path string) ([]FSEntry, error)
	ReadFile(ctx context.Context, botID, path string) (string, error)
}
