package agent

import (
	"encoding/json"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"
)

// SessionContext carries request-scoped identity and routing information.
type SessionContext struct {
	BotID             string
	ChatID            string
	SessionID         string
	ChannelIdentityID string
	CurrentPlatform   string
	ReplyTarget       string
	SessionToken      string //nolint:gosec // carries session credential material at runtime
	IsSubagent        bool
}

// SkillEntry represents a skill loaded from the bot container.
type SkillEntry struct {
	Name        string
	Description string
	Content     string
	Metadata    map[string]any
}

// Schedule represents a scheduled task definition.
type Schedule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	MaxCalls    *int   `json:"maxCalls,omitempty"`
	Command     string `json:"command"`
}

// LoopDetectionConfig controls loop detection behavior.
type LoopDetectionConfig struct {
	Enabled bool
}

// RunConfig holds everything needed for a single agent invocation.
type RunConfig struct {
	Model              *sdk.Model
	ReasoningEffort    string
	Messages           []sdk.Message
	Query              string
	System             string
	SessionType        string
	SupportsImageInput bool
	InlineImages       []sdk.ImagePart
	Identity           SessionContext
	Skills             []SkillEntry
	LoopDetection      LoopDetectionConfig
}

// GenerateResult holds the result of a non-streaming agent invocation.
type GenerateResult struct {
	Messages    []sdk.Message
	Text        string
	Attachments []FileAttachment
	Reactions   []ReactionItem
	Speeches    []SpeechItem
	Usage       *sdk.Usage
}

// FileAttachment represents a file reference extracted from agent output.
type FileAttachment struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
	Mime string `json:"mime,omitempty"`
	Name string `json:"name,omitempty"`
}

// ReactionItem represents an emoji reaction extracted from agent output.
type ReactionItem struct {
	Emoji string `json:"emoji"`
}

// SpeechItem represents a TTS request extracted from agent output.
type SpeechItem struct {
	Text string `json:"text"`
}

// SystemFile is a file loaded from the bot container for prompt generation.
type SystemFile struct {
	Filename string
	Content  string
}

// ModelConfig holds provider and model information resolved from DB.
type ModelConfig struct {
	ModelID         string
	ClientType      string
	APIKey          string //nolint:gosec // carries provider credential material at runtime
	BaseURL         string
	ReasoningConfig *ReasoningConfig
}

// ReasoningConfig controls extended thinking/reasoning behavior.
type ReasoningConfig struct {
	Enabled bool
	Effort  string
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// StripTagsFromMessages strips attachment/reaction/speech tags from assistant messages.
func StripTagsFromMessages(msgs []sdk.Message) []sdk.Message {
	resolvers := DefaultTagResolvers()
	result := make([]sdk.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role != sdk.MessageRoleAssistant {
			result = append(result, msg)
			continue
		}
		cleaned := make([]sdk.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			if tp, ok := part.(sdk.TextPart); ok {
				text, _ := ExtractTagsFromText(tp.Text, resolvers)
				cleaned = append(cleaned, sdk.TextPart{Text: text})
			} else {
				cleaned = append(cleaned, part)
			}
		}
		msg.Content = cleaned
		result = append(result, msg)
	}
	return result
}

// TimeNow is a hook for testing. Defaults to time.Now.
var TimeNow = time.Now
