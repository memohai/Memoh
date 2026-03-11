package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

func (h *Handler) buildUsageGroup() *CommandGroup {
	g := newCommandGroup("usage", "View token usage")
	g.DefaultAction = "summary"
	g.Register(SubCommand{
		Name:  "summary",
		Usage: "summary - Token usage summary (last 7 days)",
		Handler: func(cc CommandContext) (string, error) {
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return "", err
			}
			now := time.Now().UTC()
			from := now.AddDate(0, 0, -7)
			fromTS := pgtype.Timestamptz{Time: from, Valid: true}
			toTS := pgtype.Timestamptz{Time: now, Valid: true}
			nullModel := pgtype.UUID{Valid: false}

			chatRows, err := h.queries.GetMessageTokenUsageByDay(cc.Ctx, dbsqlc.GetMessageTokenUsageByDayParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS, ModelID: nullModel,
			})
			if err != nil {
				return "", err
			}

			hbRows, err := h.queries.GetHeartbeatTokenUsageByDay(cc.Ctx, dbsqlc.GetHeartbeatTokenUsageByDayParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS, ModelID: nullModel,
			})
			if err != nil {
				return "", err
			}

			if len(chatRows) == 0 && len(hbRows) == 0 {
				return "No token usage in the last 7 days.", nil
			}

			var b strings.Builder
			b.WriteString("Token usage (last 7 days):\n\n")

			if len(chatRows) > 0 {
				b.WriteString("Chat:\n")
				var totalIn, totalOut int64
				for _, r := range chatRows {
					day := r.Day.Time.Format("01-02")
					fmt.Fprintf(&b, "  %s: in=%d out=%d\n", day, r.InputTokens, r.OutputTokens)
					totalIn += r.InputTokens
					totalOut += r.OutputTokens
				}
				fmt.Fprintf(&b, "  Total: in=%d out=%d\n", totalIn, totalOut)
			}

			if len(hbRows) > 0 {
				if len(chatRows) > 0 {
					b.WriteByte('\n')
				}
				b.WriteString("Heartbeat:\n")
				var totalIn, totalOut int64
				for _, r := range hbRows {
					day := r.Day.Time.Format("01-02")
					fmt.Fprintf(&b, "  %s: in=%d out=%d\n", day, r.InputTokens, r.OutputTokens)
					totalIn += r.InputTokens
					totalOut += r.OutputTokens
				}
				fmt.Fprintf(&b, "  Total: in=%d out=%d\n", totalIn, totalOut)
			}

			return strings.TrimRight(b.String(), "\n"), nil
		},
	})
	g.Register(SubCommand{
		Name:  "by-model",
		Usage: "by-model - Token usage grouped by model",
		Handler: func(cc CommandContext) (string, error) {
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return "", err
			}
			now := time.Now().UTC()
			from := now.AddDate(0, 0, -7)
			fromTS := pgtype.Timestamptz{Time: from, Valid: true}
			toTS := pgtype.Timestamptz{Time: now, Valid: true}

			chatRows, err := h.queries.GetMessageTokenUsageByModel(cc.Ctx, dbsqlc.GetMessageTokenUsageByModelParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS,
			})
			if err != nil {
				return "", err
			}
			hbRows, err := h.queries.GetHeartbeatTokenUsageByModel(cc.Ctx, dbsqlc.GetHeartbeatTokenUsageByModelParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS,
			})
			if err != nil {
				return "", err
			}

			if len(chatRows) == 0 && len(hbRows) == 0 {
				return "No token usage in the last 7 days.", nil
			}

			var b strings.Builder
			b.WriteString("Token usage by model (last 7 days):\n\n")

			if len(chatRows) > 0 {
				b.WriteString("Chat:\n")
				for _, r := range chatRows {
					fmt.Fprintf(&b, "  %s (%s): in=%d out=%d\n", r.ModelName, r.ProviderName, r.InputTokens, r.OutputTokens)
				}
			}

			if len(hbRows) > 0 {
				if len(chatRows) > 0 {
					b.WriteByte('\n')
				}
				b.WriteString("Heartbeat:\n")
				for _, r := range hbRows {
					fmt.Fprintf(&b, "  %s (%s): in=%d out=%d\n", r.ModelName, r.ProviderName, r.InputTokens, r.OutputTokens)
				}
			}

			return strings.TrimRight(b.String(), "\n"), nil
		},
	})
	return g
}

func parseBotUUID(botID string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(botID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid bot ID: %w", err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
