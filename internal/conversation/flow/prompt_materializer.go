package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextassembly"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/messageconv"
)

type initialPromptPlan struct {
	sources          []contextassembly.Source
	baselineMessages []sdk.Message
	currentSourceID  string
	contextBudget    int
	notice           string
}

type initialPromptPlanInput struct {
	Sources         []contextassembly.Source
	CurrentSourceID string
	ContextBudget   int
	Notice          string
}

type initialPromptResult struct {
	Config        agent.RunConfig
	Entries       []contextassembly.Entry
	Allocation    contextbudget.Allocation
	FixedTokens   int
	MessageTokens int
	TotalTokens   int
}

type PromptEnvelopeOverflowError struct {
	ContextBudget int
	FixedTokens   int
	MessageLimit  int
	TotalTokens   int
	RequiredIDs   []string
	Result        contextassembly.Result
	Cause         error
}

func (e *PromptEnvelopeOverflowError) Error() string {
	return fmt.Sprintf("provider prompt exceeds context budget: emitted %d tokens with limit %d", e.TotalTokens, e.ContextBudget)
}

func (e *PromptEnvelopeOverflowError) Unwrap() error {
	return e.Cause
}

func newInitialPromptPlan(input initialPromptPlanInput) (initialPromptPlan, error) {
	sources, err := clonePromptSources(input.Sources)
	if err != nil {
		return initialPromptPlan{}, err
	}
	if input.CurrentSourceID != "" {
		currentIndex, err := uniquePromptSourceIndex(sources, input.CurrentSourceID)
		if err != nil {
			return initialPromptPlan{}, err
		}
		if sources[currentIndex].Message.Role != sdk.MessageRoleUser {
			return initialPromptPlan{}, fmt.Errorf("current prompt source %q must have user role", input.CurrentSourceID)
		}
		sources[currentIndex].Retention = contextbudget.RetentionRequired
		sources[currentIndex].CompactableTokens = 0
	}
	assembled, err := contextassembly.Assemble(contextassembly.Request{
		Sources:             sources,
		SyntheticToolResult: syntheticToolClosureError,
	})
	if err != nil {
		return initialPromptPlan{}, err
	}
	baseline := make([]sdk.Message, len(assembled.Entries))
	for index, entry := range assembled.Entries {
		baseline[index] = entry.Message
	}
	return initialPromptPlan{
		sources:          sources,
		baselineMessages: baseline,
		currentSourceID:  input.CurrentSourceID,
		contextBudget:    input.ContextBudget,
		notice:           input.Notice,
	}, nil
}

func (p initialPromptPlan) BaselineMessages() ([]sdk.Message, error) {
	messages := make([]sdk.Message, len(p.baselineMessages))
	for index, message := range p.baselineMessages {
		cloned, err := clonePromptMessage(message)
		if err != nil {
			return nil, err
		}
		messages[index] = cloned
	}
	return messages, nil
}

