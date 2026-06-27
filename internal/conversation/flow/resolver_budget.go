package flow

import (
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/historyfrag"
)

type historyBudgetItem struct {
	index        int
	record       historyfrag.HistoryRecord
	tokens       int
	priority     int
	forceKeep    bool
	overflowDrop bool
}

type historyBudgetGroup struct {
	items        []historyBudgetItem
	tokens       int
	priority     int
	newest       int
	forceKeep    bool
	overflowDrop bool
}

func selectHistoryRecordsForBudget(records []historyfrag.HistoryRecord, maxTokens int) ([]historyfrag.HistoryRecord, bool, int) {
	totalTokens := totalHistoryTokens(records)
	if maxTokens == 0 || len(records) == 0 {
		return records, false, totalTokens
	}

	groups := historyBudgetGroups(records)
	if totalTokens <= maxTokens && !historyBudgetGroupsHaveOverflowDrop(groups) {
		return records, false, totalTokens
	}
	selected := make(map[int]historyfrag.HistoryRecord, len(records))
	usedTokens := 0
	for _, group := range groups {
		if !group.forceKeep {
			continue
		}
		for _, item := range group.items {
			selected[item.index] = item.record
		}
		usedTokens += group.tokens
	}

	remaining := maxTokens - usedTokens
	if remaining < 0 {
		remaining = 0
	}
	candidates := make([]historyBudgetGroup, 0, len(groups))
	for _, group := range groups {
		if group.forceKeep || group.overflowDrop {
			continue
		}
		candidates = append(candidates, group)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority > candidates[j].priority
		}
		return candidates[i].newest > candidates[j].newest
	})
	for _, group := range candidates {
		if group.tokens > remaining {
			continue
		}
		for _, item := range group.items {
			selected[item.index] = item.record
		}
		remaining -= group.tokens
	}

	retained := make([]historyfrag.HistoryRecord, 0, len(records))
	for i := range records {
		if record, ok := selected[i]; ok {
			retained = append(retained, record)
		}
	}
	retained = dropOrphanToolRecords(retained)
	return retained, len(retained) != len(records), totalTokens
}

func historyBudgetGroupsHaveOverflowDrop(groups []historyBudgetGroup) bool {
	for _, group := range groups {
		if group.overflowDrop {
			return true
		}
	}
	return false
}

func totalHistoryTokens(records []historyfrag.HistoryRecord) int {
	total := 0
	for _, record := range records {
		total += estimateMessageTokens(record.ModelMessage)
	}
	return total
}

func historyBudgetGroups(records []historyfrag.HistoryRecord) []historyBudgetGroup {
	items := historyBudgetItems(records)
	groups := make([]historyBudgetGroup, 0, len(items))
	for i := 0; i < len(items); i++ {
		group := historyBudgetGroup{items: []historyBudgetItem{items[i]}}
		if isAssistantToolCallRecord(items[i].record) {
			for j := i + 1; j < len(items) && isToolRecord(items[j].record); j++ {
				group.items = append(group.items, items[j])
				i = j
			}
		}
		group = summarizeHistoryBudgetGroup(group)
		groups = append(groups, group)
	}
	return groups
}

func historyBudgetItems(records []historyfrag.HistoryRecord) []historyBudgetItem {
	latestSummaryByText := make(map[string]int)
	for i, record := range records {
		if key := summaryMessageKey(record); key != "" {
			latestSummaryByText[key] = i
		}
	}

	items := make([]historyBudgetItem, 0, len(records))
	for i, record := range records {
		tokens := estimateMessageTokens(record.ModelMessage)
		item := historyBudgetItem{
			index:    i,
			record:   record,
			tokens:   tokens,
			priority: historyRecordPriority(record),
		}
		if exceedsDropBudget(record.Budget, record, tokens) {
			item.overflowDrop = true
		}
		if key := summaryMessageKey(record); key != "" {
			item.priority = max(item.priority, 95)
			if latestSummaryByText[key] != i {
				item.overflowDrop = true
				item.forceKeep = false
			} else {
				item.record.Coverage = mergedSummaryCoverageForKey(item.record, records, key)
				item.forceKeep = true
			}
		}
		if !item.overflowDrop && record.Budget.Overflow == contextfrag.OverflowKeep {
			item.forceKeep = true
			item.priority = 100
		}
		items = append(items, item)
	}
	return items
}

