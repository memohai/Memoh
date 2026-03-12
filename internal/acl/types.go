package acl

import "time"

const (
	ActionChatTrigger = "chat.trigger"

	EffectAllow = "allow"
	EffectDeny  = "deny"

	SubjectKindGuestAll        = "guest_all"
	SubjectKindUser            = "user"
	SubjectKindChannelIdentity = "channel_identity"
)

type Rule struct {
	ID                         string    `json:"id"`
	BotID                      string    `json:"bot_id"`
	Action                     string    `json:"action"`
	Effect                     string    `json:"effect"`
	SubjectKind                string    `json:"subject_kind"`
	UserID                     string    `json:"user_id,omitempty"`
	ChannelIdentityID          string    `json:"channel_identity_id,omitempty"`
	UserUsername               string    `json:"user_username,omitempty"`
	UserDisplayName            string    `json:"user_display_name,omitempty"`
	UserAvatarURL              string    `json:"user_avatar_url,omitempty"`
	ChannelType                string    `json:"channel_type,omitempty"`
	ChannelSubjectID           string    `json:"channel_subject_id,omitempty"`
	ChannelIdentityDisplayName string    `json:"channel_identity_display_name,omitempty"`
	ChannelIdentityAvatarURL   string    `json:"channel_identity_avatar_url,omitempty"`
	LinkedUserID               string    `json:"linked_user_id,omitempty"`
	LinkedUserUsername         string    `json:"linked_user_username,omitempty"`
	LinkedUserDisplayName      string    `json:"linked_user_display_name,omitempty"`
	LinkedUserAvatarURL        string    `json:"linked_user_avatar_url,omitempty"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type ListRulesResponse struct {
	Items []Rule `json:"items"`
}

type UpsertRuleRequest struct {
	UserID            string `json:"user_id,omitempty"`
	ChannelIdentityID string `json:"channel_identity_id,omitempty"`
}

type UserCandidate struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Email       string `json:"email,omitempty"`
}

type UserCandidateListResponse struct {
	Items []UserCandidate `json:"items"`
}

type ChannelIdentityCandidate struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id,omitempty"`
	Channel           string `json:"channel"`
	ChannelSubjectID  string `json:"channel_subject_id"`
	DisplayName       string `json:"display_name,omitempty"`
	AvatarURL         string `json:"avatar_url,omitempty"`
	LinkedUsername    string `json:"linked_username,omitempty"`
	LinkedDisplayName string `json:"linked_display_name,omitempty"`
	LinkedAvatarURL   string `json:"linked_avatar_url,omitempty"`
}

type ChannelIdentityCandidateListResponse struct {
	Items []ChannelIdentityCandidate `json:"items"`
}
