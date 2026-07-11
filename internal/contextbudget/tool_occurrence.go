package contextbudget

import (
	"sort"
	"strconv"
	"strings"
)

const (
	DropToolOrphanCall      DropReason = "tool:orphan_call"
	DropToolOrphanResult    DropReason = "tool:orphan_result"
	DropToolDuplicateCall   DropReason = "tool:duplicate_call"
	DropToolDuplicateResult DropReason = "tool:duplicate_result"
	DropToolInvalidRole     DropReason = "tool:invalid_role"
	DropToolInvalidName     DropReason = "tool:invalid_name"
)

type ToolPartKind uint8

const (
	ToolPartCall ToolPartKind = iota + 1
	ToolPartResult
)

type ToolPart struct {
	PartIndex int
	Kind      ToolPartKind
	CallID    string
	ToolName  string
}

type ToolCarrier struct {
	// BoundaryBefore closes calls that cannot remain pending across this
	// carrier. Adapters normally set it for non-tool provider messages.
	BoundaryBefore bool
	Parts          []ToolPart
}

type ToolCarrierBinding struct {
	Group string
}

type ToolPartIssue struct {
	CarrierIndex int
	PartIndex    int
	Reason       DropReason
}

type DanglingToolCall struct {
	CarrierIndex int
	PartIndex    int
	CallID       string
	ToolName     string
	CloseBefore  int
}

type ToolPartMatch struct {
	CallCarrierIndex   int
	CallPartIndex      int
	ResultCarrierIndex int
	ResultPartIndex    int
	CallID             string
	ToolName           string
}

type ToolOccurrenceAnalysis struct {
	Bindings      []ToolCarrierBinding
	PartIssues    []ToolPartIssue
	DanglingCalls []DanglingToolCall
	Matches       []ToolPartMatch
}

type openToolCall struct {
	carrierIndex int
	partIndex    int
	callID       string
	toolName     string
}

// AnalyzeToolOccurrences derives occurrence-scoped atomic groups and
// part-level closure repairs from an ordered carrier stream.
func AnalyzeToolOccurrences(carriers []ToolCarrier) ToolOccurrenceAnalysis {
	parents := make([]int, len(carriers))
	members := make([]bool, len(carriers))
	for i := range parents {
		parents[i] = i
	}

	analysis := ToolOccurrenceAnalysis{Bindings: make([]ToolCarrierBinding, len(carriers))}
	open := make(map[string]openToolCall)
	completed := make(map[string]bool)
	for carrierIndex, carrier := range carriers {
		if carrier.BoundaryBefore {
			analysis.DanglingCalls = append(analysis.DanglingCalls, closeOpenToolCalls(open, carrierIndex)...)
		}
		for _, part := range carrier.Parts {
			callID := strings.TrimSpace(part.CallID)
			switch part.Kind {
			case ToolPartCall:
				if callID == "" {
					analysis.PartIssues = append(analysis.PartIssues, toolPartIssue(carrierIndex, part.PartIndex, DropToolOrphanCall))
					continue
				}
				if previous, exists := open[callID]; exists {
					if previous.carrierIndex == carrierIndex {
						analysis.PartIssues = append(analysis.PartIssues, toolPartIssue(carrierIndex, part.PartIndex, DropToolDuplicateCall))
						continue
					}
					analysis.DanglingCalls = append(analysis.DanglingCalls, danglingToolCall(previous, carrierIndex))
				}
				members[carrierIndex] = true
				open[callID] = openToolCall{
					carrierIndex: carrierIndex,
					partIndex:    part.PartIndex,
					callID:       callID,
					toolName:     strings.TrimSpace(part.ToolName),
				}
				completed[callID] = false
			case ToolPartResult:
				if callID == "" {
					analysis.PartIssues = append(analysis.PartIssues, toolPartIssue(carrierIndex, part.PartIndex, DropToolOrphanResult))
					continue
				}
				call, exists := open[callID]
				if !exists {
					reason := DropToolOrphanResult
					if completed[callID] {
						reason = DropToolDuplicateResult
					}
					analysis.PartIssues = append(analysis.PartIssues, toolPartIssue(carrierIndex, part.PartIndex, reason))
					continue
				}
				members[carrierIndex] = true
				unionToolCarriers(parents, carrierIndex, call.carrierIndex)
				analysis.Matches = append(analysis.Matches, ToolPartMatch{
					CallCarrierIndex:   call.carrierIndex,
					CallPartIndex:      call.partIndex,
					ResultCarrierIndex: carrierIndex,
					ResultPartIndex:    part.PartIndex,
					CallID:             call.callID,
					ToolName:           call.toolName,
				})
				delete(open, callID)
				completed[callID] = true
			}
		}
	}
	analysis.DanglingCalls = append(analysis.DanglingCalls, closeOpenToolCalls(open, len(carriers))...)
	sort.SliceStable(analysis.DanglingCalls, func(i, j int) bool {
		left := analysis.DanglingCalls[i]
		right := analysis.DanglingCalls[j]
		if left.CloseBefore != right.CloseBefore {
			return left.CloseBefore < right.CloseBefore
		}
		if left.CarrierIndex != right.CarrierIndex {
			return left.CarrierIndex < right.CarrierIndex
		}
		if left.PartIndex != right.PartIndex {
			return left.PartIndex < right.PartIndex
		}
		return left.CallID < right.CallID
	})

	groups := make(map[int]string)
	groupCount := 0
	for carrierIndex, member := range members {
		if !member {
			continue
		}
		root := findToolCarrier(parents, carrierIndex)
		group := groups[root]
		if group == "" {
			groupCount++
			group = "tool-occurrence:" + strconv.Itoa(groupCount)
			groups[root] = group
		}
		analysis.Bindings[carrierIndex].Group = group
	}
	return analysis
}

