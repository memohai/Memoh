package accounts

import "time"

// Account represents a human account credential record.
type Account struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email,omitempty"`
	Role        string    `json:"role"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastLoginAt time.Time `json:"last_login_at,omitzero"`
}

// CreateAccountRequest is the input for creating an account.
type CreateAccountRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email,omitempty"`
	Role        string `json:"role,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

// UpdateAccountRequest is the input for admin-level account updates.
type UpdateAccountRequest struct {
	Role        *string `json:"role,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// UpdateProfileRequest is the input for self-service profile updates.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
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
