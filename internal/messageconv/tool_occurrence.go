package messageconv

import (
	"sort"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
)

const defaultSyntheticToolClosureResult = "tool execution interrupted before a response was recorded"

type SDKToolRepairEntry struct {
	Message     sdk.Message
	SourceIndex int
	Synthetic   bool
}

type SDKToolRepairResult struct {
	Entries          []SDKToolRepairEntry
	RemovedParts     []contextbudget.ToolPartIssue
	SynthesizedCalls []contextbudget.DanglingToolCall
	NormalizedParts  int
	Changed          bool
}

type sdkToolPartPosition struct {
	messageIndex int
	partIndex    int
}

// AnalyzeSDKToolOccurrences adapts typed SDK messages to the representation-
// independent occurrence analyzer.
func AnalyzeSDKToolOccurrences(messages []sdk.Message) contextbudget.ToolOccurrenceAnalysis {
	carriers := make([]contextbudget.ToolCarrier, len(messages))
	var roleIssues []contextbudget.ToolPartIssue
	for messageIndex, message := range messages {
		carrier := contextbudget.ToolCarrier{
			BoundaryBefore: message.Role != sdk.MessageRoleTool,
		}
		for partIndex, part := range message.Content {
			switch typed := part.(type) {
			case sdk.ToolCallPart:
				if message.Role != sdk.MessageRoleAssistant {
					roleIssues = append(roleIssues, contextbudget.ToolPartIssue{
						CarrierIndex: messageIndex,
						PartIndex:    partIndex,
						Reason:       contextbudget.DropToolInvalidRole,
					})
					continue
				}
				if strings.TrimSpace(typed.ToolName) == "" {
					roleIssues = append(roleIssues, contextbudget.ToolPartIssue{
						CarrierIndex: messageIndex,
						PartIndex:    partIndex,
						Reason:       contextbudget.DropToolInvalidName,
					})
					continue
				}
				carrier.Parts = append(carrier.Parts, contextbudget.ToolPart{
					PartIndex: partIndex,
					Kind:      contextbudget.ToolPartCall,
					CallID:    typed.ToolCallID,
					ToolName:  typed.ToolName,
				})
			case sdk.ToolResultPart:
				if message.Role != sdk.MessageRoleTool {
					roleIssues = append(roleIssues, contextbudget.ToolPartIssue{
						CarrierIndex: messageIndex,
						PartIndex:    partIndex,
						Reason:       contextbudget.DropToolInvalidRole,
					})
					continue
				}
				carrier.Parts = append(carrier.Parts, contextbudget.ToolPart{
					PartIndex: partIndex,
					Kind:      contextbudget.ToolPartResult,
					CallID:    typed.ToolCallID,
				})
			}
		}
		carriers[messageIndex] = carrier
	}
	analysis := contextbudget.AnalyzeToolOccurrences(carriers)
	analysis.PartIssues = append(analysis.PartIssues, roleIssues...)
	sort.SliceStable(analysis.PartIssues, func(i, j int) bool {
		left := analysis.PartIssues[i]
		right := analysis.PartIssues[j]
		if left.CarrierIndex != right.CarrierIndex {
			return left.CarrierIndex < right.CarrierIndex
		}
		if left.PartIndex != right.PartIndex {
			return left.PartIndex < right.PartIndex
		}
		return left.Reason < right.Reason
	})
	return analysis
}

