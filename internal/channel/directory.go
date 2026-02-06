package channel

import "context"

type DirectoryEntryKind string

const (
	DirectoryEntryUser  DirectoryEntryKind = "user"
	DirectoryEntryGroup DirectoryEntryKind = "group"
)

type DirectoryEntry struct {
	Kind      DirectoryEntryKind `json:"kind"`
	ID        string             `json:"id"`
	Name      string             `json:"name,omitempty"`
	Handle    string             `json:"handle,omitempty"`
	AvatarURL string             `json:"avatar_url,omitempty"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
}

type DirectoryQuery struct {
	Query string             `json:"query,omitempty"`
	Limit int                `json:"limit,omitempty"`
	Kind  DirectoryEntryKind `json:"kind,omitempty"`
}

type ChannelDirectoryAdapter interface {
	ListPeers(ctx context.Context, cfg ChannelConfig, query DirectoryQuery) ([]DirectoryEntry, error)
	ListGroups(ctx context.Context, cfg ChannelConfig, query DirectoryQuery) ([]DirectoryEntry, error)
	ListGroupMembers(ctx context.Context, cfg ChannelConfig, groupID string, query DirectoryQuery) ([]DirectoryEntry, error)
	ResolveTarget(ctx context.Context, cfg ChannelConfig, input string, kind DirectoryEntryKind) (DirectoryEntry, error)
}
