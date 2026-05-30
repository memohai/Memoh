package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

var usageRangePresets = []string{"24h", "7d", "30d", "all"}

// resolveUsageRange maps a --range key to a query window. Unknown/empty keys
// default to the last 7 days. Returns the normalized key (for ●-marking the
// active preset), the window start, and a human label.
func resolveUsageRange(key string) (norm string, from time.Time, label string) {
	now := time.Now().UTC()
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "24h":
		return "24h", now.Add(-24 * time.Hour), "24 hours"
	case "30d":
		return "30d", now.AddDate(0, 0, -30), "30 days"
	case "all":
		return "all", time.Unix(0, 0).UTC(), "all time"
	default:
		return "7d", now.AddDate(0, 0, -7), "7 days"
	}
}

func usageRangeView(action, current string) *Interactive {
	return &Interactive{
		Kind:  InteractiveRange,
		Range: &RangeView{Resource: "usage", Action: action, Current: current, Presets: usageRangePresets},
	}
}

func (h *Handler) buildUsageGroup() *CommandGroup {
	g := newCommandGroup("usage", "View token usage")
	g.DefaultAction = "summary"
	g.Register(SubCommand{
		Name:  "summary",
		Usage: "summary [--range 24h|7d|30d|all] - Token usage summary",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.queries == nil {
				return &Result{Text: "Token usage isn't available right now."}, nil
			}
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return nil, err
			}
			norm, from, label := resolveUsageRange(cc.Range)
			now := time.Now().UTC()
			fromTS := pgtype.Timestamptz{Time: from, Valid: true}
			toTS := pgtype.Timestamptz{Time: now, Valid: true}
			nullModel := pgtype.UUID{Valid: false}

			rows, err := h.queries.GetTokenUsageByDayAndType(cc.Ctx, dbsqlc.GetTokenUsageByDayAndTypeParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS, ModelID: nullModel,
			})
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				return &Result{
					Text:        "No token usage recorded yet.\n\nUsage appears here after the bot answers a message.",
					Interactive: usageRangeView("summary", norm),
				}, nil
			}

			type bucket struct {
				label string
				rows  []dbsqlc.GetTokenUsageByDayAndTypeRow
			}
			buckets := []bucket{{label: "Chat"}, {label: "Heartbeat"}, {label: "Schedule"}}
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
			b.WriteString(MdBold(fmt.Sprintf("Token usage (%s)", label)) + "\n\n")
			first := true
			for _, bk := range buckets {
				if len(bk.rows) == 0 {
					continue
				}
				if !first {
					b.WriteByte('\n')
				}
				first = false
				b.WriteString(MdBold(bk.label) + ":\n")
				var totalIn, totalOut int64
				for _, r := range bk.rows {
					day := r.Day.Time.Format("Jan 02")
					fmt.Fprintf(&b, "  %s  %s in · %s out\n", day, formatTokens(r.InputTokens), formatTokens(r.OutputTokens))
					totalIn += r.InputTokens
					totalOut += r.OutputTokens
				}
				fmt.Fprintf(&b, "  Total  %s in · %s out\n", formatTokens(totalIn), formatTokens(totalOut))
			}

			return &Result{
				Text:        strings.TrimRight(b.String(), "\n"),
				Interactive: usageRangeView("summary", norm),
			}, nil
		},
	})
	g.Register(SubCommand{
		Name:  "by-model",
		Usage: "by-model [--range 24h|7d|30d|all] - Token usage grouped by model",
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if h.queries == nil {
				return &Result{Text: "Token usage isn't available right now."}, nil
			}
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return nil, err
			}
			norm, from, label := resolveUsageRange(cc.Range)
			now := time.Now().UTC()
			fromTS := pgtype.Timestamptz{Time: from, Valid: true}
			toTS := pgtype.Timestamptz{Time: now, Valid: true}

			rows, err := h.queries.GetTokenUsageByModel(cc.Ctx, dbsqlc.GetTokenUsageByModelParams{
				BotID: botUUID, FromTime: fromTS, ToTime: toTS,
			})
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				return &Result{
					Text:        "No token usage recorded yet.\n\nUsage appears here after the bot answers a message.",
					Interactive: usageRangeView("by-model", norm),
				}, nil
			}

			var b strings.Builder
			b.WriteString(MdBold(fmt.Sprintf("Token usage by model (%s)", label)) + "\n\n")
			for _, r := range rows {
				name := r.ModelName
				switch {
				case strings.EqualFold(strings.TrimSpace(name), "unknown"):
					// The SQL COALESCEs missing model/provider joins to "Unknown".
					name = "Other models"
				case strings.TrimSpace(r.ProviderName) != "" &&
					!strings.EqualFold(strings.TrimSpace(r.ProviderName), "unknown") &&
					!strings.Contains(strings.ToLower(name), strings.ToLower(r.ProviderName)):
					name = fmt.Sprintf("%s (%s)", name, r.ProviderName)
				}
				fmt.Fprintf(&b, "  %s — %s in · %s out\n", name, formatTokens(r.InputTokens), formatTokens(r.OutputTokens))
			}

			return &Result{
				Text:        strings.TrimRight(b.String(), "\n"),
				Interactive: usageRangeView("by-model", norm),
			}, nil
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
