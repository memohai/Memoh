package contacts

import "time"

type Contact struct {
	ID          string
	BotID       string
	UserID      string
	DisplayName string
	Alias       string
	Tags        []string
	Status      string
	Metadata    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ContactChannel struct {
	ID         string
	BotID      string
	ContactID  string
	Platform   string
	ExternalID string
	Metadata   map[string]any
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CreateRequest struct {
	BotID       string
	UserID      string
	DisplayName string
	Alias       string
	Tags        []string
	Status      string
	Metadata    map[string]any
}

type UpdateRequest struct {
	DisplayName *string
	Alias       *string
	Tags        *[]string
	Status      *string
	Metadata    map[string]any
}