func (p initialPromptPlan) Materialize(_ context.Context, cfg agent.RunConfig, tools []sdk.Tool) (initialPromptResult, error) {
	if !hasPromptBaseline(cfg.Messages, p.baselineMessages) {
		return initialPromptResult{}, errors.New("initial prompt baseline changed before materialization")
	}

	sources, err := clonePromptSources(p.sources)
	if err != nil {
		return initialPromptResult{}, err
	}
	if !cfg.ContextQueryMaterialized {
		imageParts, err := promptImageParts(cfg.InlineImages)
		if err != nil {
			return initialPromptResult{}, err
		}
		if len(imageParts) > 0 {
			if p.currentSourceID == "" {
				return initialPromptResult{}, errors.New("inline images require a current prompt source identity")
			}
			currentIndex, err := uniquePromptSourceIndex(sources, p.currentSourceID)
			if err != nil {
				return initialPromptResult{}, err
			}
			current := &sources[currentIndex]
			current.Message.Content = append(current.Message.Content, imageParts...)
			current.Retention = contextbudget.RetentionRequired
			cfg.ContextQueryMaterialized = true
		}
	}
	for index, message := range cfg.Messages[len(p.baselineMessages):] {
		cloned, err := clonePromptMessage(message)
		if err != nil {
			return initialPromptResult{}, fmt.Errorf("clone prepared prompt message %d: %w", index, err)
		}
		sources = append(sources, contextassembly.Source{
			ID:        fmt.Sprintf("prepared:%d", index),
			Message:   cloned,
			Retention: contextbudget.RetentionRequired,
		})
	}

	fixedTokens, err := providerFixedPromptTokens(cfg, tools)
	if err != nil {
		return initialPromptResult{}, err
	}
	var messageLimit *int
	limit := 0
	if p.contextBudget > 0 {
		limit = max(p.contextBudget-fixedTokens, 0)
		messageLimit = &limit
	}
	assembled, assembleErr := contextassembly.Assemble(contextassembly.Request{
		Sources:             sources,
		EnvelopeLimit:       messageLimit,
		Notice:              p.notice,
		SyntheticToolResult: syntheticToolClosureError,
	})
	result := initialPromptResult{
		Config:        cfg,
		Entries:       assembled.Entries,
		Allocation:    assembled.Allocation,
		FixedTokens:   fixedTokens,
		MessageTokens: assembled.EmittedTokens,
		TotalTokens:   fixedTokens + assembled.EmittedTokens,
	}
	result.Config.Messages = make([]sdk.Message, len(assembled.Entries))
	for index, entry := range assembled.Entries {
		cloned, err := clonePromptMessage(entry.Message)
		if err != nil {
			return result, err
		}
		result.Config.Messages[index] = cloned
	}
	if p.contextBudget <= 0 || result.TotalTokens <= p.contextBudget {
		return result, assembleErr
	}
	return result, &PromptEnvelopeOverflowError{
		ContextBudget: p.contextBudget,
		FixedTokens:   fixedTokens,
		MessageLimit:  limit,
		TotalTokens:   result.TotalTokens,
		RequiredIDs:   requiredPromptSourceIDs(sources),
		Result:        assembled,
		Cause:         assembleErr,
	}
}

func uniquePromptSourceIndex(sources []contextassembly.Source, id string) (int, error) {
	index := -1
	for sourceIndex, source := range sources {
		if source.ID != id {
			continue
		}
		if index >= 0 {
			return -1, fmt.Errorf("current prompt source %q is not unique", id)
		}
		index = sourceIndex
	}
	if index < 0 {
		return -1, fmt.Errorf("current prompt source %q was not found", id)
	}
	return index, nil
}

func hasPromptBaseline(messages []sdk.Message, baseline []sdk.Message) bool {
	if len(messages) < len(baseline) {
		return false
	}
	for index := range baseline {
		if !reflect.DeepEqual(messages[index], baseline[index]) {
			return false
		}
	}
	return true
}

func promptImageParts(images []sdk.ImagePart) ([]sdk.MessagePart, error) {
	parts := make([]sdk.MessagePart, 0, len(images))
	for _, image := range images {
		if strings.TrimSpace(image.Image) != "" {
			parts = append(parts, image)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	cloned, err := clonePromptMessage(sdk.Message{Role: sdk.MessageRoleUser, Content: parts})
	if err != nil {
		return nil, fmt.Errorf("clone inline prompt images: %w", err)
	}
	return cloned.Content, nil
}

func providerFixedPromptTokens(cfg agent.RunConfig, tools []sdk.Tool) (int, error) {
	toolTokens, err := messageconv.EstimateSDKToolDefinitionTokens(tools)
	if err != nil {
		return 0, err
	}
	return contextbudget.EstimateTextTokens(cfg.System) + toolTokens, nil
}

func clonePromptSources(sources []contextassembly.Source) ([]contextassembly.Source, error) {
	cloned := make([]contextassembly.Source, len(sources))
	for index, source := range sources {
		cloned[index] = source
		message, err := clonePromptMessage(source.Message)
		if err != nil {
			return nil, fmt.Errorf("clone prompt source %q: %w", source.ID, err)
		}
		cloned[index].Message = message
	}
	return cloned, nil
}

func clonePromptMessage(message sdk.Message) (sdk.Message, error) {
	encoded, err := json.Marshal(message)
	if err != nil {
		return sdk.Message{}, fmt.Errorf("marshal prompt message: %w", err)
	}
	var cloned sdk.Message
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return sdk.Message{}, fmt.Errorf("unmarshal prompt message: %w", err)
	}
	return cloned, nil
}

func requiredPromptSourceIDs(sources []contextassembly.Source) []string {
	ids := make([]string, 0)
	for _, source := range sources {
		if source.Retention == contextbudget.RetentionRequired {
			ids = append(ids, source.ID)
		}
	}
	return ids
}
