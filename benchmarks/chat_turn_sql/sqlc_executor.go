package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	postgresqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type sqlcExecutor struct {
	cfg     Config
	queries *postgresqlc.Queries
}

func newSQLCExecutor(cfg Config, pool *pgxpool.Pool) *sqlcExecutor {
	return &sqlcExecutor{
		cfg:     cfg,
		queries: postgresqlc.New(pool),
	}
}

func (*sqlcExecutor) querySource() string {
	return querySourceGeneratedSQLC
}

func (*sqlcExecutor) scanMode() string {
	return scanModeSQLCStructScan
}

func (e *sqlcExecutor) execQuery(ctx context.Context, queryName string, s SessionSeed, rng *rand.Rand) (int64, error) {
	headID := selectedHead(e.cfg, s, rng)
	maxCount := pageSizeInt32(e.cfg)
	switch queryName {
	case queryChatPageUI:
		return e.execChatPageUI(ctx, s, rng)
	case queryLocateWindow:
		return e.execLocateWindow(ctx, s, rng)
	case queryApprovalResolve:
		return e.execApprovalResolve(ctx, s)
	case queryUserInputResolve:
		return e.execUserInputResolve(ctx, s)
	case querySSELiveFilter:
		return e.execQuery(ctx, queryTurnAncestor, s, rng)
	case queryLatestPage:
		items, err := e.queries.ListMessagesLatestBySession(ctx, postgresqlc.ListMessagesLatestBySessionParams{
			MaxCount:   maxCount,
			HeadTurnID: pgUUID(headID),
			SessionID:  pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryBeforePage:
		cursorID, cursorTime := selectedCursorForHead(s, headID, rng)
		items, err := e.queries.ListMessagesBeforeBySession(ctx, postgresqlc.ListMessagesBeforeBySessionParams{
			BeforeID:   pgUUID(cursorID),
			CreatedAt:  pgTimestamptz(cursorTime),
			MaxCount:   maxCount,
			HeadTurnID: pgUUID(headID),
			SessionID:  pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryAfterPage:
		cursorID, cursorTime := selectedCursorForHead(s, headID, rng)
		items, err := e.queries.ListMessagesAfterBySession(ctx, postgresqlc.ListMessagesAfterBySessionParams{
			AfterID:    pgUUID(cursorID),
			CreatedAt:  pgTimestamptz(cursorTime),
			MaxCount:   maxCount,
			HeadTurnID: pgUUID(headID),
			SessionID:  pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryExternalLookup:
		externalID := selectedExternalMessageID(s, rng)
		_, err := e.queries.GetMessageByExternalIDBySession(ctx, postgresqlc.GetMessageByExternalIDBySessionParams{
			ExternalMessageID: pgText(externalID),
			HeadTurnID:        pgUUID(headID),
			SessionID:         pgUUID(s.SessionID),
		})
		return rowsForOne(err)
	case queryTurnGraph:
		items, err := e.queries.ListSessionTurnGraphTurns(ctx, pgUUID(s.SessionID))
		return int64(len(items)), err
	case queryHeadResolve:
		target := variantResolveTarget(queryName, s)
		if argErr, ok := target.(queryArgError); ok {
			return 0, argErr
		}
		_, err := e.queries.ResolveSessionTurnHead(ctx, postgresqlc.ResolveSessionTurnHeadParams{
			SessionID:    pgUUID(s.SessionID),
			TargetTurnID: pgUUID(target.(uuid.UUID)),
		})
		return rowsForOne(err)
	case queryTurnSiblings:
		items, err := e.queries.ListSessionTurnSiblings(ctx, postgresqlc.ListSessionTurnSiblingsParams{
			SessionID: pgUUID(s.SessionID),
			TurnIds:   pgUUIDs(variantPageTurnIDs(s)),
		})
		return int64(len(items)), err
	case queryTurnPath:
		items, err := e.queries.ListSessionTurnPathIDs(ctx, pgUUID(variantPathHead(e.cfg, s, rng)))
		return int64(len(items)), err
	case queryTurnAncestor:
		ancestor, descendant := variantAncestorArgs(queryName, s, rng)
		if argErr, ok := ancestor.(queryArgError); ok {
			return 0, argErr
		}
		if argErr, ok := descendant.(queryArgError); ok {
			return 0, argErr
		}
		_, err := e.queries.GetSessionTurnAncestorMatch(ctx, postgresqlc.GetSessionTurnAncestorMatchParams{
			AncestorTurnID: pgUUID(ancestor.(uuid.UUID)),
			TurnID:         pgUUID(descendant.(uuid.UUID)),
		})
		return rowsForOptionalOne(err)
	case queryApprovalToolCalls:
		toolCallID, err := requiredText(queryName, s.ApprovalToolCallID)
		if err != nil {
			return 0, err
		}
		turnID, err := requiredUUID(queryName, s.ApprovalTurnID)
		if err != nil {
			return 0, err
		}
		items, err := e.queries.ListToolApprovalsBySessionToolCalls(ctx, postgresqlc.ListToolApprovalsBySessionToolCallsParams{
			BotID:       pgUUID(s.BotID),
			SessionID:   pgUUID(s.SessionID),
			ToolCallIds: []string{toolCallID},
			TurnIds:     pgUUIDs([]uuid.UUID{turnID}),
		})
		return int64(len(items)), err
	case queryApprovalPendingList:
		items, err := e.queries.ListPendingToolApprovalsBySession(ctx, postgresqlc.ListPendingToolApprovalsBySessionParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryApprovalGraphList:
		items, err := e.queries.ListToolApprovalsBySessionTurnGraph(ctx, postgresqlc.ListToolApprovalsBySessionTurnGraphParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryApprovalLatest:
		_, err := e.queries.GetLatestPendingToolApprovalBySession(ctx, postgresqlc.GetLatestPendingToolApprovalBySessionParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return rowsForOne(err)
	case queryApprovalShortID:
		shortID, err := requiredShortID(queryName, s.ApprovalShortID)
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingToolApprovalBySessionShortID(ctx, postgresqlc.GetPendingToolApprovalBySessionShortIDParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
			ShortID:   shortID,
		})
		return rowsForOne(err)
	case queryApprovalVisibleRequest:
		requestID, err := requiredUUID(queryName, s.ApprovalRequestID)
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingToolApprovalByVisibleRequestID(ctx, postgresqlc.GetPendingToolApprovalByVisibleRequestIDParams{
			ID:        pgUUID(requestID),
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return rowsForOne(err)
	case queryApprovalBaseHeadRequest:
		requestID, err := requiredUUID(queryName, s.ApprovalBaseReqID)
		if err != nil {
			return 0, err
		}
		baseHeadID, err := requiredUUID(queryName, selectedHeadForBase(s))
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingToolApprovalByBaseHeadRequestID(ctx, postgresqlc.GetPendingToolApprovalByBaseHeadRequestIDParams{
			ID:             pgUUID(requestID),
			BotID:          pgUUID(s.BotID),
			SessionID:      pgUUID(s.SessionID),
			BaseHeadTurnID: pgUUID(baseHeadID),
		})
		return rowsForOne(err)
	case queryApprovalReplyMessage:
		promptID, err := requiredText(queryName, s.ApprovalPromptID)
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingToolApprovalByReplyMessage(ctx, postgresqlc.GetPendingToolApprovalByReplyMessageParams{
			BotID:                   pgUUID(s.BotID),
			SessionID:               pgUUID(s.SessionID),
			PromptExternalMessageID: promptID,
		})
		return rowsForOne(err)
	case queryUserInputToolCalls:
		toolCallID, err := requiredText(queryName, s.UserInputToolCallID)
		if err != nil {
			return 0, err
		}
		turnID, err := requiredUUID(queryName, s.UserInputTurnID)
		if err != nil {
			return 0, err
		}
		items, err := e.queries.ListUserInputsBySessionToolCalls(ctx, postgresqlc.ListUserInputsBySessionToolCallsParams{
			BotID:       pgUUID(s.BotID),
			SessionID:   pgUUID(s.SessionID),
			ToolCallIds: []string{toolCallID},
			TurnIds:     pgUUIDs([]uuid.UUID{turnID}),
		})
		return int64(len(items)), err
	case queryUserInputPendingList:
		items, err := e.queries.ListPendingUserInputsBySession(ctx, postgresqlc.ListPendingUserInputsBySessionParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryUserInputGraphList:
		items, err := e.queries.ListUserInputsBySessionTurnGraph(ctx, postgresqlc.ListUserInputsBySessionTurnGraphParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryUserInputLatest:
		_, err := e.queries.GetLatestPendingUserInputBySession(ctx, postgresqlc.GetLatestPendingUserInputBySessionParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return rowsForOne(err)
	case queryUserInputShortID:
		shortID, err := requiredShortID(queryName, s.UserInputShortID)
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingUserInputBySessionShortID(ctx, postgresqlc.GetPendingUserInputBySessionShortIDParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
			ShortID:   shortID,
		})
		return rowsForOne(err)
	case queryUserInputVisibleRequest:
		requestID, err := requiredUUID(queryName, s.UserInputRequestID)
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingUserInputByVisibleRequestID(ctx, postgresqlc.GetPendingUserInputByVisibleRequestIDParams{
			ID:        pgUUID(requestID),
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
		})
		return rowsForOne(err)
	case queryUserInputBaseHeadRequest:
		requestID, err := requiredUUID(queryName, s.UserInputBaseReqID)
		if err != nil {
			return 0, err
		}
		baseHeadID, err := requiredUUID(queryName, selectedHeadForBase(s))
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingUserInputByBaseHeadRequestID(ctx, postgresqlc.GetPendingUserInputByBaseHeadRequestIDParams{
			ID:             pgUUID(requestID),
			BotID:          pgUUID(s.BotID),
			SessionID:      pgUUID(s.SessionID),
			BaseHeadTurnID: pgUUID(baseHeadID),
		})
		return rowsForOne(err)
	case queryUserInputReplyMessage:
		promptID, err := requiredText(queryName, s.UserInputPromptID)
		if err != nil {
			return 0, err
		}
		_, err = e.queries.GetPendingUserInputByReplyMessage(ctx, postgresqlc.GetPendingUserInputByReplyMessageParams{
			BotID:                   pgUUID(s.BotID),
			SessionID:               pgUUID(s.SessionID),
			PromptExternalMessageID: promptID,
		})
		return rowsForOne(err)
	default:
		return 0, fmt.Errorf("unknown query %s", queryName)
	}
}

func (e *sqlcExecutor) execChatPageUI(ctx context.Context, s SessionSeed, rng *rand.Rand) (int64, error) {
	rows, err := e.execQuery(ctx, queryLatestPage, s, rng)
	if err != nil {
		return rows, err
	}
	extra, err := e.execQuery(ctx, queryTurnSiblings, s, rng)
	rows += extra
	if err != nil {
		return rows, err
	}
	if s.ApprovalToolCallID != "" && s.ApprovalTurnID != uuid.Nil {
		extra, err = e.execQuery(ctx, queryApprovalToolCalls, s, rng)
		rows += extra
		if err != nil {
			return rows, err
		}
	}
	if s.UserInputToolCallID != "" && s.UserInputTurnID != uuid.Nil {
		extra, err = e.execQuery(ctx, queryUserInputToolCalls, s, rng)
		rows += extra
		if err != nil {
			return rows, err
		}
	}
	return rows, nil
}

func (e *sqlcExecutor) execLocateWindow(ctx context.Context, s SessionSeed, rng *rand.Rand) (int64, error) {
	headID := selectedHead(e.cfg, s, rng)
	externalID := selectedExternalMessageID(s, rng)
	_, err := e.queries.GetMessageByExternalIDBySession(ctx, postgresqlc.GetMessageByExternalIDBySessionParams{
		ExternalMessageID: pgText(externalID),
		HeadTurnID:        pgUUID(headID),
		SessionID:         pgUUID(s.SessionID),
	})
	rows, err := rowsForOne(err)
	if err != nil {
		return rows, err
	}
	cursorID, cursorTime := selectedCursorForHead(s, headID, rng)
	before, err := e.queries.ListMessagesBeforeBySession(ctx, postgresqlc.ListMessagesBeforeBySessionParams{
		BeforeID:   pgUUID(cursorID),
		CreatedAt:  pgTimestamptz(cursorTime),
		MaxCount:   pageSizeInt32(e.cfg),
		HeadTurnID: pgUUID(headID),
		SessionID:  pgUUID(s.SessionID),
	})
	extra := int64(len(before))
	rows += extra
	if err != nil {
		return rows, err
	}
	after, err := e.queries.ListMessagesAfterBySession(ctx, postgresqlc.ListMessagesAfterBySessionParams{
		AfterID:    pgUUID(cursorID),
		CreatedAt:  pgTimestamptz(cursorTime),
		MaxCount:   pageSizeInt32(e.cfg),
		HeadTurnID: pgUUID(headID),
		SessionID:  pgUUID(s.SessionID),
	})
	extra = int64(len(after))
	rows += extra
	return rows, err
}

func (e *sqlcExecutor) execApprovalResolve(ctx context.Context, s SessionSeed) (int64, error) {
	_, err := e.queries.GetLatestPendingToolApprovalBySession(ctx, postgresqlc.GetLatestPendingToolApprovalBySessionParams{
		BotID:     pgUUID(s.BotID),
		SessionID: pgUUID(s.SessionID),
	})
	rows, err := rowsForOne(err)
	if err != nil {
		return rows, err
	}
	if s.ApprovalShortID > 0 {
		_, err = e.queries.GetPendingToolApprovalBySessionShortID(ctx, postgresqlc.GetPendingToolApprovalBySessionShortIDParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
			ShortID:   s.ApprovalShortID,
		})
		extra, err := rowsForOne(err)
		rows += extra
		if err != nil {
			return rows, err
		}
	}
	return rows, nil
}

func (e *sqlcExecutor) execUserInputResolve(ctx context.Context, s SessionSeed) (int64, error) {
	_, err := e.queries.GetLatestPendingUserInputBySession(ctx, postgresqlc.GetLatestPendingUserInputBySessionParams{
		BotID:     pgUUID(s.BotID),
		SessionID: pgUUID(s.SessionID),
	})
	rows, err := rowsForOne(err)
	if err != nil {
		return rows, err
	}
	if s.UserInputShortID > 0 {
		_, err = e.queries.GetPendingUserInputBySessionShortID(ctx, postgresqlc.GetPendingUserInputBySessionShortIDParams{
			BotID:     pgUUID(s.BotID),
			SessionID: pgUUID(s.SessionID),
			ShortID:   s.UserInputShortID,
		})
		extra, err := rowsForOne(err)
		rows += extra
		if err != nil {
			return rows, err
		}
	}
	return rows, nil
}

func pageSizeInt32(cfg Config) int32 {
	// #nosec G115 -- cfg.validate rejects page sizes above math.MaxInt32.
	return int32(cfg.Workload.PageSize)
}

func rowsForOne(err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func rowsForOptionalOne(err error) (int64, error) {
	if err == nil {
		return 1, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return 0, err
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgUUIDs(ids []uuid.UUID) []pgtype.UUID {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		out = append(out, pgUUID(id))
	}
	return out
}

func pgText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func pgTimestamptz(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func requiredShortID(queryName string, v int32) (int32, error) {
	if v <= 0 {
		return 0, queryArgError(fmt.Sprintf("%s requires a pending short_id; increase pending_ratio or request density", queryName))
	}
	return v, nil
}

func requiredUUID(queryName string, id uuid.UUID) (uuid.UUID, error) {
	if id == uuid.Nil {
		return uuid.Nil, queryArgError(fmt.Sprintf("%s requires a pending request id; increase pending_ratio or request density", queryName))
	}
	return id, nil
}

func requiredText(queryName, value string) (string, error) {
	if value == "" {
		return "", queryArgError(fmt.Sprintf("%s requires a prompt external id; increase pending_ratio or request density", queryName))
	}
	return value, nil
}
