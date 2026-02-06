package preauth

import "time"

type Key struct {
	ID             string
	BotID          string
	Token          string
	IssuedByUserID string
	ExpiresAt      time.Time
	UsedAt         time.Time
	CreatedAt      time.Time
}