func closeOpenToolCalls(open map[string]openToolCall, before int) []DanglingToolCall {
	if len(open) == 0 {
		return nil
	}
	calls := make([]openToolCall, 0, len(open))
	for callID, call := range open {
		calls = append(calls, call)
		delete(open, callID)
	}
	sort.SliceStable(calls, func(i, j int) bool {
		if calls[i].carrierIndex != calls[j].carrierIndex {
			return calls[i].carrierIndex < calls[j].carrierIndex
		}
		if calls[i].partIndex != calls[j].partIndex {
			return calls[i].partIndex < calls[j].partIndex
		}
		return calls[i].callID < calls[j].callID
	})
	dangling := make([]DanglingToolCall, 0, len(calls))
	for _, call := range calls {
		dangling = append(dangling, danglingToolCall(call, before))
	}
	return dangling
}

func danglingToolCall(call openToolCall, before int) DanglingToolCall {
	return DanglingToolCall{
		CarrierIndex: call.carrierIndex,
		PartIndex:    call.partIndex,
		CallID:       call.callID,
		ToolName:     call.toolName,
		CloseBefore:  before,
	}
}

func toolPartIssue(carrierIndex, partIndex int, reason DropReason) ToolPartIssue {
	return ToolPartIssue{CarrierIndex: carrierIndex, PartIndex: partIndex, Reason: reason}
}

func findToolCarrier(parents []int, index int) int {
	root := index
	for parents[root] != root {
		root = parents[root]
	}
	for parents[index] != index {
		next := parents[index]
		parents[index] = root
		index = next
	}
	return root
}

func unionToolCarriers(parents []int, left, right int) {
	leftRoot := findToolCarrier(parents, left)
	rightRoot := findToolCarrier(parents, right)
	if leftRoot == rightRoot {
		return
	}
	if leftRoot < rightRoot {
		parents[rightRoot] = leftRoot
		return
	}
	parents[leftRoot] = rightRoot
}
