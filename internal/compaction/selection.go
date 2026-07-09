package compaction

import (
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/userinput"
)

type CompactPolicy string

const (
	CompactPolicyCanDrop             CompactPolicy = "can_drop"
	CompactPolicyPreserveRecent      CompactPolicy = "preserve_recent"
	CompactPolicyPreserveToolClosure CompactPolicy = "preserve_tool_closure"
	CompactPolicyMustKeep            CompactPolicy = "must_keep"
)

// CompactionCandidate is the typed view of one uncompacted history row used
// during candidate selection. RawContent and RawUsage retain the raw row payload
// so token estimation stays byte-identical to the legacy path; Record carries the
// typed classifier output used for policies, tool-aware boundaries, and
// summarizer rendering.
type CompactionCandidate struct {
	ID         pgtype.UUID
	RawContent []byte
	RawUsage   []byte
	Record     historyfrag.HistoryRecord
	Policies   []CompactPolicy
}

func (c CompactionCandidate) HasPolicy(policy CompactPolicy) bool {
	for _, p := range c.Policies {
		if p == policy {
			return true
		}
	}
	return false
}

// itemsFromRows classifies each uncompacted row into a typed CompactionCandidate.
// A row that cannot be classified is skipped (and counted) rather than aborting
// the whole compaction: it simply stays in active history to be retried, which
// matches the legacy path's inability to fail at selection time.
func itemsFromRows(rows []sqlc.ListUncompactedMessagesBySessionRow) ([]CompactionCandidate, int) {
	items := make([]CompactionCandidate, 0, len(rows))
	skipped := 0
	for _, row := range rows {
		record, err := historyfrag.FromDBMessage(rowToMessage(row), rowScopeFallback(row))
		if err != nil {
			skipped++
			continue
		}
		items = append(items, CompactionCandidate{
			ID:         row.ID,
			RawContent: row.Content,
			RawUsage:   row.Usage,
			Record:     record,
			Policies:   candidatePolicies(record),
		})
	}
	if len(items) > 0 {
		propagateMustKeepAcrossToolExchanges(items)
		markSelectionPolicies(items)
	}
	return items, skipped
}

// toolExchangeGroups partitions items into tool-exchange groups: a maximal run
// starting at a non-result row that carries the tool-closure policy (an
// assistant tool-call row) followed by every immediately-adjacent tool-result
// row answering it. Rows outside any exchange are returned as singleton
// groups, so an exchange can never be split by ID-blind index slicing.
func toolExchangeGroups(items []CompactionCandidate) [][]int {
	groups := make([][]int, 0, len(items))
	for i := 0; i < len(items); {
		if isToolResultItem(items[i]) || !items[i].HasPolicy(CompactPolicyPreserveToolClosure) {
			groups = append(groups, []int{i})
			i++
			continue
		}
		group := []int{i}
		j := i + 1
		for j < len(items) && isToolResultItem(items[j]) {
			group = append(group, j)
			j++
		}
		groups = append(groups, group)
		i = j
	}
	return groups
}

// propagateMustKeepAcrossToolExchanges spreads CompactPolicyMustKeep across an
// entire tool exchange: if the call row or any of its answering tool-result
// rows is must-keep, every row in that exchange becomes must-keep, so
// excluding only the must-keep row can never orphan a sibling tool call/result.
func propagateMustKeepAcrossToolExchanges(items []CompactionCandidate) {
	for _, group := range toolExchangeGroups(items) {
		if len(group) < 2 {
			continue
		}
		mustKeep := false
		for _, idx := range group {
			if items[idx].HasPolicy(CompactPolicyMustKeep) {
				mustKeep = true
				break
			}
		}
		if !mustKeep {
			continue
		}
		for _, idx := range group {
			items[idx].Policies = appendPolicy(items[idx].Policies, CompactPolicyMustKeep)
		}
	}
}

