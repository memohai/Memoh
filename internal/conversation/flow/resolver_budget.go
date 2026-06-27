package flow

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
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
		if callIDs := historyRecordToolCallIDs(items[i].record); len(callIDs) > 0 {
			for j := i + 1; j < len(items) && isToolRecord(items[j].record); j++ {
				if historyRecordHasMatchingToolResult(items[j].record, callIDs) {
					group.items = append(group.items, items[j])
					deleteToolResultIDs(callIDs, items[j].record)
				}
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

func forceKeepToolResultForBudget(records []historyfrag.HistoryRecord, toolCallID string) []historyfrag.HistoryRecord {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return records
	}
	for i := len(records) - 1; i >= 0; i-- {
		if !isToolRecord(records[i]) {
			continue
		}
		for _, id := range historyRecordToolResultIDs(records[i]) {
			if id != toolCallID {
				continue
			}
			records[i].Budget.Overflow = contextfrag.OverflowKeep
			return records
		}
	}
	return records
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
	pendingToolCallIDs := map[string]struct{}{}
	for _, record := range records {
		if isToolRecord(record) {
			if historyRecordHasMatchingToolResult(record, pendingToolCallIDs) {
				out = append(out, record)
				deleteToolResultIDs(pendingToolCallIDs, record)
			}
			continue
		}
		out = append(out, record)
		pendingToolCallIDs = historyRecordToolCallIDs(record)
	}
	return out
}

func isToolRecord(record historyfrag.HistoryRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.ModelMessage.Role), "tool")
}

func isAssistantToolCallRecord(record historyfrag.HistoryRecord) bool {
	return len(historyRecordToolCallIDs(record)) > 0
}

func historyRecordToolCallIDs(record historyfrag.HistoryRecord) map[string]struct{} {
	msg := record.ModelMessage
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
		return nil
	}
	ids := make(map[string]struct{}, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		if id := strings.TrimSpace(call.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	for _, part := range historyRecordToolParts(msg) {
		partType := normalizeToolPartType(part)
		if strings.Contains(partType, "tool-call") {
			if id := strings.TrimSpace(toolPartString(part, "toolCallId", "tool_call_id")); id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func historyRecordHasMatchingToolResult(record historyfrag.HistoryRecord, callIDs map[string]struct{}) bool {
	if len(callIDs) == 0 || !isToolRecord(record) {
		return false
	}
	for _, id := range historyRecordToolResultIDs(record) {
		if _, ok := callIDs[id]; ok {
			return true
		}
	}
	return false
}

func deleteToolResultIDs(callIDs map[string]struct{}, record historyfrag.HistoryRecord) {
	for _, id := range historyRecordToolResultIDs(record) {
		delete(callIDs, id)
	}
}

func historyRecordToolResultIDs(record historyfrag.HistoryRecord) []string {
	msg := record.ModelMessage
	ids := make([]string, 0, 1)
	if id := strings.TrimSpace(msg.ToolCallID); id != "" {
		ids = append(ids, id)
	}
	for _, part := range historyRecordToolParts(msg) {
		partType := normalizeToolPartType(part)
		if !strings.Contains(partType, "tool-result") {
			continue
		}
		if id := strings.TrimSpace(toolPartString(part, "toolCallId", "tool_call_id")); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func historyRecordToolParts(msg conversation.ModelMessage) []map[string]any {
	if len(msg.Content) == 0 {
		return nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(msg.Content, &parts); err != nil {
		return nil
	}
	return parts
}

func normalizeToolPartType(part map[string]any) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(toolPartString(part, "type"))), "_", "-")
}

func toolPartString(part map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := part[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
