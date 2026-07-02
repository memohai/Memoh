package conversation

import (
	"strings"

	"github.com/memohai/memoh/internal/userinput"
)

func uiUserInputFromPayload(userInputID string, shortID int, status string, payload any, canRespond bool, persistTurnID ...string) *UIUserInput {
	userInputID = strings.TrimSpace(userInputID)
	if userInputID == "" {
		return nil
	}
	if status = strings.TrimSpace(status); status == "" {
		status = userinput.StatusPending
	}
	return &UIUserInput{
		UserInputID:   userInputID,
		ShortID:       shortID,
		Status:        status,
		Questions:     userinput.PayloadFromStored(payload).Questions,
		CanRespond:    canRespond,
		PersistTurnID: firstOptionalString(persistTurnID),
	}
}

func firstOptionalString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