// RepairSDKToolOccurrences removes invalid parts and inserts synthetic error
// results for dangling calls while preserving each entry's source index.
func RepairSDKToolOccurrences(messages []sdk.Message, syntheticResult string) SDKToolRepairResult {
	normalized, normalizedParts := normalizeSDKToolMessages(messages)
	analysis := AnalyzeSDKToolOccurrences(normalized)
	alignSDKToolResults(normalized, analysis.Matches, normalizedParts)
	result := SDKToolRepairResult{
		RemovedParts:     append([]contextbudget.ToolPartIssue(nil), analysis.PartIssues...),
		SynthesizedCalls: append([]contextbudget.DanglingToolCall(nil), analysis.DanglingCalls...),
		NormalizedParts:  len(normalizedParts),
		Changed:          len(normalizedParts) > 0 || len(analysis.PartIssues) > 0 || len(analysis.DanglingCalls) > 0,
	}
	issuesBySource := make(map[int]map[int]struct{})
	for _, issue := range analysis.PartIssues {
		parts := issuesBySource[issue.CarrierIndex]
		if parts == nil {
			parts = make(map[int]struct{})
			issuesBySource[issue.CarrierIndex] = parts
		}
		parts[issue.PartIndex] = struct{}{}
	}

	syntheticResult = strings.TrimSpace(syntheticResult)
	if syntheticResult == "" {
		syntheticResult = defaultSyntheticToolClosureResult
	}
	syntheticByBoundary := make(map[int][]contextbudget.DanglingToolCall)
	for _, dangling := range analysis.DanglingCalls {
		syntheticByBoundary[dangling.CloseBefore] = append(syntheticByBoundary[dangling.CloseBefore], dangling)
	}

	result.Entries = make([]SDKToolRepairEntry, 0, len(normalized)+len(analysis.DanglingCalls))
	for messageIndex := 0; messageIndex <= len(normalized); messageIndex++ {
		for _, dangling := range syntheticByBoundary[messageIndex] {
			result.Entries = append(result.Entries, SDKToolRepairEntry{
				Message: sdk.ToolMessage(sdk.ToolResultPart{
					ToolCallID: dangling.CallID,
					ToolName:   dangling.ToolName,
					Result:     syntheticResult,
					IsError:    true,
				}),
				SourceIndex: dangling.CarrierIndex,
				Synthetic:   true,
			})
		}
		if messageIndex == len(normalized) {
			break
		}

		message := normalized[messageIndex]
		invalidParts := issuesBySource[messageIndex]
		if len(invalidParts) > 0 {
			content := make([]sdk.MessagePart, 0, len(message.Content)-len(invalidParts))
			for partIndex, part := range message.Content {
				if _, invalid := invalidParts[partIndex]; invalid {
					continue
				}
				content = append(content, part)
			}
			message.Content = content
			if len(message.Content) == 0 {
				continue
			}
		}
		result.Entries = append(result.Entries, SDKToolRepairEntry{Message: message, SourceIndex: messageIndex})
	}
	return result
}

func normalizeSDKToolMessages(messages []sdk.Message) ([]sdk.Message, map[sdkToolPartPosition]struct{}) {
	normalized := make([]sdk.Message, len(messages))
	changed := make(map[sdkToolPartPosition]struct{})
	for messageIndex, message := range messages {
		normalized[messageIndex] = message
		normalized[messageIndex].Content = append([]sdk.MessagePart(nil), message.Content...)
		for partIndex, part := range message.Content {
			switch typed := part.(type) {
			case sdk.ToolCallPart:
				if message.Role != sdk.MessageRoleAssistant {
					continue
				}
				next := typed
				next.ToolCallID = strings.TrimSpace(next.ToolCallID)
				next.ToolName = strings.TrimSpace(next.ToolName)
				if next.ToolCallID != typed.ToolCallID || next.ToolName != typed.ToolName {
					normalized[messageIndex].Content[partIndex] = next
					changed[sdkToolPartPosition{messageIndex: messageIndex, partIndex: partIndex}] = struct{}{}
				}
			case sdk.ToolResultPart:
				if message.Role != sdk.MessageRoleTool {
					continue
				}
				next := typed
				next.ToolCallID = strings.TrimSpace(next.ToolCallID)
				next.ToolName = strings.TrimSpace(next.ToolName)
				if next.ToolCallID != typed.ToolCallID || next.ToolName != typed.ToolName {
					normalized[messageIndex].Content[partIndex] = next
					changed[sdkToolPartPosition{messageIndex: messageIndex, partIndex: partIndex}] = struct{}{}
				}
			}
		}
	}
	return normalized, changed
}

func alignSDKToolResults(
	messages []sdk.Message,
	matches []contextbudget.ToolPartMatch,
	changed map[sdkToolPartPosition]struct{},
) {
	for _, match := range matches {
		if match.ResultCarrierIndex < 0 || match.ResultCarrierIndex >= len(messages) {
			continue
		}
		content := messages[match.ResultCarrierIndex].Content
		if match.ResultPartIndex < 0 || match.ResultPartIndex >= len(content) {
			continue
		}
		result, ok := content[match.ResultPartIndex].(sdk.ToolResultPart)
		if !ok || result.ToolCallID == match.CallID && result.ToolName == match.ToolName {
			continue
		}
		result.ToolCallID = match.CallID
		result.ToolName = match.ToolName
		messages[match.ResultCarrierIndex].Content[match.ResultPartIndex] = result
		changed[sdkToolPartPosition{messageIndex: match.ResultCarrierIndex, partIndex: match.ResultPartIndex}] = struct{}{}
	}
}
