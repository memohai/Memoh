package channel

import "context"

// DirectoryEntryKind classifies a directory entry as a user or a group.
type DirectoryEntryKind string

// DirectoryEntryKind values for listing and resolving directory entries.
const (
	DirectoryEntryUser  DirectoryEntryKind = "user"
	DirectoryEntryGroup DirectoryEntryKind = "group"
)

// DirectoryEntry represents a single user or group discovered through the channel's directory.
type DirectoryEntry struct {
	Kind      DirectoryEntryKind `json:"kind"`
	ID        string             `json:"id"`
	Name      string             `json:"name,omitempty"`
	Handle    string             `json:"handle,omitempty"`
	AvatarURL string             `json:"avatar_url,omitempty"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
}

// DirectoryQuery contains filters for directory listing operations.
type DirectoryQuery struct {
	Query string             `json:"query,omitempty"`
	Limit int                `json:"limit,omitempty"`
	Kind  DirectoryEntryKind `json:"kind,omitempty"`
}

// DirectoryAdapter provides contact and group lookup for a channel platform.
type DirectoryAdapter interface {
	ListPeers(ctx context.Context, cfg Config, query DirectoryQuery) ([]DirectoryEntry, error)
	ListGroups(ctx context.Context, cfg Config, query DirectoryQuery) ([]DirectoryEntry, error)
	ListGroupMembers(ctx context.Context, cfg Config, groupID string, query DirectoryQuery) ([]DirectoryEntry, error)
	ResolveEntry(ctx context.Context, cfg Config, input string, kind DirectoryEntryKind) (DirectoryEntry, error)
}
