package accounts

import "time"

const (
	PasswordProviderType = "password"
)

// Account represents a human user profile.
type Account struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Timezone    string    `json:"timezone,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastLoginAt time.Time `json:"last_login_at,omitempty"`
}

type PasswordIdentity struct {
	ID               string
	UserID           string
	Subject          string
	Email            string
	Username         string
	CredentialSecret string
	LastLoginAt      time.Time
}

// CreateAccountRequest is the input for creating a password-backed account.
type CreateAccountRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"` //nolint:gosec // intentional request credential
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

// UpdateAccountRequest is the input for admin-level account updates.
type UpdateAccountRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// UpdateProfileRequest is the input for self-service profile updates.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Timezone    *string `json:"timezone,omitempty"`
}

// UpdatePasswordRequest is the input for password change.
type UpdatePasswordRequest struct {
	CurrentPassword string `json:"current_password,omitempty"`
	NewPassword     string `json:"new_password"`
}

// ResetPasswordRequest is the input for admin password reset.
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ListAccountsResponse wraps a list of accounts.
type ListAccountsResponse struct {
	Items []Account `json:"items"`
}

type UpsertAccountInput struct {
	UserID      string
	Username    string
	Email       string
	DisplayName string
	AvatarURL   string
	IsActive    bool
}

type CreateHumanInput struct {
	Username    string
	Email       string
	DisplayName string
	AvatarURL   string
	IsActive    bool
}

type UpsertPasswordIdentityInput struct {
	UserID           string
	Subject          string
	Email            string
	Username         string
	DisplayName      string
	AvatarURL        string
	CredentialSecret string
}

type UpdateAdminInput struct {
	UserID      string
	DisplayName string
	AvatarURL   string
	IsActive    bool
}

type UpdateProfileInput struct {
	UserID      string
	DisplayName string
	AvatarURL   string
	Timezone    string
}
