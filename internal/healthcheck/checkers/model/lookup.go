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

// GetBotModelIDs fetches the chat, memory, and embedding model IDs for a bot.
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
	if bot.MemoryModelID.Valid {
		m.MemoryModelID = bot.MemoryModelID.String()
	}
	if bot.EmbeddingModelID.Valid {
		m.EmbeddingModelID = bot.EmbeddingModelID.String()
	}
	return m, nil
}
