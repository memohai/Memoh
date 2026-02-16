package bots

import (
	"context"
	"time"
)

// Bot represents a bot entity.
type Bot struct {
	ID              string         `json:"id"`
	OwnerUserID     string         `json:"owner_user_id"`
	Type            string         `json:"type"`
	DisplayName     string         `json:"display_name"`
	AvatarURL       string         `json:"avatar_url,omitempty"`
	IsActive        bool           `json:"is_active"`
	AllowGuest      bool           `json:"allow_guest"`
	Status          string         `json:"status"`
	CheckState      string         `json:"check_state"`
	CheckIssueCount int32          `json:"check_issue_count"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// BotMember represents a bot membership record.
type BotMember struct {
	BotID     string    `json:"bot_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// BotCheck represents one resource check row for a bot.
type BotCheck struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	TitleKey string         `json:"title_key"`
	Subtitle string         `json:"subtitle,omitempty"`
	Status   string         `json:"status"`
	Summary  string         `json:"summary"`
	Detail   string         `json:"detail,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CreateBotRequest is the input for creating a bot.
type CreateBotRequest struct {
	Type        string         `json:"type"`
	DisplayName string         `json:"display_name,omitempty"`
	AvatarURL   string         `json:"avatar_url,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// UpdateBotRequest is the input for updating a bot.
type UpdateBotRequest struct {
	DisplayName *string        `json:"display_name,omitempty"`
	AvatarURL   *string        `json:"avatar_url,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TransferBotRequest is the input for transferring bot ownership.
type TransferBotRequest struct {
	OwnerUserID string `json:"owner_user_id"`
}

// UpsertMemberRequest is the input for upserting a bot member.
type UpsertMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"`
}

// ListBotsResponse wraps a list of bots.
type ListBotsResponse struct {
	Items []Bot `json:"items"`
}

// ListMembersResponse wraps a list of bot members.
type ListMembersResponse struct {
	Items []BotMember `json:"items"`
}

// ListChecksResponse wraps a list of bot checks.
type ListChecksResponse struct {
	Items []BotCheck `json:"items"`
}

// ContainerLifecycle handles container lifecycle events bound to bot operations.
type ContainerLifecycle interface {
	SetupBotContainer(ctx context.Context, botID string) error
	CleanupBotContainer(ctx context.Context, botID string) error
}

// RuntimeChecker produces runtime check items for a bot.
type RuntimeChecker interface {
	// ListChecks evaluates dynamic runtime checks for a bot.
	ListChecks(ctx context.Context, botID string) []BotCheck
}

const (
	BotTypePersonal = "personal"
	BotTypePublic   = "public"
)

const (
	BotStatusCreating = "creating"
	BotStatusReady    = "ready"
	BotStatusDeleting = "deleting"
)

const (
	BotCheckStateOK      = "ok"
	BotCheckStateIssue   = "issue"
	BotCheckStateUnknown = "unknown"
)

const (
	BotCheckStatusOK      = "ok"
	BotCheckStatusWarn    = "warn"
	BotCheckStatusError   = "error"
	BotCheckStatusUnknown = "unknown"
)

const (
	BotCheckTypeContainerInit   = "container.init"
	BotCheckTypeContainerRecord = "container.record"
	BotCheckTypeContainerTask   = "container.task"
	BotCheckTypeContainerData   = "container.data_path"
	BotCheckTypeDelete          = "bot.delete"
	BotCheckTypeMCPConnection   = "mcp.connection"
	BotCheckTypeChannelConn     = "channel.connection"
)

const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleMember = "member"
)
