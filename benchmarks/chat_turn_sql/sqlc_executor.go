package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
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
	case queryLatestPage:
		items, err := e.queries.ListMessagesLatestBySession(ctx, postgresqlc.ListMessagesLatestBySessionParams{
			MaxCount:   maxCount,
			HeadTurnID: pgUUID(headID),
			SessionID:  pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryBeforePage:
		cursorID, cursorTime := selectedCursor(s, rng)
		items, err := e.queries.ListMessagesBeforeBySession(ctx, postgresqlc.ListMessagesBeforeBySessionParams{
			BeforeID:   pgUUID(cursorID),
			CreatedAt:  pgTimestamptz(cursorTime),
			MaxCount:   maxCount,
			HeadTurnID: pgUUID(headID),
			SessionID:  pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryAfterPage:
		cursorID, cursorTime := selectedCursor(s, rng)
		items, err := e.queries.ListMessagesAfterBySession(ctx, postgresqlc.ListMessagesAfterBySessionParams{
			AfterID:    pgUUID(cursorID),
			CreatedAt:  pgTimestamptz(cursorTime),
			MaxCount:   maxCount,
			HeadTurnID: pgUUID(headID),
			SessionID:  pgUUID(s.SessionID),
		})
		return int64(len(items)), err
	case queryExternalLookup:
		_, err := e.queries.GetMessageByExternalIDBySession(ctx, postgresqlc.GetMessageByExternalIDBySessionParams{
			ExternalMessageID: pgText(s.ExternalMessageID),
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
