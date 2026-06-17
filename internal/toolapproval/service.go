package toolapproval

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/decision"
	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/settings"
)

type Service struct {
	queries dbstore.Queries

	settings *settings.Service
	hooks    *hooks.Service
	logger   *slog.Logger

	waiter *decision.Waiter[Request]
}

func NewService(log *slog.Logger, queries dbstore.Queries, settings *settings.Service) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries:  queries,
		settings: settings,
		logger:   log.With(slog.String("service", "toolapproval")),
		waiter:   decision.NewWaiter[Request](),
	}
}

func (s *Service) SetHookService(h *hooks.Service) {
	if s != nil {
		s.hooks = h
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
	operation, ok := OperationForTool(input.ToolName)
	if !ok {
		return Request{}, errors.New("unsupported tool approval operation")
	}
	persistBranchID := optionalUUID(input.PersistBranchID)
	persistTurnID := optionalUUID(input.PersistTurnID)
	if err := s.validatePersistContext(ctx, sessionID, persistBranchID, persistTurnID); err != nil {
		return Request{}, err
	}
	if err := s.runApprovalHook(ctx, hooks.EventBeforeApprovalCreate, input, Request{}, true); err != nil {
		return Request{}, err
	}
	params := sqlc.CreateToolApprovalRequestParams{
		BotID:                        botID,
		SessionID:                    sessionID,
		RouteID:                      optionalUUID(input.RouteID),
		ChannelIdentityID:            channelIdentityID,
		ToolCallID:                   strings.TrimSpace(input.ToolCallID),
		ToolName:                     strings.TrimSpace(input.ToolName),
		Operation:                    operation,
		ToolInput:                    toolInput,
		RequestedByChannelIdentityID: requestedByID,
		RequestedMessageID:           optionalUUID(input.RequestedMessageID),
		PersistBranchID:              persistBranchID,
		SourcePlatform:               strings.TrimSpace(input.SourcePlatform),
		ReplyTarget:                  strings.TrimSpace(input.ReplyTarget),
		ConversationType:             strings.TrimSpace(input.ConversationType),
	}
	var row sqlc.ToolApprovalRequest
	if persistTurnID.Valid {
		row, err = s.queries.CreateToolApprovalRequestForTurn(ctx, sqlc.CreateToolApprovalRequestForTurnParams{
			BotID:                        params.BotID,
			SessionID:                    params.SessionID,
			RouteID:                      params.RouteID,
			ChannelIdentityID:            params.ChannelIdentityID,
			ToolCallID:                   params.ToolCallID,
			ToolName:                     params.ToolName,
			Operation:                    params.Operation,
			ToolInput:                    params.ToolInput,
			RequestedByChannelIdentityID: params.RequestedByChannelIdentityID,
			RequestedMessageID:           params.RequestedMessageID,
			PersistBranchID:              persistBranchID,
			PersistTurnID:                persistTurnID,
			SourcePlatform:               params.SourcePlatform,
			ReplyTarget:                  params.ReplyTarget,
			ConversationType:             params.ConversationType,
		})
	} else {
		row, err = s.queries.CreateToolApprovalRequest(ctx, params)
	}
	if err != nil {
		return Request{}, err
	}
	req := requestFromRow(row)
	if req.Status != StatusPending {
		return Request{}, ErrAlreadyDecided
	}
	_ = s.runApprovalHook(ctx, hooks.EventApprovalRequested, input, req, false)
	return req, nil
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
			if !s.requestVisibleInActivePath(ctx, row.SessionID, req.PersistBranchID, req.PersistTurnID) {
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
			return s.visiblePendingRequestFromRow(ctx, sessionID, row, err)
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
			if !s.requestVisibleInActivePath(ctx, sessionID, req.PersistBranchID, req.PersistTurnID) {
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
		return s.visiblePendingRequestFromRow(ctx, sessionID, row, err)
	}
	return s.latestVisiblePendingBySession(ctx, botID, sessionID)
}

func (s *Service) requestVisibleInActivePath(ctx context.Context, sessionID pgtype.UUID, branchID, turnID string) bool {
	persistBranchID := optionalUUID(branchID)
	persistTurnID := optionalUUID(turnID)
	visible, err := s.queries.IsSessionPersistContextVisible(ctx, sqlc.IsSessionPersistContextVisibleParams{
		SessionID:       sessionID,
		PersistBranchID: persistBranchID,
		PersistTurnID:   persistTurnID,
	})
	return err == nil && visible
}

func (s *Service) visiblePendingRequestFromRow(ctx context.Context, sessionID pgtype.UUID, row sqlc.ToolApprovalRequest, err error) (Request, error) {
	req, err := requestFromRowOrErr(row, err)
	if err != nil {
		return Request{}, err
	}
	if req.Status != StatusPending {
		return Request{}, ErrNotFound
	}
	if !s.requestVisibleInActivePath(ctx, sessionID, req.PersistBranchID, req.PersistTurnID) {
		return Request{}, ErrNotFound
	}
	return req, nil
}

func (s *Service) latestVisiblePendingBySession(ctx context.Context, botID, sessionID pgtype.UUID) (Request, error) {
	rows, err := s.queries.ListPendingToolApprovalsBySession(ctx, sqlc.ListPendingToolApprovalsBySessionParams{
		BotID:     botID,
		SessionID: sessionID,
	})
	if err != nil {
		return Request{}, err
	}
	for i := len(rows) - 1; i >= 0; i-- {
		req := requestFromRow(rows[i])
		if s.requestVisibleInActivePath(ctx, rows[i].SessionID, req.PersistBranchID, req.PersistTurnID) {
			return req, nil
		}
	}
	return Request{}, ErrNotFound
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
	if err := s.ensurePendingRequestVisible(ctx, approvalID); err != nil {
		return Request{}, err
	}
	row, err := s.queries.ApproveToolApprovalRequest(ctx, sqlc.ApproveToolApprovalRequestParams{
		ID:                         id,
		Reason:                     strings.TrimSpace(reason),
		DecidedByChannelIdentityID: decidedBy,
	})
	req, err := s.resolveAndNotify(ctx, approvalID, row, err)
	if err == nil && strings.TrimSpace(actorID) != "" {
		req.DecidedByUser = true
	}
	if err == nil {
		_ = s.runApprovalHook(ctx, hooks.EventApprovalResolved, CreatePendingInput{}, req, false)
	}
	return req, err
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
	if err := s.ensurePendingRequestVisible(ctx, approvalID); err != nil {
		return Request{}, err
	}
	row, err := s.queries.RejectToolApprovalRequest(ctx, sqlc.RejectToolApprovalRequestParams{
		ID:                         id,
		Reason:                     strings.TrimSpace(reason),
		DecidedByChannelIdentityID: decidedBy,
	})
	req, err := s.resolveAndNotify(ctx, approvalID, row, err)
	if err == nil && strings.TrimSpace(actorID) != "" {
		req.DecidedByUser = true
	}
	if err == nil {
		_ = s.runApprovalHook(ctx, hooks.EventApprovalResolved, CreatePendingInput{}, req, false)
	}
	return req, err
}

func (s *Service) ensurePendingRequestVisible(ctx context.Context, approvalID string) error {
	req, err := s.Get(ctx, approvalID)
	if err != nil {
		return err
	}
	if req.Status != StatusPending {
		return ErrAlreadyDecided
	}
	sessionID, err := db.ParseUUID(req.SessionID)
	if err != nil {
		return err
	}
	if !s.requestVisibleInActivePath(ctx, sessionID, req.PersistBranchID, req.PersistTurnID) {
		return ErrNotFound
	}
	return nil
}

// CancelPendingForSession closes pending approvals that belonged to an ended
// turn and wakes any in-process waiters.
func (s *Service) CancelPendingForSession(ctx context.Context, botID, sessionID, reason string) ([]Request, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("tool approval queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "tool approval cancelled: the turn that requested it ended"
	}
	rows, err := s.queries.CancelPendingToolApprovalsBySession(ctx, sqlc.CancelPendingToolApprovalsBySessionParams{
		BotID:     pgBotID,
		SessionID: pgSessionID,
		Reason:    reason,
	})
	if err != nil {
		return nil, err
	}
	requests := make([]Request, 0, len(rows))
	for _, row := range rows {
		req := requestFromRow(row)
		requests = append(requests, req)
		s.notifyResolved(req)
	}
	return requests, nil
}

func (s *Service) Get(ctx context.Context, approvalID string) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("tool approval queries not configured")
	}
	id, err := db.ParseUUID(approvalID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.GetToolApprovalRequest(ctx, id)
	return requestFromRowOrErr(row, err)
}

func (s *Service) WaitForDecision(ctx context.Context, approvalID string) (Request, error) {
	if s == nil {
		return Request{}, errors.New("tool approval service not configured")
	}
	poll := func(ctx context.Context) (Request, bool, error) {
		req, err := s.Get(ctx, approvalID)
		if err != nil {
			return Request{}, false, err
		}
		if req.Status != StatusPending {
			return req, true, nil
		}
		return Request{}, false, nil
	}
	req, err := s.waiter.Await(ctx, approvalID, decision.DefaultFallbackInterval, poll)
	if err != nil && ctx.Err() != nil {
		return s.resolvedAfterContextDone(ctx, approvalID)
	}
	return req, err
}

func (s *Service) RegisterWaiter(approvalID string) func() {
	if s == nil || s.waiter == nil {
		return func() {}
	}
	return s.waiter.Register(approvalID)
}

func (s *Service) NotifyApprovalTimeout(ctx context.Context, req Request) {
	_ = s.runApprovalHook(ctx, hooks.EventApprovalTimeout, CreatePendingInput{}, req, false)
}

func (s *Service) runApprovalHook(ctx context.Context, event string, input CreatePendingInput, req Request, failOnError bool) error {
	if s == nil || s.hooks == nil {
		return nil
	}
	botID := firstApprovalValue(req.BotID, input.BotID)
	sessionID := firstApprovalValue(req.SessionID, input.SessionID)
	payload := map[string]any{
		"tool_call_id": firstApprovalValue(req.ToolCallID, input.ToolCallID),
		"tool_name":    firstApprovalValue(req.ToolName, input.ToolName),
		"operation":    req.Operation,
		"status":       req.Status,
		"approval_id":  req.ID,
		"short_id":     req.ShortID,
		"reason":       req.DecisionReason,
	}
	if req.Operation == "" {
		if operation, ok := OperationForTool(input.ToolName); ok {
			payload["operation"] = operation
		}
	}
	if req.ToolInput != nil {
		payload["tool_input"] = req.ToolInput
	} else if input.ToolInput != nil {
		payload["tool_input"] = input.ToolInput
	}
	result, err := s.hooks.Run(ctx, hooks.Request{
		Version:   1,
		Event:     event,
		BotID:     botID,
		SessionID: sessionID,
		Tool: &hooks.ToolPayload{
			Name:   firstApprovalValue(req.ToolName, input.ToolName),
			CallID: firstApprovalValue(req.ToolCallID, input.ToolCallID),
			Input:  payload["tool_input"],
		},
		Approval: payload,
	}, nil)
	if err == nil && result.Decision == hooks.DecisionDeny {
		err = hooks.ErrDenied
	}
	if err != nil {
		if failOnError {
			return err
		}
		if s.logger != nil {
			s.logger.Warn("approval hook failed",
				slog.String("event", event),
				slog.String("bot_id", botID),
				slog.String("session_id", sessionID),
				slog.Any("error", err),
			)
		}
	}
	return nil
}

func firstApprovalValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Service) HasWaiter(approvalID string) bool {
	return s != nil && s.waiter != nil && s.waiter.Has(approvalID)
}

