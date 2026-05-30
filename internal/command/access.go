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
			if cc.WriteAccess {
				writeAccess = "yes"
			}

			pairs := []kv{
				{"Bot Role", fallbackValue(cc.Role)},
				{"Write Commands", writeAccess},
				{"Channel", fallbackValue(cc.ChannelType)},
				{"Conversation Type", fallbackValue(cc.ConversationType)},
				// Identifier rows vanish when empty rather than printing "(none)";
				// they are support/debug detail, not the facts the user came for.
				{"Channel Identity", strings.TrimSpace(cc.ChannelIdentityID)},
				{"Linked User", strings.TrimSpace(cc.UserID)},
				{"Conversation ID", strings.TrimSpace(cc.ConversationID)},
				{"Thread ID", strings.TrimSpace(cc.ThreadID)},
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
					// Don't leak raw DB/driver error text into user output.
					aclStatus = "error"
				case allowed:
					aclStatus = "allow"
				default:
					aclStatus = "deny"
				}
			}
			pairs = append(pairs, kv{"Chat ACL", humanizeStatus(aclStatus)})

			return formatKVTitled("Access", pairs), nil
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

// formatChangedValue renders a write-command confirmation. Success is
// acknowledged once with a ✅ badge; the new value is rendered tap-to-copy when
// it is a machine token. When the value did not actually change, it returns an
// idempotent line instead of a confusing transition.
func formatChangedValue(label, before, after string) string {
	a := fallbackValue(after)
	if strings.EqualFold(strings.TrimSpace(before), strings.TrimSpace(after)) {
		return fmt.Sprintf("%s is already set to %s.", label, renderValue(a))
	}
	return fmt.Sprintf("✅ %s changed to %s.", label, renderValue(a))
}
