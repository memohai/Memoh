package contextassembly

import (
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/messageconv"
)

type Source struct {
	ID      string
	Message sdk.Message
	// CompactableTokens is raw preselection pressure. Generated repair and
	// notice tokens must not be included.
	CompactableTokens int
	DropTier          int
	Retention         contextbudget.Retention
	PolicyReason      contextbudget.DropReason
}

type Request struct {
	Sources []Source
	// EnvelopeLimit is the remaining budget after caller-owned non-source
	// reserves. Assemble deducts Notice itself. Nil is unlimited.
	EnvelopeLimit *int
	// Notice is optional system text emitted only after a spatial budget trim.
	Notice              string
	SyntheticToolResult string
}

type Entry struct {
	Message sdk.Message
	// SourceIndex is -1 for notice and synthetic entries so generated content
	// cannot inherit source metadata or artifact identity.
	SourceIndex int
	Synthetic   bool
}

type Result struct {
	Entries       []Entry
	Allocation    contextbudget.Allocation
	EmittedTokens int
}

type OverflowError struct {
	Limit         int
	EmittedTokens int
	Result        Result
}

func (e *OverflowError) Error() string {
	return fmt.Sprintf("assembled context exceeds envelope: emitted %d tokens with limit %d", e.EmittedTokens, e.Limit)
}

type UnrenderableRequiredError struct {
	Sources []contextbudget.Decision
	Result  Result
}

func (e *UnrenderableRequiredError) Error() string {
	return fmt.Sprintf("%d required context source(s) have no provider-visible payload", len(e.Sources))
}

type preparedEntry struct {
	message     sdk.Message
	sourceIndex int
	synthetic   bool
}

func Assemble(request Request) (Result, error) {
	entries, items, unrenderable := prepareSources(request.Sources, request.SyntheticToolResult)
	allocation := contextbudget.Allocate(contextbudget.Request{
		SourceLimit: request.EnvelopeLimit,
		Items:       items,
	})

	notice, noticeTokens, hasNotice := prepareNotice(request.Notice)
	includeNotice := request.EnvelopeLimit != nil && allocation.BudgetTrimmed && hasNotice
	if includeNotice {
		limit := max(*request.EnvelopeLimit-noticeTokens, 0)
		allocation = contextbudget.Allocate(contextbudget.Request{
			SourceLimit: &limit,
			Items:       items,
		})
	}

	kept := make([]bool, len(request.Sources))
	for _, decision := range allocation.Kept {
		kept[decision.Index] = true
	}
	result := Result{Allocation: allocation}
	if includeNotice {
		result.Entries = append(result.Entries, Entry{Message: notice, SourceIndex: -1})
		result.EmittedTokens += noticeTokens
	}
	for _, entry := range entries {
		if !kept[entry.sourceIndex] {
			continue
		}
		sourceIndex := entry.sourceIndex
		if entry.synthetic {
			sourceIndex = -1
		}
		result.Entries = append(result.Entries, Entry{
			Message:     entry.message,
			SourceIndex: sourceIndex,
			Synthetic:   entry.synthetic,
		})
		result.EmittedTokens += messageconv.EstimateSDKMessageTokens(entry.message)
	}

	if len(unrenderable) > 0 {
		return result, &UnrenderableRequiredError{Sources: unrenderable, Result: result}
	}
	if request.EnvelopeLimit == nil {
		return result, nil
	}
	limit := max(*request.EnvelopeLimit, 0)
	if result.EmittedTokens <= limit {
		return result, nil
	}
	return result, &OverflowError{
		Limit:         limit,
		EmittedTokens: result.EmittedTokens,
		Result:        result,
	}
}

func prepareSources(sources []Source, syntheticToolResult string) ([]preparedEntry, []contextbudget.Item, []contextbudget.Decision) {
	messages := make([]sdk.Message, len(sources))
	for i, source := range sources {
		messages[i] = source.Message
	}
	repair := messageconv.RepairSDKToolOccurrences(messages, syntheticToolResult)
	entries := make([]preparedEntry, 0, len(repair.Entries))
	hasEntries := make([]bool, len(sources))
	for _, repaired := range repair.Entries {
		message := messageconv.CanonicalSDKMessage(repaired.Message)
		if strings.TrimSpace(string(message.Role)) == "" || !hasProviderVisiblePayload(message) {
			continue
		}
		entries = append(entries, preparedEntry{
			message:     message,
			sourceIndex: repaired.SourceIndex,
			synthetic:   repaired.Synthetic,
		})
		hasEntries[repaired.SourceIndex] = true
	}

	groups := make([]string, len(sources))
	tokens := make([]int, len(sources))
	repairedMessages := make([]sdk.Message, len(entries))
	for i, entry := range entries {
		repairedMessages[i] = entry.message
		tokens[entry.sourceIndex] += messageconv.EstimateSDKMessageTokens(entry.message)
	}
	analysis := messageconv.AnalyzeSDKToolOccurrences(repairedMessages)
	for i, binding := range analysis.Bindings {
		if binding.Group == "" {
			continue
		}
		sourceIndex := entries[i].sourceIndex
		if groups[sourceIndex] == "" {
			groups[sourceIndex] = binding.Group
		}
	}

	items := make([]contextbudget.Item, len(sources))
	var unrenderable []contextbudget.Decision
	for i, source := range sources {
		retention := source.Retention
		policyReason := source.PolicyReason
		if !hasEntries[i] {
			if retention == contextbudget.RetentionRequired {
				unrenderable = append(unrenderable, contextbudget.Decision{Index: i, ID: source.ID})
			} else {
				retention = contextbudget.RetentionDrop
				if policyReason == "" {
					policyReason = contextbudget.DropPolicy
				}
			}
		}
		items[i] = contextbudget.Item{
			ID:                source.ID,
			Group:             groups[i],
			Tokens:            tokens[i],
			CompactableTokens: source.CompactableTokens,
			DropTier:          source.DropTier,
			Retention:         retention,
			PolicyReason:      policyReason,
		}
	}
	return entries, items, unrenderable
}

func hasProviderVisiblePayload(message sdk.Message) bool {
	for _, part := range message.Content {
		switch typed := part.(type) {
		case sdk.TextPart:
			if strings.TrimSpace(typed.Text) != "" {
				return true
			}
		case sdk.ImagePart:
			if strings.TrimSpace(typed.Image) != "" {
				return true
			}
		case sdk.FilePart:
			if strings.TrimSpace(typed.Data) != "" {
				return true
			}
		case sdk.ToolCallPart, sdk.ToolResultPart:
			return true
		case sdk.ReasoningPart:
			continue
		default:
			return true
		}
	}
	return false
}

func prepareNotice(text string) (sdk.Message, int, bool) {
	if strings.TrimSpace(text) == "" {
		return sdk.Message{}, 0, false
	}
	notice := sdk.SystemMessage(text)
	return notice, messageconv.EstimateSDKMessageTokens(notice), true
}