func markSelectionPolicies(items []CompactionCandidate) {
	if latestUser := latestUserIndex(items); latestUser == 0 && len(items) > 1 {
		tailStart := recentTailProtectedStart(items, 1)
		for i := range items {
			if i == 0 || i >= tailStart {
				items[i].Policies = appendPolicy(items[i].Policies, CompactPolicyPreserveRecent)
				continue
			}
			if items[i].HasPolicy(CompactPolicyMustKeep) {
				continue
			}
			items[i].Policies = appendPolicy(items[i].Policies, CompactPolicyCanDrop)
		}
		return
	}

	start := recentProtectedStart(items)
	for i := range items {
		if i < start {
			if !items[i].HasPolicy(CompactPolicyMustKeep) {
				items[i].Policies = appendPolicy(items[i].Policies, CompactPolicyCanDrop)
			}
			continue
		}
		items[i].Policies = appendPolicy(items[i].Policies, CompactPolicyPreserveRecent)
	}
}

func recentProtectedStart(items []CompactionCandidate) int {
	if len(items) == 0 {
		return 0
	}
	if latestUser := latestUserIndex(items); latestUser >= 0 {
		return latestUser
	}
	return recentTailProtectedStart(items, 0)
}

func latestUserIndex(items []CompactionCandidate) int {
	for i := len(items) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(items[i].Record.ModelMessage.Role), "user") {
			return i
		}
	}
	return -1
}

func recentTailProtectedStart(items []CompactionCandidate, minStart int) int {
	start := len(items) - 1
	for start > minStart && isToolClosureResultItem(items[start]) {
		start--
	}
	return start
}

func rowToMessage(row sqlc.ListUncompactedMessagesBySessionRow) messagepkg.Message {
	return messagepkg.Message{
		ID:                      formatUUID(row.ID),
		BotID:                   formatUUID(row.BotID),
		SessionID:               formatUUID(row.SessionID),
		SenderChannelIdentityID: formatUUID(row.SenderChannelIdentityID),
		SenderUserID:            formatUUID(row.SenderUserID),
		SenderDisplayName:       textValue(row.SenderDisplayName),
		SenderAvatarURL:         textValue(row.SenderAvatarUrl),
		Platform:                textValue(row.Platform),
		ExternalMessageID:       textValue(row.ExternalMessageID),
		SourceReplyToMessageID:  textValue(row.SourceReplyToMessageID),
		Role:                    row.Role,
		Content:                 row.Content,
		Metadata:                metadataMap(row.Metadata),
		Usage:                   row.Usage,
		CompactID:               formatUUID(row.CompactID),
		EventID:                 formatUUID(row.EventID),
		DisplayContent:          textValue(row.DisplayText),
		CreatedAt:               row.CreatedAt.Time,
	}
}

func rowScopeFallback(row sqlc.ListUncompactedMessagesBySessionRow) historyfrag.ScopeFallback {
	return historyfrag.ScopeFallback{
		ConversationType: textValue(row.ConversationType),
		ConversationName: strings.TrimSpace(row.ConversationName),
		ReplyTarget:      textValue(row.ReplyTarget),
	}
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func metadataMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func candidatePolicies(record historyfrag.HistoryRecord) []CompactPolicy {
	var policies []CompactPolicy
	if isToolExchangeRecord(record) {
		policies = appendPolicy(policies, CompactPolicyPreserveToolClosure)
	}
	if isAskUserRecord(record) {
		policies = appendPolicy(policies, CompactPolicyMustKeep)
		policies = appendPolicy(policies, CompactPolicyPreserveToolClosure)
	}
	return policies
}

func appendPolicy(policies []CompactPolicy, policy CompactPolicy) []CompactPolicy {
	for _, p := range policies {
		if p == policy {
			return policies
		}
	}
	return append(policies, policy)
}

func isToolExchangeRecord(record historyfrag.HistoryRecord) bool {
	mm := record.ModelMessage
	if strings.EqualFold(strings.TrimSpace(mm.Role), "tool") {
		return true
	}
	if len(mm.ToolCalls) > 0 {
		return true
	}
	for _, p := range parseEntryParts(mm.Content) {
		if strings.Contains(p.Type, "tool-call") ||
			strings.Contains(p.Type, "tool_call") ||
			strings.Contains(p.Type, "tool-result") ||
			strings.Contains(p.Type, "tool_result") {
			return true
		}
	}
	return false
}

func isAskUserRecord(record historyfrag.HistoryRecord) bool {
	mm := record.ModelMessage
	if isAskUserToolName(mm.Name) {
		return true
	}
	for _, call := range mm.ToolCalls {
		if isAskUserToolName(call.Function.Name) {
			return true
		}
	}
	for _, p := range parseEntryParts(mm.Content) {
		if isAskUserToolName(p.ToolName) {
			return true
		}
	}
	return false
}

func isAskUserToolName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), userinput.ToolNameAskUser)
}

