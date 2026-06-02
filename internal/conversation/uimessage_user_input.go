package conversation

import "strings"

func uiUserInputFromPayload(userInputID string, shortID int, status string, payload any, canRespond bool) *UIUserInput {
	userInputID = strings.TrimSpace(userInputID)
	if userInputID == "" {
		return nil
	}
	if status = strings.TrimSpace(status); status == "" {
		status = "pending"
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return &UIUserInput{
			UserInputID: userInputID,
			ShortID:     shortID,
			Status:      status,
			CanRespond:  canRespond,
		}
	}
	return &UIUserInput{
		UserInputID: userInputID,
		ShortID:     shortID,
		Status:      status,
		Question:    stringFromAny(obj["question"]),
		Options:     uiUserInputOptionsFromAny(obj["options"]),
		AllowCustom: boolFromAny(obj["allow_custom"], false),
		InputType:   stringFromAny(obj["input_type"]),
		Placeholder: stringFromAny(obj["placeholder"]),
		CanRespond:  canRespond,
	}
}

func uiUserInputOptionsFromAny(value any) []UIUserInputOption {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	options := make([]UIUserInputOption, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			label := stringFromAny(item)
			if label == "" {
				continue
			}
			options = append(options, UIUserInputOption{
				ID:    label,
				Label: label,
				Value: item,
			})
			continue
		}
		id := stringFromAny(obj["id"])
		label := stringFromAny(obj["label"])
		if id == "" && label == "" {
			continue
		}
		if id == "" {
			id = label
		}
		if label == "" {
			label = id
		}
		options = append(options, UIUserInputOption{
			ID:          id,
			Label:       label,
			Description: stringFromAny(obj["description"]),
			Value:       obj["value"],
			InputType:   normalizeUIUserInputOptionType(stringFromAny(obj["input_type"])),
			Placeholder: stringFromAny(obj["placeholder"]),
		})
	}
	return options
}

func normalizeUIUserInputOptionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text":
		return "text"
	default:
		return ""
	}
}
