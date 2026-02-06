package bots

import "time"

type Bot struct {
	ID          string         `json:"id"`
	OwnerUserID string         `json:"owner_user_id"`
	Type        string         `json:"type"`
	DisplayName string         `json:"display_name"`
	AvatarURL   string         `json:"avatar_url,omitempty"`
	IsActive    bool           `json:"is_active"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type BotMember struct {
	BotID     string    `json:"bot_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateBotRequest struct {
	Type        string         `json:"type"`
	DisplayName string         `json:"display_name,omitempty"`
	AvatarURL   string         `json:"avatar_url,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateBotRequest struct {
	DisplayName *string        `json:"display_name,omitempty"`
	AvatarURL   *string        `json:"avatar_url,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type TransferBotRequest struct {
	OwnerUserID string `json:"owner_user_id"`
}

type UpsertMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"`
}

type ListBotsResponse struct {
	Items []Bot `json:"items"`
}

type ListMembersResponse struct {
	Items []BotMember `json:"items"`
}

const (
	BotTypePersonal = "personal"
	BotTypePublic   = "public"
)

const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleMember = "member"
)
