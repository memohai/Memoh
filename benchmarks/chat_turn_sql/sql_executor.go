package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

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
	case queryLatestPage:
		args = []any{s.SessionID, nilUUID(headID), e.cfg.Workload.PageSize}
	case queryBeforePage:
		cursorID, cursorTime := selectedCursor(s, rng)
		args = []any{s.SessionID, nilUUID(headID), nilUUID(cursorID), cursorTime, e.cfg.Workload.PageSize}
	case queryAfterPage:
		cursorID, cursorTime := selectedCursor(s, rng)
		args = []any{s.SessionID, nilUUID(headID), nilUUID(cursorID), cursorTime, e.cfg.Workload.PageSize}
	case queryExternalLookup:
		args = []any{s.SessionID, nilUUID(headID), s.ExternalMessageID}
	case queryTurnGraph, queryGraphMetadata:
		args = []any{s.SessionID}
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
