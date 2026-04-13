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
			if h.queries == nil {
				return "Usage info is not available.", nil
			}
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return "", err
			}
			now := time.Now().UTC()
			from := now.AddDate(0, 0, -7)
			fromTS := pgtype.Timestamptz{Time: from, Valid: true}
			toTS := pgtype.Timestamptz{Time: now, Valid: true}
			nullModel := pgtype.UUID{Valid: false}

			rows, err := h.queries.GetTokenUsageByDayAndType(cc.Ctx, dbsqlc.GetTokenUsageByDayAndTypeParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS, ModelID: nullModel,
			})
			if err != nil {
				return "", err
			}

			if len(rows) == 0 {
				return "No token usage in the last 7 days.", nil
			}

			type bucket struct {
				label string
				rows  []dbsqlc.GetTokenUsageByDayAndTypeRow
			}
			buckets := []bucket{
				{label: "Chat"},
				{label: "Heartbeat"},
				{label: "Schedule"},
			}
			for _, r := range rows {
				switch r.SessionType {
				case "heartbeat":
					buckets[1].rows = append(buckets[1].rows, r)
				case "schedule":
					buckets[2].rows = append(buckets[2].rows, r)
				default:
					buckets[0].rows = append(buckets[0].rows, r)
				}
			}

			var b strings.Builder
			b.WriteString("Token usage (last 7 days):\n\n")

			first := true
			for _, bk := range buckets {
				if len(bk.rows) == 0 {
					continue
				}
				if !first {
					b.WriteByte('\n')
				}
				first = false
				b.WriteString(bk.label + ":\n")
				var totalIn, totalOut int64
				for _, r := range bk.rows {
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
			if h.queries == nil {
				return "Usage info is not available.", nil
			}
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return "", err
			}
			now := time.Now().UTC()
			from := now.AddDate(0, 0, -7)
			fromTS := pgtype.Timestamptz{Time: from, Valid: true}
			toTS := pgtype.Timestamptz{Time: now, Valid: true}

			rows, err := h.queries.GetTokenUsageByModel(cc.Ctx, dbsqlc.GetTokenUsageByModelParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS,
			})
			if err != nil {
				return "", err
			}

			if len(rows) == 0 {
				return "No token usage in the last 7 days.", nil
			}

			var b strings.Builder
			b.WriteString("Token usage by model (last 7 days):\n\n")

			for _, r := range rows {
				fmt.Fprintf(&b, "  %s (%s): in=%d out=%d\n", r.ModelName, r.ProviderName, r.InputTokens, r.OutputTokens)
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
