package identities

import "time"

// ChannelIdentity is a unified inbound identity subject across channels.
type ChannelIdentity struct {
	ID               string         `json:"id"`
	UserID           string         `json:"user_id,omitempty"`
	Channel          string         `json:"channel"`
	ChannelSubjectID string         `json:"channel_subject_id"`
	DisplayName      string         `json:"display_name,omitempty"`
	AvatarURL        string         `json:"avatar_url,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}