type usagePayload struct {
	OutputTokens      *int `json:"outputTokens"`
	OutputTokensSnake *int `json:"output_tokens"`
}

func estimateItemTokens(item CompactionCandidate) int {
	if len(item.RawUsage) > 0 {
		var u usagePayload
		if json.Unmarshal(item.RawUsage, &u) == nil {
			if u.OutputTokens != nil && *u.OutputTokens > 0 {
				return *u.OutputTokens
			}
			if u.OutputTokensSnake != nil && *u.OutputTokensSnake > 0 {
				return *u.OutputTokensSnake
			}
		}
	}
	return len(item.RawContent) / 4
}

func estimateCompactPromptTokens(item CompactionCandidate) int {
	tokens := estimateItemTokens(item)
	if header := renderEntryHeader(item.Record); header != "" {
		tokens += estimateBytesAsTokens(header)
	}
	return tokens
}

func estimateBytesAsTokens(value string) int {
	if value == "" {
		return 0
	}
	return (len(value) + 3) / 4
}

// splitByRatio splits items so that roughly the first ratio% (by token weight)
// are returned for compaction, and the rest are kept as-is.
func splitByRatio(items []CompactionCandidate, totalInputTokens, ratio int) []CompactionCandidate {
	if ratio <= 0 || totalInputTokens <= 0 || len(items) == 0 {
		return nil
	}
	if ratio >= 100 {
		return guardedCompactionItems(items, len(items))
	}

	keepTokens := totalInputTokens * (100 - ratio) / 100
	if keepTokens <= 0 {
		return guardedCompactionItems(items, len(items))
	}

	accumulated := 0
	cutoff := len(items)
	for i := len(items) - 1; i >= 0; i-- {
		accumulated += estimateItemTokens(items[i])
		if accumulated >= keepTokens {
			cutoff = i + 1
			break
		}
	}

	if cutoff <= 0 {
		return nil
	}
	return guardedCompactionItems(items, cutoff)
}

// splitByTarget returns the oldest items to compact so that the remaining newest
// items fit within targetTokens. Used by synchronous compaction.
func splitByTarget(items []CompactionCandidate, targetTokens int) []CompactionCandidate {
	if targetTokens <= 0 || len(items) == 0 {
		return nil
	}
	accumulated := 0
	cutoff := 0
	for i := len(items) - 1; i >= 0; i-- {
		accumulated += estimateItemTokens(items[i])
		if accumulated > targetTokens {
			cutoff = i + 1
			break
		}
	}
	if cutoff <= 0 {
		return nil
	}
	return guardedCompactionItems(items, cutoff)
}

func guardedCompactionItems(items []CompactionCandidate, cutoff int) []CompactionCandidate {
	if cutoff <= 0 || len(items) == 0 {
		return nil
	}
	protectedStart := firstPolicyStart(items, CompactPolicyPreserveRecent)
	if protectedStart <= 0 {
		return guardedCurrentTurnCompactionItems(items, cutoff)
	}
	if cutoff > protectedStart {
		cutoff = protectedStart
	}
	cutoff = adjustForToolBoundary(items, cutoff)
	if cutoff > protectedStart {
		return nil
	}
	if cutoff <= 0 {
		return nil
	}
	return firstCompactableRun(items[:cutoff])
}

func guardedCurrentTurnCompactionItems(items []CompactionCandidate, cutoff int) []CompactionCandidate {
	if len(items) <= 1 {
		return nil
	}
	protectedTailStart := firstPolicyStartAfter(items, CompactPolicyPreserveRecent, 1)
	if protectedTailStart <= 1 {
		return nil
	}
	if cutoff > protectedTailStart {
		cutoff = protectedTailStart
	}
	if cutoff <= 1 {
		return nil
	}
	cutoff = adjustForToolBoundary(items, cutoff)
	if cutoff > protectedTailStart {
		return nil
	}
	return firstCompactableRun(items[1:cutoff])
}

