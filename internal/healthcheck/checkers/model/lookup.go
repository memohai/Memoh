package modelchecker

import (
	"context"
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// QueriesLookup adapts sqlc.Queries to the BotModelLookup interface.
type QueriesLookup struct {
	queries *sqlc.Queries
}

// NewQueriesLookup creates a BotModelLookup backed by sqlc.Queries.
func NewQueriesLookup(queries *sqlc.Queries) *QueriesLookup {
	return &QueriesLookup{queries: queries}
}

// GetBotModelIDs fetches model IDs configured directly on the bot.
func (l *QueriesLookup) GetBotModelIDs(ctx context.Context, botID string) (BotModels, error) {
	if strings.TrimSpace(botID) == "" {
		return BotModels{}, fmt.Errorf("bot id is required")
	}
	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return BotModels{}, fmt.Errorf("invalid bot id: %w", err)
	}

	bot, err := l.queries.GetBotByID(ctx, pgID)
	if err != nil {
		return BotModels{}, fmt.Errorf("get bot: %w", err)
	}

	var m BotModels
	if bot.ChatModelID.Valid {
		m.ChatModelID = bot.ChatModelID.String()
	}
	return m, nil
}