func mergedSummaryCoverageForKey(record historyfrag.HistoryRecord, records []historyfrag.HistoryRecord, key string) *contextfrag.SummaryCoverage {
	var coveredRefs []contextfrag.ContextRef
	for _, candidate := range records {
		if summaryMessageKey(candidate) != key || candidate.Coverage == nil {
			continue
		}
		coveredRefs = appendUniqueContextRefs(coveredRefs, candidate.Coverage.CoveredRefs...)
	}
	if len(coveredRefs) == 0 {
		return record.Coverage
	}
	coverage := contextfrag.NewSummaryCoverage(record.Ref, append([]contextfrag.ContextRef(nil), coveredRefs...))
	return &coverage
}

func appendUniqueContextRefs(refs []contextfrag.ContextRef, incoming ...contextfrag.ContextRef) []contextfrag.ContextRef {
	for _, ref := range incoming {
		found := false
		for _, existing := range refs {
			if existing.EqualIdentity(ref) {
				found = true
				break
			}
		}
		if !found {
			refs = append(refs, ref)
		}
	}
	return refs
}

func summarizeHistoryBudgetGroup(group historyBudgetGroup) historyBudgetGroup {
	for _, item := range group.items {
		group.tokens += item.tokens
		group.priority = max(group.priority, item.priority)
		group.newest = max(group.newest, item.index)
		group.forceKeep = group.forceKeep || item.forceKeep
		group.overflowDrop = group.overflowDrop || item.overflowDrop
	}
	return group
}

func historyRecordPriority(record historyfrag.HistoryRecord) int {
	if record.Kind == contextfrag.KindConversationSummary || record.Lifecycle == historyfrag.LifecycleActiveSummary {
		return 95
	}
	return historyfrag.ToFrag(record).Priority
}

func exceedsDropBudget(policy contextfrag.BudgetPolicy, record historyfrag.HistoryRecord, tokens int) bool {
	if policy.Overflow != contextfrag.OverflowDrop {
		return false
	}
	if policy.MaxTokens > 0 && tokens > policy.MaxTokens {
		return true
	}
	text := record.ModelMessage.TextContent()
	return policy.MaxChars > 0 && len(text) > policy.MaxChars
}

func dropOrphanToolRecords(records []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	out := records[:0]
	previousAssistantToolCall := false
	for _, record := range records {
		if isToolRecord(record) {
			if previousAssistantToolCall {
				out = append(out, record)
			}
			continue
		}
		out = append(out, record)
		previousAssistantToolCall = isAssistantToolCallRecord(record)
	}
	return out
}

func isToolRecord(record historyfrag.HistoryRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.ModelMessage.Role), "tool")
}

func isAssistantToolCallRecord(record historyfrag.HistoryRecord) bool {
	msg := record.ModelMessage
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
		return false
	}
	if len(msg.ToolCalls) > 0 {
		return true
	}
	for _, part := range msg.ContentParts() {
		partType := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(part.Type)), "_", "-")
		if strings.Contains(partType, "tool-call") {
			return true
		}
	}
	return false
}

func summaryMessageKey(record historyfrag.HistoryRecord) string {
	if record.Kind != contextfrag.KindConversationSummary && record.Lifecycle != historyfrag.LifecycleActiveSummary && record.Lifecycle != historyfrag.LifecycleLegacySummary {
		return ""
	}
	text := strings.TrimSpace(record.ModelMessage.TextContent())
	if text == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(record.ModelMessage.Role)) + "\x00" + text
}
