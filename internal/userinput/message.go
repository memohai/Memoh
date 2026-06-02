package userinput

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

func BuildPrompt(req Request) channel.Message {
	text := fmt.Sprintf("User input required #%d\n%s", req.ShortID, req.UIPayload.Question)
	if len(req.UIPayload.Options) > 0 {
		var lines []string
		for idx, option := range req.UIPayload.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", idx+1, option.Label))
		}
		text += "\n\n" + strings.Join(lines, "\n")
	}
	text += fmt.Sprintf("\n\nReply with /choose %d <option> or /cancel %d.", req.ShortID, req.ShortID)

	actions := make([]channel.Action, 0, len(req.UIPayload.Options)+1)
	for _, option := range req.UIPayload.Options {
		actions = append(actions, channel.Action{
			Type:  ActionTypeUserInput,
			Label: option.Label,
			Value: ActionSubmit + ":" + req.ID + ":" + option.ID,
		})
	}
	actions = append(actions, channel.Action{
		Type:  ActionTypeUserInput,
		Label: "Cancel",
		Value: ActionCancel + ":" + req.ID,
	})

	return channel.Message{
		Format:  channel.MessageFormatPlain,
		Text:    text,
		Actions: actions,
		Metadata: map[string]any{
			"user_input_id":       req.ID,
			"user_input_short_id": req.ShortID,
			"tool_call_id":        req.ToolCallID,
		},
	}
}

func ParseActionValue(value string) (action, requestID, optionID string, ok bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 {
		return "", "", "", false
	}
	action = strings.TrimSpace(parts[0])
	requestID = strings.TrimSpace(parts[1])
	if len(parts) > 2 {
		optionID = strings.TrimSpace(parts[2])
	}
	switch action {
	case ActionSubmit:
		return action, requestID, optionID, requestID != "" && optionID != ""
	case ActionCancel:
		return action, requestID, "", requestID != ""
	default:
		return "", "", "", false
	}
}
