package userinput

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	ToolNameAskUser = "ask_user"

	ProviderSourceACPMCP = "acp_mcp"

	StatusPending   = "pending"
	StatusSubmitted = "submitted"
	StatusCanceled  = "canceled"
	StatusExpired   = "expired"
	StatusFailed    = "failed"

	DeferredKind = "user_input"

	// PayloadVersion is the canonical ask_user payload version written to
	// storage. Older rows are upgraded on read by PayloadFromStored.
	PayloadVersion = 2

	QuestionKindSingleSelect = "single_select"
	QuestionKindMultiSelect  = "multi_select"
	QuestionKindText         = "text"

	MaxQuestionsPerRequest = 4
	MinOptionsPerQuestion  = 2
	MaxOptionsPerQuestion  = 20
)

var (
	ErrNotFound       = errors.New("user input request not found")
	ErrAlreadyDecided = errors.New("user input request already decided")
)

type CreatePendingInput struct {
	BotID                        string
	SessionID                    string
	RouteID                      string
	ChannelIdentityID            string
	RequestedByChannelIdentityID string
	ToolCallID                   string
	ToolName                     string
	Input                        any
	ProviderMetadata             map[string]any
	SourcePlatform               string
	ReplyTarget                  string
	ConversationType             string
	ExpiresAt                    *time.Time
	PersistTurnID                string
}

type ResolveInput struct {
	BotID                  string
	SessionID              string
	BaseHeadTurnID         string
	ExplicitID             string
	ReplyExternalMessageID string
}

// QuestionAnswer is the user's answer to a single question of an ask_user
// request. Selections are always arrays; single_select is an array of one.
type QuestionAnswer struct {
	QuestionID string   `json:"question_id"`
	OptionIDs  []string `json:"option_ids,omitempty"`
	CustomText string   `json:"custom_text,omitempty"`
	Text       string   `json:"text,omitempty"`
}

type SubmitInput struct {
	RequestID              string
	ActorChannelIdentityID string
	Answers                []QuestionAnswer
}

type CancelInput struct {
	RequestID              string
	ActorChannelIdentityID string
	Reason                 string
}

type Request struct {
	ID                      string         `json:"id"`
	BotID                   string         `json:"bot_id"`
	SessionID               string         `json:"session_id"`
	RouteID                 string         `json:"route_id,omitempty"`
	ChannelIdentityID       string         `json:"channel_identity_id,omitempty"`
	ToolCallID              string         `json:"tool_call_id"`
	ToolName                string         `json:"tool_name"`
	ShortID                 int            `json:"short_id"`
	Status                  string         `json:"status"`
	Input                   map[string]any `json:"input,omitempty"`
	UIPayload               UIPayload      `json:"ui_payload"`
	Result                  map[string]any `json:"result,omitempty"`
	ProviderMetadata        map[string]any `json:"-"`
	PromptExternalMessageID string         `json:"prompt_external_message_id,omitempty"`
	SourcePlatform          string         `json:"source_platform,omitempty"`
	ReplyTarget             string         `json:"reply_target,omitempty"`
	ConversationType        string         `json:"conversation_type,omitempty"`
	PersistTurnID           string         `json:"persist_turn_id,omitempty"`
	ExpiresAt               *time.Time     `json:"expires_at,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	RespondedAt             *time.Time     `json:"responded_at,omitempty"`
	CanceledAt              *time.Time     `json:"canceled_at,omitempty"`
}

// UIPayload is the canonical, normalized ask_user payload (v2). It is the
// single shape stored, streamed, and rendered; ParseAskUserPayload is the
// only writer and PayloadFromStored the only reader.
type UIPayload struct {
	Version   int          `json:"version"`
	Questions []UIQuestion `json:"questions"`
}

type UIQuestion struct {
	ID          string     `json:"id"`
	Text        string     `json:"text"`
	Kind        string     `json:"kind"`
	Options     []UIOption `json:"options,omitempty"`
	AllowCustom bool       `json:"allow_custom,omitempty"`
	Placeholder string     `json:"placeholder,omitempty"`
}

type UIOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func (p UIPayload) Question(id string) (UIQuestion, bool) {
	for _, question := range p.Questions {
		if question.ID == id {
			return question, true
		}
	}
	return UIQuestion{}, false
}

func (q UIQuestion) Option(id string) (UIOption, bool) {
	for _, option := range q.Options {
		if option.ID == id {
			return option, true
		}
	}
	return UIOption{}, false
}

func ResultBytes(req Request) []byte {
	if len(req.Result) == 0 {
		return []byte("{}")
	}
	data, err := json.Marshal(req.Result)
	if err != nil {
		return []byte("{}")
	}
	return data
}
