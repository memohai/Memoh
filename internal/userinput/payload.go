package userinput

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const defaultQuestion = "Please choose an option."

var ErrInvalidAskUserInput = errors.New("invalid ask_user input")

func ValidateAskUserInput(input any) error {
	raw := payloadObject(input)
	if len(raw) == 0 {
		return fmt.Errorf("%w: input must be a JSON object with a non-empty question", ErrInvalidAskUserInput)
	}
	if stringValue(raw["question"]) == "" {
		return fmt.Errorf("%w: question is required", ErrInvalidAskUserInput)
	}
	return nil
}

func normalizeUIPayload(input any) UIPayload {
	raw := payloadObject(input)

	payload := UIPayload{
		Question:    stringValue(raw["question"]),
		AllowCustom: boolValue(raw["allow_custom"]),
		InputType:   stringValue(raw["input_type"]),
		Placeholder: stringValue(raw["placeholder"]),
	}
	payload.Options = normalizeOptions(raw["options"])
	if payload.Question == "" {
		payload.Question = defaultQuestion
	}
	if payload.InputType == "" {
		if len(payload.Options) > 0 {
			payload.InputType = "choice"
		} else {
			payload.InputType = "text"
		}
	}
	if len(payload.Options) == 0 {
		payload.AllowCustom = true
	}
	return payload
}

func payloadObject(input any) map[string]any {
	if input == nil {
		return nil
	}
	if raw, ok := input.(map[string]any); ok {
		return raw
	}
	data, err := json.Marshal(input)
	if err != nil || len(data) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	return raw
}

func normalizeOptions(value any) []UIOption {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	options := make([]UIOption, 0, len(items))
	for idx, item := range items {
		option := normalizeOption(item, idx)
		if option.ID == "" && option.Label == "" {
			continue
		}
		if option.ID == "" {
			option.ID = fmt.Sprintf("option_%d", idx+1)
		}
		if option.Label == "" {
			option.Label = option.ID
		}
		options = append(options, option)
	}
	return options
}

func normalizeOption(item any, idx int) UIOption {
	switch typed := item.(type) {
	case map[string]any:
		option := UIOption{
			ID:          stringValue(typed["id"]),
			Label:       stringValue(typed["label"]),
			Description: stringValue(typed["description"]),
			Value:       typed["value"],
			InputType:   normalizeInputType(stringValue(typed["input_type"])),
			Placeholder: stringValue(typed["placeholder"]),
		}
		if option.ID == "" {
			option.ID = stringValue(typed["value"])
		}
		if option.ID == "" {
			option.ID = stringValue(typed["label"])
		}
		if option.ID == "" {
			option.ID = fmt.Sprintf("option_%d", idx+1)
		}
		if option.Label == "" {
			option.Label = stringValue(typed["value"])
		}
		if option.Value == nil && option.InputType != "text" {
			option.Value = option.ID
		}
		return option
	case string:
		trimmed := strings.TrimSpace(typed)
		return UIOption{ID: trimmed, Label: trimmed, Value: trimmed}
	default:
		text := stringValue(typed)
		return UIOption{ID: text, Label: text, Value: typed}
	}
}

func normalizeInputType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text":
		return "text"
	default:
		return ""
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
