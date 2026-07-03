package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	queryChatPageUI               = "chat_page_ui"
	queryLocateWindow             = "locate_window"
	queryApprovalResolve          = "approval_resolve"
	queryUserInputResolve         = "user_input_resolve"
	querySSELiveFilter            = "sse_live_filter"
	queryLatestPage               = "latest_page"
	queryBeforePage               = "before_page"
	queryAfterPage                = "after_page"
	queryExternalLookup           = "external_lookup"
	queryTurnGraph                = "turn_graph"
	queryHeadResolve              = "head_resolve"
	queryTurnSiblings             = "turn_siblings"
	queryTurnPath                 = "turn_path"
	queryTurnAncestor             = "turn_ancestor"
	queryApprovalToolCalls        = "approval_tool_calls"
	queryApprovalPendingList      = "approval_pending_list"
	queryApprovalGraphList        = "approval_graph_list"
	queryApprovalLatest           = "approval_latest"
	queryApprovalShortID          = "approval_short_id"
	queryApprovalVisibleRequest   = "approval_visible_request"
	queryApprovalBaseHeadRequest  = "approval_base_head_request"
	queryApprovalReplyMessage     = "approval_reply_message"
	queryUserInputToolCalls       = "user_input_tool_calls"
	queryUserInputPendingList     = "user_input_pending_list"
	queryUserInputGraphList       = "user_input_graph_list"
	queryUserInputLatest          = "user_input_latest"
	queryUserInputShortID         = "user_input_short_id"
	queryUserInputVisibleRequest  = "user_input_visible_request"
	queryUserInputBaseHeadRequest = "user_input_base_head_request"
	queryUserInputReplyMessage    = "user_input_reply_message"
)

type QueryDefinition struct {
	Name       string
	SourceFile string
	SourceName string
	Args       []string
}

