package toolapproval

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/settings"
)

type Service struct {
	queries  *sqlc.Queries
	settings *settings.Service
	logger   *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries, settings *settings.Service) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries:  queries,
		settings: settings,
		logger:   log.With(slog.String("service", "toolapproval")),
	}
}

func (s *Service) Evaluate(ctx context.Context, input CreatePendingInput) (Evaluation, error) {
	eval, err := s.EvaluatePolicy(ctx, input)
	if err != nil || eval.Decision == DecisionBypass {
		return eval, err
	}
	req, err := s.CreatePending(ctx, input)
	if err != nil {
		return Evaluation{}, err
	}
	return Evaluation{Decision: DecisionNeedsApproval, Request: req}, nil
}

func (s *Service) EvaluatePolicy(ctx context.Context, input CreatePendingInput) (Evaluation, error) {
	if s == nil || s.settings == nil {
		return Evaluation{Decision: DecisionBypass}, nil
	}
	botSettings, err := s.settings.GetBot(ctx, input.BotID)
	if err != nil {
		return Evaluation{}, err
	}
	if !needsApproval(botSettings.ToolApprovalConfig, input.ToolName, input.ToolInput) {
		return Evaluation{Decision: DecisionBypass}, nil
	}
	return Evaluation{Decision: DecisionNeedsApproval}, nil
}

func (s *Service) CreatePending(ctx context.Context, input CreatePendingInput) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("tool approval queries not configured")
	}
	botID, err := db.ParseUUID(input.BotID)
	if err != nil {
		return Request{}, err
	}
	sessionID, err := db.ParseUUID(input.SessionID)
	if err != nil {
		return Request{}, err
	}
	toolInput, err := json.Marshal(input.ToolInput)
	if err != nil {
		return Request{}, err
	}
	channelIdentityID, err := s.optionalChannelIdentityUUID(ctx, input.ChannelIdentityID)
	if err != nil {
		return Request{}, err
	}
	requestedByID, err := s.optionalChannelIdentityUUID(ctx, input.RequestedByChannelIdentityID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.CreateToolApprovalRequest(ctx, sqlc.CreateToolApprovalRequestParams{
		BotID:                        botID,
		SessionID:                    sessionID,
		RouteID:                      optionalUUID(input.RouteID),
		ChannelIdentityID:            channelIdentityID,
		ToolCallID:                   strings.TrimSpace(input.ToolCallID),
		ToolName:                     strings.TrimSpace(input.ToolName),
		ToolInput:                    toolInput,
		RequestedByChannelIdentityID: requestedByID,
		RequestedMessageID:           optionalUUID(input.RequestedMessageID),
		SourcePlatform:               strings.TrimSpace(input.SourcePlatform),
		ReplyTarget:                  strings.TrimSpace(input.ReplyTarget),
		ConversationType:             strings.TrimSpace(input.ConversationType),
	})
	if err != nil {
		return Request{}, err
	}
	return requestFromRow(row), nil
}