// firstCompactableRun returns the first maximal run of non-must-keep items in
// the span. Must-keep rows (e.g. ask_user) stay in raw history and split the
// span into islands; compacting only the first run keeps every compaction log
// covering a contiguous history range, so the read path replaces it in place
// without reordering rows across a must-keep island under a shared compact_id.
// A must-keep island at the head is skipped rather than starving the run behind
// it, and later runs are picked up on subsequent passes.
func firstCompactableRun(items []CompactionCandidate) []CompactionCandidate {
	start := 0
	for start < len(items) && items[start].HasPolicy(CompactPolicyMustKeep) {
		start++
	}
	end := start
	for end < len(items) && !items[end].HasPolicy(CompactPolicyMustKeep) {
		end++
	}
	if end == start {
		return nil
	}
	return items[start:end]
}

func firstPolicyStart(items []CompactionCandidate, policy CompactPolicy) int {
	return firstPolicyStartAfter(items, policy, 0)
}

func firstPolicyStartAfter(items []CompactionCandidate, policy CompactPolicy, start int) int {
	if start < 0 {
		start = 0
	}
	for i, item := range items {
		if i < start {
			continue
		}
		if item.HasPolicy(policy) {
			return i
		}
	}
	return len(items)
}

// adjustForToolBoundary moves the compact/keep cutoff forward so the kept
// (newest) side never begins with an orphan tool result whose tool call is
// being compacted. Tool results are pulled into the compact set so each tool
// exchange stays intact on one side of the boundary.
func adjustForToolBoundary(items []CompactionCandidate, cutoff int) int {
	for cutoff > 0 && cutoff < len(items) && isToolClosureResultItem(items[cutoff]) {
		cutoff++
	}
	return cutoff
}

func isToolClosureResultItem(item CompactionCandidate) bool {
	return item.HasPolicy(CompactPolicyPreserveToolClosure) && isToolResultItem(item)
}

func isToolResultItem(item CompactionCandidate) bool {
	return strings.EqualFold(strings.TrimSpace(item.Record.ModelMessage.Role), "tool")
}

// buildEntriesAndIDs renders the summarizer entries and the ids to mark
// compacted, grouped by tool exchange (see toolExchangeGroups). A group is
// emitted only when every row in it renders non-empty; an incomplete group —
// e.g. a reasoning-only message, or a renderable tool call whose result renders
// empty — is withheld from BOTH the prompt and the marked ids. That keeps
// entries and ids aligned: the summarizer never sees content that would remain
// in raw history (which would duplicate it), and marking never strands an
// orphan tool row after the summary replaces the rest.
func buildEntriesAndIDs(items []CompactionCandidate) ([]messageEntry, []pgtype.UUID) {
	rendered := make([]string, len(items))
	renderedOK := make([]bool, len(items))
	for i, it := range items {
		content := renderCandidateEntry(it.Record)
		if strings.TrimSpace(content) == "" {
			continue
		}
		rendered[i] = content
		renderedOK[i] = true
	}

	entries := make([]messageEntry, 0, len(items))
	ids := make([]pgtype.UUID, 0, len(items))
	for _, group := range toolExchangeGroups(items) {
		complete := true
		for _, idx := range group {
			if !renderedOK[idx] {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		for _, idx := range group {
			entries = append(entries, messageEntry{
				Role:    items[idx].Record.ModelMessage.Role,
				Content: rendered[idx],
			})
			ids = append(ids, items[idx].ID)
		}
	}
	return entries, ids
}

// trimCompactMessages trims the compaction input from the tail (oldest) so the
// total estimated tokens stay within maxTokens.
func trimCompactMessages(items []CompactionCandidate, maxTokens int) []CompactionCandidate {
	if len(items) == 0 || maxTokens <= 0 {
		return items
	}
	total := 0
	for _, it := range items {
		total += estimateCompactPromptTokens(it)
	}
	if total <= maxTokens {
		return items
	}
	accumulated := 0
	cutoff := len(items)
	for i := len(items) - 1; i >= 0; i-- {
		accumulated += estimateCompactPromptTokens(items[i])
		if accumulated > maxTokens {
			cutoff = i + 1
			break
		}
	}
	cutoff = adjustForToolBoundary(items, cutoff)
	if cutoff >= len(items) {
		return items
	}
	return items[cutoff:]
}
