package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/acl"
)

func (h *Handler) buildAccessGroup() *CommandGroup {
	g := newCommandGroup("access", "Inspect identity and permission context")
	g.DefaultAction = "show"
	g.Register(SubCommand{
		Name:  "show",
		Usage: "show - Show current identity, write access, and chat ACL context",
		Handler: func(cc CommandContext) (string, error) {
			writeAccess := "no"
			if cc.Role == "owner" {
				writeAccess = "yes"
			}

			pairs := []kv{
				{"Channel Identity", fallbackValue(cc.ChannelIdentityID)},
				{"Linked User", fallbackValue(cc.UserID)},
				{"Bot Role", fallbackValue(cc.Role)},
				{"Write Commands", writeAccess},
				{"Channel", fallbackValue(cc.ChannelType)},
				{"Conversation Type", fallbackValue(cc.ConversationType)},
				{"Conversation ID", fallbackValue(cc.ConversationID)},
				{"Thread ID", fallbackValue(cc.ThreadID)},
			}
			if strings.TrimSpace(cc.RouteID) != "" {
				pairs = append(pairs, kv{"Route ID", cc.RouteID})
			}
			if strings.TrimSpace(cc.SessionID) != "" {
				pairs = append(pairs, kv{"Session ID", cc.SessionID})
			}

			aclStatus := "unavailable"
			if h.aclEvaluator != nil && strings.TrimSpace(cc.ChannelType) != "" {
				allowed, err := h.aclEvaluator.Evaluate(cc.Ctx, acl.EvaluateRequest{
					BotID:             cc.BotID,
					ChannelIdentityID: cc.ChannelIdentityID,
					ChannelType:       cc.ChannelType,
					SourceScope: acl.SourceScope{
						ConversationType: cc.ConversationType,
						ConversationID:   cc.ConversationID,
						ThreadID:         cc.ThreadID,
					},
				})
				switch {
				case err != nil:
					aclStatus = "error: " + err.Error()
				case allowed:
					aclStatus = "allow"
				default:
					aclStatus = "deny"
				}
			}
			pairs = append(pairs, kv{"Chat ACL", aclStatus})

			return formatKV(pairs), nil
		},
	})
	return g
}

func fallbackValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(none)"
	}
	return value
}

func formatChangedValue(label, before, after string) string {
	return fmt.Sprintf("%s: %s -> %s", label, fallbackValue(before), fallbackValue(after))
}