// CanRespond reports whether a waiter-backed approval can accept a user
// decision in this process. Native chat approvals are DB-deferred and must not
// use this helper; it is for ACP/MCP flows whose caller is blocked in process.
func (s *Service) CanRespond(req Request) bool {
	return strings.EqualFold(NormalizedStatus(req.Status), StatusPending) && s.HasWaiter(req.ID)
}

// resolveAndNotify converts a guarded approval update into the shared service
// result. If the update matched no pending row, a terminal row with the same
// ID means another responder or waiter won the race.
func (s *Service) resolveAndNotify(ctx context.Context, approvalID string, row sqlc.ToolApprovalRequest, err error) (Request, error) {
	req, err := requestFromRowOrErr(row, err)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if existing, getErr := s.Get(ctx, approvalID); getErr == nil && existing.Status != StatusPending {
				return Request{}, ErrAlreadyDecided
			}
		}
		return Request{}, err
	}
	s.notifyResolved(req)
	return req, nil
}

func (s *Service) notifyResolved(req Request) {
	if s == nil || s.waiter == nil {
		return
	}
	s.waiter.Notify(req.ID, req)
}

func (s *Service) resolvedAfterContextDone(ctx context.Context, approvalID string) (Request, error) {
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if req, err := s.Get(finalCtx, approvalID); err == nil && req.Status != StatusPending {
		return req, nil
	}
	return Request{}, ctx.Err()
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

func (s *Service) UpdatePersistContext(ctx context.Context, approvalID, branchID, turnID string) (Request, error) {
	id, err := db.ParseUUID(approvalID)
	if err != nil {
		return Request{}, err
	}
	current, err := s.queries.GetToolApprovalRequest(ctx, id)
	if err != nil {
		return Request{}, mapLookupErr(err)
	}
	persistBranchID := optionalUUID(branchID)
	persistTurnID := optionalUUID(turnID)
	if err := s.validatePersistContext(ctx, current.SessionID, persistBranchID, persistTurnID); err != nil {
		return Request{}, err
	}
	row, err := s.queries.UpdateToolApprovalPersistContext(ctx, sqlc.UpdateToolApprovalPersistContextParams{
		ID:              id,
		PersistBranchID: persistBranchID,
		PersistTurnID:   persistTurnID,
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) validatePersistContext(ctx context.Context, sessionID, branchID, turnID pgtype.UUID) error {
	if !branchID.Valid {
		if turnID.Valid {
			return errors.New("persist turn requires session and branch")
		}
		return nil
	}
	if !sessionID.Valid {
		return errors.New("persist branch requires session")
	}
	if _, err := s.queries.GetSessionBranchForPersist(ctx, sqlc.GetSessionBranchForPersistParams{
		BranchID:  branchID,
		SessionID: sessionID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("persist branch does not belong to session")
		}
		return err
	}
	if !turnID.Valid {
		return nil
	}
	if _, err := s.queries.GetHistoryTurnForMessagePersist(ctx, sqlc.GetHistoryTurnForMessagePersistParams{
		TurnID:    turnID,
		SessionID: sessionID,
		BranchID:  branchID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("persist turn does not belong to session branch")
		}
		return err
	}
	return nil
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
		req := requestFromRow(row)
		if !s.requestVisibleInActivePath(ctx, row.SessionID, req.PersistBranchID, req.PersistTurnID) {
			continue
		}
		result = append(result, req)
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
		Operation:               strings.TrimSpace(row.Operation),
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
	if req.Operation == "" {
		req.Operation, _ = OperationForTool(req.ToolName)
	}
	if row.RouteID.Valid {
		req.RouteID = uuid.UUID(row.RouteID.Bytes).String()
	}
	if row.ChannelIdentityID.Valid {
		req.ChannelIdentityID = uuid.UUID(row.ChannelIdentityID.Bytes).String()
	}
	if row.DecidedByChannelIdentityID.Valid {
		req.DecidedByUser = true
	}
	if row.DecidedAt.Valid {
		decided := row.DecidedAt.Time
		req.DecidedAt = &decided
	}
	if row.PersistBranchID.Valid {
		req.PersistBranchID = uuid.UUID(row.PersistBranchID.Bytes).String()
	}
	if row.PersistTurnID.Valid {
		req.PersistTurnID = uuid.UUID(row.PersistTurnID.Bytes).String()
	}
	return req
}
