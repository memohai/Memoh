package messagesource

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	VersionInvalid = -1
	Version1       = 1
)

type Context struct {
	Version           int    `json:"version"`
	SenderDisplayName string `json:"sender_display_name"`
	Platform          string `json:"platform"`
	ConversationType  string `json:"conversation_type"`
	ConversationName  string `json:"conversation_name"`
}

func NewV1(senderDisplayName, platform, conversationType, conversationName string) Context {
	return Context{
		Version:           Version1,
		SenderDisplayName: strings.TrimSpace(senderDisplayName),
		Platform:          strings.TrimSpace(platform),
		ConversationType:  strings.TrimSpace(conversationType),
		ConversationName:  strings.TrimSpace(conversationName),
	}
}

func (context Context) Normalize() Context {
	context.SenderDisplayName = strings.TrimSpace(context.SenderDisplayName)
	context.Platform = strings.TrimSpace(context.Platform)
	context.ConversationType = strings.TrimSpace(context.ConversationType)
	context.ConversationName = strings.TrimSpace(context.ConversationName)
	return context
}

func Encode(context Context) ([]byte, error) {
	context = context.Normalize()
	if context.Version != Version1 {
		return nil, errors.New("unsupported message source context version")
	}
	return json.Marshal(context)
}

func Decode(raw []byte) (Context, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return Context{}, nil
	}
	type wireContext struct {
		Version           *int    `json:"version"`
		SenderDisplayName *string `json:"sender_display_name"`
		Platform          *string `json:"platform"`
		ConversationType  *string `json:"conversation_type"`
		ConversationName  *string `json:"conversation_name"`
	}
	var wire wireContext
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Context{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Context{}, errors.New("message source context must contain one JSON value")
	}
	if wire.Version == nil || *wire.Version != Version1 ||
		wire.SenderDisplayName == nil || wire.Platform == nil ||
		wire.ConversationType == nil || wire.ConversationName == nil {
		return Context{}, errors.New("invalid message source context v1")
	}
	return NewV1(
		*wire.SenderDisplayName,
		*wire.Platform,
		*wire.ConversationType,
		*wire.ConversationName,
	), nil
}

func DecodeOrInvalid(raw []byte) Context {
	context, err := Decode(raw)
	if err != nil {
		return Context{Version: VersionInvalid}
	}
	return context
}
