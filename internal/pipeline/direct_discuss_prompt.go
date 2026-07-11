package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/historyfrag"
)

type DirectDiscussPromptSource struct {
	ID          string
	Message     sdk.Message
	Required    bool
	Compactable bool
	SummaryFrag *contextfrag.ContextFrag
	ImageRefs   []ImageAttachmentRef
}

type DirectDiscussPromptInput struct {
	Sources         []DirectDiscussPromptSource
	CurrentSourceID string
	ActorUserID     string
}

type DirectDiscussPromptReceipt interface {
	Finish(context.Context) error
}

type PreparedDirectDiscussPrompt struct {
	RunConfig agentpkg.RunConfig
	Receipt   DirectDiscussPromptReceipt
}

type DirectDiscussPromptPreparer interface {
	PrepareDirectDiscussPrompt(context.Context, DirectDiscussPromptInput) (PreparedDirectDiscussPrompt, error)
}

func buildDirectDiscussPromptInput(
	messages []ContextMessage,
	artifacts []CompactionArtifact,
	scope contextfrag.Scope,
	lateBinding string,
	actorUserID string,
) DirectDiscussPromptInput {
	artifactsByID := make(map[string]CompactionArtifact, len(artifacts))
	for _, artifact := range artifacts {
		if id := strings.TrimSpace(artifact.ID); id != "" {
			artifactsByID[id] = artifact
		}
	}
	input := DirectDiscussPromptInput{
		Sources:     make([]DirectDiscussPromptSource, 0, len(messages)+1),
		ActorUserID: strings.TrimSpace(actorUserID),
	}
	for index, message := range messages {
		sourceID := directDiscussSourceID(message, index)
		source := DirectDiscussPromptSource{
			ID:          sourceID,
			Message:     directDiscussSDKMessage(message),
			Required:    message.Current,
			Compactable: strings.TrimSpace(message.CompactionArtifactID) == "",
		}
		if artifactID := strings.TrimSpace(message.CompactionArtifactID); artifactID != "" {
			source.Required = true
			source.Compactable = false
			if artifact, ok := artifactsByID[artifactID]; ok {
				frag := compactionSummarySourceFrag(artifact, scope)
				source.SummaryFrag = &frag
			}
		}
		if message.Current {
			source.ImageRefs = append([]ImageAttachmentRef(nil), message.ImageRefs...)
			input.CurrentSourceID = sourceID
		}
		input.Sources = append(input.Sources, source)
	}
	input.Sources = append(input.Sources, DirectDiscussPromptSource{
		ID:       "discuss:late-binding",
		Message:  sdk.UserMessage(lateBinding),
		Required: true,
	})
	return input
}

func directDiscussSDKMessage(message ContextMessage) sdk.Message {
	if len(message.RawContent) > 0 {
		raw, err := json.Marshal(struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}{Role: message.Role, Content: message.RawContent})
		if err == nil {
			var decoded sdk.Message
			if json.Unmarshal(raw, &decoded) == nil {
				return decoded
			}
		}
	}
	if message.Role == "assistant" {
		return sdk.AssistantMessage(message.Content)
	}
	return sdk.UserMessage(message.Content)
}

func directDiscussSourceID(message ContextMessage, index int) string {
	if id := strings.TrimSpace(message.CompactionArtifactID); id != "" {
		return "compaction:" + id
	}
	if id := strings.TrimSpace(message.SourceMessageID); id != "" {
		return "history:" + id
	}
	ids := make([]string, 0, len(message.RenderedMessageIDs))
	for _, id := range message.RenderedMessageIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		return "rendered:" + strings.Join(ids, "|")
	}
	return fmt.Sprintf("discuss-source:%03d", index)
}

func compactionSummarySourceFrag(artifact CompactionArtifact, scope contextfrag.Scope) contextfrag.ContextFrag {
	coveredRefs := make([]contextfrag.ContextRef, 0, len(artifact.Sources))
	for _, source := range artifact.Sources {
		if contextfrag.ValidateContextRef(source.Ref) == nil {
			coveredRefs = append(coveredRefs, source.Ref)
		}
	}
	record := historyfrag.SummaryRecord(artifact.ID, artifact.Summary, coveredRefs, scope)
	return historyfrag.ToFrag(record)
}
