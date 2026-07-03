package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sqlTemplateExecutor struct {
	cfg     Config
	pool    *pgxpool.Pool
	queries QuerySet
}

func newSQLTemplateExecutor(cfg Config, pool *pgxpool.Pool, queries QuerySet) (*sqlTemplateExecutor, error) {
	if len(queries) == 0 {
		return nil, errors.New("sql runner requires loaded SQL templates")
	}
	return &sqlTemplateExecutor{
		cfg:     cfg,
		pool:    pool,
		queries: queries,
	}, nil
}

func (*sqlTemplateExecutor) querySource() string {
	return querySourceSQLTemplate
}

func (*sqlTemplateExecutor) scanMode() string {
	return scanModeRowDrainOnly
}

func (e *sqlTemplateExecutor) execQuery(ctx context.Context, queryName string, s SessionSeed, rng *rand.Rand) (int64, error) {
	sql, ok := e.queries[queryName]
	if !ok {
		return 0, fmt.Errorf("query %s not loaded", queryName)
	}
	headID := selectedHead(e.cfg, s, rng)
	var args []any
	switch queryName {
	case queryChatPageUI:
		return e.execComposite(ctx, []string{queryLatestPage, queryTurnSiblings, queryApprovalToolCalls, queryUserInputToolCalls}, s, rng, true)
	case queryLocateWindow:
		return e.execComposite(ctx, []string{queryExternalLookup, queryBeforePage, queryAfterPage}, s, rng, false)
	case queryApprovalResolve:
		return e.execComposite(ctx, []string{queryApprovalLatest, queryApprovalShortID}, s, rng, true)
	case queryUserInputResolve:
		return e.execComposite(ctx, []string{queryUserInputLatest, queryUserInputShortID}, s, rng, true)
	case querySSELiveFilter:
		return e.execQuery(ctx, queryTurnAncestor, s, rng)
	case queryLatestPage:
		args = []any{s.SessionID, nilUUID(headID), e.cfg.Workload.PageSize}
	case queryBeforePage:
		cursorID, cursorTime := selectedCursorForHead(s, headID, rng)
		args = []any{s.SessionID, nilUUID(headID), nilUUID(cursorID), cursorTime, e.cfg.Workload.PageSize}
	case queryAfterPage:
		cursorID, cursorTime := selectedCursorForHead(s, headID, rng)
		args = []any{s.SessionID, nilUUID(headID), nilUUID(cursorID), cursorTime, e.cfg.Workload.PageSize}
	case queryExternalLookup:
		args = []any{s.SessionID, nilUUID(headID), selectedExternalMessageID(s, rng)}
	case queryTurnGraph:
		args = []any{s.SessionID}
	case queryHeadResolve:
		args = []any{s.SessionID, variantResolveTarget(queryName, s)}
	case queryTurnSiblings:
		args = []any{s.SessionID, variantPageTurnIDs(s)}
	case queryTurnPath:
		args = []any{variantPathHead(e.cfg, s, rng)}
	case queryTurnAncestor:
		ancestor, descendant := variantAncestorArgs(queryName, s, rng)
		args = []any{ancestor, descendant}
	case queryApprovalToolCalls:
		toolCallID := requireText(queryName, s.ApprovalToolCallID)
		turnID := requireUUID(queryName, s.ApprovalTurnID)
		if argErr, ok := toolCallID.(queryArgError); ok {
			return 0, argErr
		}
		if argErr, ok := turnID.(queryArgError); ok {
			return 0, argErr
		}
		args = []any{s.BotID, s.SessionID, []string{toolCallID.(string)}, []uuid.UUID{turnID.(uuid.UUID)}}
	case queryApprovalPendingList, queryApprovalGraphList, queryApprovalLatest:
		args = []any{s.BotID, s.SessionID}
	case queryApprovalShortID:
		args = []any{s.BotID, s.SessionID, requireShortID(queryName, s.ApprovalShortID)}
	case queryApprovalVisibleRequest:
		args = []any{requireUUID(queryName, s.ApprovalRequestID), s.BotID, s.SessionID}
	case queryApprovalBaseHeadRequest:
		args = []any{requireUUID(queryName, s.ApprovalBaseReqID), s.BotID, s.SessionID, requireUUID(queryName, selectedHeadForBase(s))}
	case queryApprovalReplyMessage:
		args = []any{s.BotID, s.SessionID, requireText(queryName, s.ApprovalPromptID)}
	case queryUserInputToolCalls:
		toolCallID := requireText(queryName, s.UserInputToolCallID)
		turnID := requireUUID(queryName, s.UserInputTurnID)
		if argErr, ok := toolCallID.(queryArgError); ok {
			return 0, argErr
		}
		if argErr, ok := turnID.(queryArgError); ok {
			return 0, argErr
		}
		args = []any{s.BotID, s.SessionID, []string{toolCallID.(string)}, []uuid.UUID{turnID.(uuid.UUID)}}
	case queryUserInputPendingList, queryUserInputGraphList, queryUserInputLatest:
		args = []any{s.BotID, s.SessionID}
	case queryUserInputShortID:
		args = []any{s.BotID, s.SessionID, requireShortID(queryName, s.UserInputShortID)}
	case queryUserInputVisibleRequest:
		args = []any{requireUUID(queryName, s.UserInputRequestID), s.BotID, s.SessionID}
	case queryUserInputBaseHeadRequest:
		args = []any{requireUUID(queryName, s.UserInputBaseReqID), s.BotID, s.SessionID, requireUUID(queryName, selectedHeadForBase(s))}
	case queryUserInputReplyMessage:
		args = []any{s.BotID, s.SessionID, requireText(queryName, s.UserInputPromptID)}
	default:
		return 0, fmt.Errorf("unknown query %s", queryName)
	}
	for _, arg := range args {
		if argErr, ok := arg.(queryArgError); ok {
			return 0, argErr
		}
	}
	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func (e *sqlTemplateExecutor) execComposite(ctx context.Context, parts []string, s SessionSeed, rng *rand.Rand, skipMissing bool) (int64, error) {
	var total int64
	for _, name := range parts {
		if skipMissing && compositePartMissing(name, s) {
			continue
		}
		rows, err := e.execQuery(ctx, name, s, rng)
		total += rows
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func compositePartMissing(name string, s SessionSeed) bool {
	switch name {
	case queryApprovalToolCalls:
		return s.ApprovalToolCallID == "" || s.ApprovalTurnID == uuid.Nil
	case queryUserInputToolCalls:
		return s.UserInputToolCallID == "" || s.UserInputTurnID == uuid.Nil
	case queryApprovalShortID:
		return s.ApprovalShortID <= 0
	case queryUserInputShortID:
		return s.UserInputShortID <= 0
	default:
		return false
	}
}