var queryDefinitions = []QueryDefinition{
	{Name: queryLatestPage, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ListMessagesLatestBySession", Args: []string{"session_id", "head_turn_id", "max_count"}},
	{Name: queryBeforePage, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ListMessagesBeforeBySession", Args: []string{"session_id", "head_turn_id", "before_id", "created_at", "max_count"}},
	{Name: queryAfterPage, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ListMessagesAfterBySession", Args: []string{"session_id", "head_turn_id", "after_id", "created_at", "max_count"}},
	{Name: queryExternalLookup, SourceFile: "db/postgres/queries/messages.sql", SourceName: "GetMessageByExternalIDBySession", Args: []string{"session_id", "head_turn_id", "external_message_id"}},
	{Name: queryTurnGraph, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ListSessionTurnGraphTurns", Args: []string{"session_id"}},
	{Name: queryHeadResolve, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ResolveSessionTurnHead", Args: []string{"session_id", "target_turn_id"}},
	{Name: queryTurnSiblings, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ListSessionTurnSiblings", Args: []string{"session_id", "turn_ids"}},
	{Name: queryTurnPath, SourceFile: "db/postgres/queries/messages.sql", SourceName: "ListSessionTurnPathIDs", Args: []string{"head_turn_id"}},
	{Name: queryTurnAncestor, SourceFile: "db/postgres/queries/messages.sql", SourceName: "GetSessionTurnAncestorMatch", Args: []string{"ancestor_turn_id", "turn_id"}},
	{Name: queryApprovalToolCalls, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "ListToolApprovalsBySessionToolCalls", Args: []string{"bot_id", "session_id", "tool_call_ids", "turn_ids"}},
	{Name: queryApprovalPendingList, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "ListPendingToolApprovalsBySession", Args: []string{"bot_id", "session_id"}},
	{Name: queryApprovalGraphList, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "ListToolApprovalsBySessionTurnGraph", Args: []string{"bot_id", "session_id"}},
	{Name: queryApprovalLatest, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "GetLatestPendingToolApprovalBySession", Args: []string{"bot_id", "session_id"}},
	{Name: queryApprovalShortID, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "GetPendingToolApprovalBySessionShortID", Args: []string{"bot_id", "session_id", "short_id"}},
	{Name: queryApprovalVisibleRequest, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "GetPendingToolApprovalByVisibleRequestID", Args: []string{"id", "bot_id", "session_id"}},
	{Name: queryApprovalBaseHeadRequest, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "GetPendingToolApprovalByBaseHeadRequestID", Args: []string{"id", "bot_id", "session_id", "base_head_turn_id"}},
	{Name: queryApprovalReplyMessage, SourceFile: "db/postgres/queries/tool_approval.sql", SourceName: "GetPendingToolApprovalByReplyMessage", Args: []string{"bot_id", "session_id", "prompt_external_message_id"}},
	{Name: queryUserInputToolCalls, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "ListUserInputsBySessionToolCalls", Args: []string{"bot_id", "session_id", "tool_call_ids", "turn_ids"}},
	{Name: queryUserInputPendingList, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "ListPendingUserInputsBySession", Args: []string{"bot_id", "session_id"}},
	{Name: queryUserInputGraphList, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "ListUserInputsBySessionTurnGraph", Args: []string{"bot_id", "session_id"}},
	{Name: queryUserInputLatest, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "GetLatestPendingUserInputBySession", Args: []string{"bot_id", "session_id"}},
	{Name: queryUserInputShortID, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "GetPendingUserInputBySessionShortID", Args: []string{"bot_id", "session_id", "short_id"}},
	{Name: queryUserInputVisibleRequest, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "GetPendingUserInputByVisibleRequestID", Args: []string{"id", "bot_id", "session_id"}},
	{Name: queryUserInputBaseHeadRequest, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "GetPendingUserInputByBaseHeadRequestID", Args: []string{"id", "bot_id", "session_id", "base_head_turn_id"}},
	{Name: queryUserInputReplyMessage, SourceFile: "db/postgres/queries/user_input.sql", SourceName: "GetPendingUserInputByReplyMessage", Args: []string{"bot_id", "session_id", "prompt_external_message_id"}},
}

var (
	knownQueries   = queryNames(queryDefinitions)
	knownScenarios = append([]string{
		queryChatPageUI,
		queryLocateWindow,
		queryApprovalResolve,
		queryUserInputResolve,
		querySSELiveFilter,
	}, knownQueries...)
)

type QuerySet map[string]string

type WeightedQuery struct {
	Name       string
	Cumulative int
}

func queryNames(defs []QueryDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

func queryDefinition(name string) (QueryDefinition, bool) {
	for _, def := range queryDefinitions {
		if def.Name == name {
			return def, true
		}
	}
	return QueryDefinition{}, false
}

func loadQueries(dir string) (QuerySet, error) {
	queries := make(QuerySet, len(knownQueries))
	for _, name := range knownQueries {
		path := filepath.Join(dir, name+".sql")
		// #nosec G304 -- benchmark SQL templates are intentionally loaded from a user-provided directory.
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read query %s: %w", name, err)
		}
		sql := strings.TrimSpace(string(raw))
		if sql == "" {
			return nil, fmt.Errorf("query %s is empty", name)
		}
		queries[name] = sql
	}
	return queries, nil
}

func isKnownQuery(name string) bool {
	for _, known := range knownScenarios {
		if name == known {
			return true
		}
	}
	return false
}

func normalizeWeights(weights map[string]int) ([]WeightedQuery, error) {
	if len(weights) == 0 {
		return nil, errors.New("workload.query_weights must not be empty")
	}
	total := 0
	normalized := make([]WeightedQuery, 0, len(weights))
	for _, name := range knownScenarios {
		weight := weights[name]
		if weight <= 0 {
			continue
		}
		total += weight
		normalized = append(normalized, WeightedQuery{Name: name, Cumulative: total})
	}
	if total <= 0 {
		return nil, errors.New("workload.query_weights must include at least one positive known query")
	}
	for name, weight := range weights {
		if weight > 0 && !isKnownQuery(name) {
			return nil, fmt.Errorf("unknown weighted query %q", name)
		}
	}
	return normalized, nil
}

func pickWeightedQuery(weighted []WeightedQuery, n int) string {
	if len(weighted) == 0 {
		return queryLatestPage
	}
	total := weighted[len(weighted)-1].Cumulative
	if total <= 0 {
		return weighted[0].Name
	}
	slot := n % total
	for _, item := range weighted {
		if slot < item.Cumulative {
			return item.Name
		}
	}
	return weighted[len(weighted)-1].Name
}