func (s *Service) ResolveTarget(ctx context.Context, input ResolveInput) (Request, error) {
	botID, err := db.ParseUUID(input.BotID)
	if err != nil {
		return Request{}, err
	}
	explicit := strings.TrimSpace(input.ExplicitID)
	if strings.TrimSpace(input.SessionID) == "" && explicit != "" {
		if parsed, err := db.ParseUUID(explicit); err == nil {
			row, err := s.queries.GetToolApprovalRequest(ctx, parsed)
			if err != nil {
				return Request{}, mapLookupErr(err)
			}
			req := requestFromRow(row)
			if req.BotID != uuid.UUID(botID.Bytes).String() || req.Status != StatusPending {
				return Request{}, ErrNotFound
			}
			return req, nil
		}
		return Request{}, ErrNotFound
	}
	sessionID, err := db.ParseUUID(input.SessionID)
	if err != nil {
		return Request{}, err
	}
	if explicit != "" {
		if shortID, err := strconv.Atoi(explicit); err == nil {
			row, err := s.queries.GetPendingToolApprovalBySessionShortID(ctx, sqlc.GetPendingToolApprovalBySessionShortIDParams{
				BotID:     botID,
				SessionID: sessionID,
				ShortID:   int32(shortID), //nolint:gosec // user-facing approval numbers are small positive integers.
			})
			return requestFromRowOrErr(row, err)
		}
		if parsed, err := db.ParseUUID(explicit); err == nil {
			row, err := s.queries.GetToolApprovalRequest(ctx, parsed)
			if err != nil {
				return Request{}, mapLookupErr(err)
			}
			req := requestFromRow(row)
			if req.BotID != uuid.UUID(botID.Bytes).String() || req.SessionID != uuid.UUID(sessionID.Bytes).String() || req.Status != StatusPending {
				return Request{}, ErrNotFound
			}
			return req, nil
		}
		return Request{}, ErrNotFound
	}
	if replyID := strings.TrimSpace(input.ReplyExternalMessageID); replyID != "" {
		row, err := s.queries.GetPendingToolApprovalByReplyMessage(ctx, sqlc.GetPendingToolApprovalByReplyMessageParams{
			BotID:                   botID,
			SessionID:               sessionID,
			PromptExternalMessageID: replyID,
		})
		if err == nil {
			return requestFromRow(row), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Request{}, err
		}
	}
	row, err := s.queries.GetLatestPendingToolApprovalBySession(ctx, sqlc.GetLatestPendingToolApprovalBySessionParams{
		BotID:     botID,
		SessionID: sessionID,
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) Approve(ctx context.Context, approvalID, actorID, reason string) (Request, error) {
	id, err := db.ParseUUID(approvalID)
	if err != nil {
		return Request{}, err
	}
	decidedBy, err := s.optionalChannelIdentityUUID(ctx, actorID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.ApproveToolApprovalRequest(ctx, sqlc.ApproveToolApprovalRequestParams{
		ID:                         id,
		Reason:                     strings.TrimSpace(reason),
		DecidedByChannelIdentityID: decidedBy,
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) Reject(ctx context.Context, approvalID, actorID, reason string) (Request, error) {
	id, err := db.ParseUUID(approvalID)
	if err != nil {
		return Request{}, err
	}
	decidedBy, err := s.optionalChannelIdentityUUID(ctx, actorID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.RejectToolApprovalRequest(ctx, sqlc.RejectToolApprovalRequestParams{
		ID:                         id,
		Reason:                     strings.TrimSpace(reason),
		DecidedByChannelIdentityID: decidedBy,
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) UpdatePromptMessage(ctx context.Context, approvalID, promptMessageID, externalID string) (Request, error) {
	id, err := db.ParseUUID(approvalID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.UpdateToolApprovalPromptMessage(ctx, sqlc.UpdateToolApprovalPromptMessageParams{
		ID:                      id,
		PromptMessageID:         optionalUUID(promptMessageID),
		PromptExternalMessageID: strings.TrimSpace(externalID),
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) ListPendingBySession(ctx context.Context, botID, sessionID string) ([]Request, error) {
	return s.listBySession(ctx, botID, sessionID, true)
}

func (s *Service) ListBySession(ctx context.Context, botID, sessionID string) ([]Request, error) {
	return s.listBySession(ctx, botID, sessionID, false)
}

func (s *Service) listBySession(ctx context.Context, botID, sessionID string, pendingOnly bool) ([]Request, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	var rows []sqlc.ToolApprovalRequest
	if pendingOnly {
		rows, err = s.queries.ListPendingToolApprovalsBySession(ctx, sqlc.ListPendingToolApprovalsBySessionParams{
			BotID:     pgBotID,
			SessionID: pgSessionID,
		})
	} else {
		rows, err = s.queries.ListToolApprovalsBySession(ctx, sqlc.ListToolApprovalsBySessionParams{
			BotID:     pgBotID,
			SessionID: pgSessionID,
		})
	}
	if err != nil {
		return nil, err
	}
	result := make([]Request, 0, len(rows))
	for _, row := range rows {
		result = append(result, requestFromRow(row))
	}
	return result, nil
}

func requestFromRowOrErr(row sqlc.ToolApprovalRequest, err error) (Request, error) {
	if err != nil {
		return Request{}, mapLookupErr(err)
	}
	return requestFromRow(row), nil
}

func mapLookupErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func optionalUUID(value string) pgtype.UUID {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.UUID{}
	}
	parsed, err := db.ParseUUID(trimmed)
	if err != nil {
		return pgtype.UUID{}
	}
	return parsed
}

func (s *Service) optionalChannelIdentityUUID(ctx context.Context, value string) (pgtype.UUID, error) {
	id := optionalUUID(value)
	if !id.Valid {
		return pgtype.UUID{}, nil
	}
	if s == nil || s.queries == nil {
		return pgtype.UUID{}, nil
	}
	if _, err := s.queries.GetChannelIdentityByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, err
	}
	return id, nil
}

func requestFromRow(row sqlc.ToolApprovalRequest) Request {
	var input map[string]any
	_ = json.Unmarshal(row.ToolInput, &input)
	req := Request{
		ID:                      uuid.UUID(row.ID.Bytes).String(),
		BotID:                   uuid.UUID(row.BotID.Bytes).String(),
		SessionID:               uuid.UUID(row.SessionID.Bytes).String(),
		ToolCallID:              strings.TrimSpace(row.ToolCallID),
		ToolName:                strings.TrimSpace(row.ToolName),
		ToolInput:               input,
		ShortID:                 int(row.ShortID),
		Status:                  strings.TrimSpace(row.Status),
		DecisionReason:          strings.TrimSpace(row.DecisionReason),
		PromptExternalMessageID: strings.TrimSpace(row.PromptExternalMessageID),
		SourcePlatform:          strings.TrimSpace(row.SourcePlatform),
		ReplyTarget:             strings.TrimSpace(row.ReplyTarget),
		ConversationType:        strings.TrimSpace(row.ConversationType),
		CreatedAt:               row.CreatedAt.Time,
	}
	if row.RouteID.Valid {
		req.RouteID = uuid.UUID(row.RouteID.Bytes).String()
	}
	if row.ChannelIdentityID.Valid {
		req.ChannelIdentityID = uuid.UUID(row.ChannelIdentityID.Bytes).String()
	}
	if row.DecidedAt.Valid {
		decided := row.DecidedAt.Time
		req.DecidedAt = &decided
	}
	return req
}
