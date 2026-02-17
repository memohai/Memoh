package bind

import (
	"errors"
	"time"
)

// Errors returned by bind operations.
var (
	ErrCodeNotFound = errors.New("bind code not found")
	ErrCodeUsed     = errors.New("bind code already used")
	ErrCodeExpired  = errors.New("bind code expired")
	ErrCodeMismatch = errors.New("bind code context mismatch")
	ErrLinkConflict = errors.New("channel identity user link conflict")
)

// Code represents a one-time bind code for linking channel identity to user.
type Code struct {
	ID                      string    `json:"id"`
	Platform                string    `json:"platform,omitempty"`
	Token                   string    `json:"token"`
	IssuedByUserID          string    `json:"issued_by_user_id"`
	ExpiresAt               time.Time `json:"expires_at,omitzero"`
	UsedAt                  time.Time `json:"used_at,omitzero"`
	UsedByChannelIdentityID string    `json:"used_by_channel_identity_id,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}
