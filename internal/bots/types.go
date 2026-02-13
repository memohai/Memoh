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
	CheckKey string         `json:"check_key"`
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

// ListCheckKeysResponse wraps the list of available check keys.
type ListCheckKeysResponse struct {
	Keys []string `json:"keys"`
}

// ContainerLifecycle handles container lifecycle events bound to bot operations.
type ContainerLifecycle interface {
	SetupBotContainer(ctx context.Context, botID string) error
	CleanupBotContainer(ctx context.Context, botID string) error
}

// RuntimeChecker produces runtime check items for a bot.
type RuntimeChecker interface {
	// CheckKeys returns the check keys this checker can evaluate for a bot.
	CheckKeys(ctx context.Context, botID string) []string
	// RunCheck evaluates a single check key and returns the result.
	RunCheck(ctx context.Context, botID, key string) BotCheck
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
	BotCheckKeyContainerInit   = "container.init"
	BotCheckKeyContainerRecord = "container.record"
	BotCheckKeyContainerTask   = "container.task"
	BotCheckKeyContainerData   = "container.data_path"
	BotCheckKeyDelete          = "bot.delete"
)

const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleMember = "member"
)
