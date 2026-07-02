package userinput

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
)

const (
	submitInstruction = "The user submitted this answer for the current ask_user request. Use it only to resolve that specific question. If the user later asks for another choice, quiz, or decision, call ask_user again before grading or continuing."
	cancelInstruction = "The user canceled this input request. Do not ask the same question again; continue with a reasonable choice from the available context or briefly explain the next step."
)

type Service struct {
	queries dbstore.Queries

	logger *slog.Logger

	waiter *decision.Waiter[Request]
}

func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "userinput")),
		waiter:  decision.NewWaiter[Request](),
	}
}

// RegisterWaiter records that a caller in this process owns the request's
// resolution. Callers that announce a pending request to users must register
// BEFORE announcing, or an instant response can be misjudged as orphaned.
// The returned release must run when the wait ends.
func (s *Service) RegisterWaiter(requestID string) func() {
	if s == nil || s.waiter == nil {
		return func() {}
	}
	return s.waiter.Register(requestID)
}

// HasWaiter reports whether anyone in this process is currently registered
// for the request. It is only a local fast-path signal; DB status remains the
// cross-process source of truth for whether a request can accept a response.
func (s *Service) HasWaiter(requestID string) bool {
	return s != nil && s.waiter != nil && s.waiter.Has(requestID)
}

// CanRespond reports whether the UI should offer a response action for this
// request in the current server process. ACP/MCP requests are consumed by an
// in-process waiter, so a pending DB row alone is not enough.
func (s *Service) CanRespond(req Request) bool {
	if req.Status != StatusPending {
		return false
	}
	if IsACPMCPRequest(req) {
		return s.HasWaiter(req.ID)
	}
	return true
}

func (s *Service) notifyResolved(req Request) {
	if s == nil || s.waiter == nil {
		return
	}
	s.waiter.Notify(req.ID, req)
}

// resolveAndNotify converts a terminal-transition row, then wakes any waiters.
// Shared by Submit, Cancel, and Fail so notification can never drift between
// resolution paths. A guarded update that matched no row is disambiguated:
// an existing non-pending request means the transition lost a race to another
// decision (or to expiry), not that the request is unknown.
func (s *Service) resolveAndNotify(ctx context.Context, requestID string, row sqlc.UserInputRequest, err error) (Request, error) {
	resolved, err := requestFromRowOrErr(row, err)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if req, getErr := s.Get(ctx, requestID); getErr == nil && req.Status != StatusPending {
				return Request{}, ErrAlreadyDecided
			}
		}
		return Request{}, err
	}
	s.notifyResolved(resolved)
	return resolved, nil
}

func (s *Service) CreatePending(ctx context.Context, input CreatePendingInput) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("user input queries not configured")
	}
	botID, err := db.ParseUUID(input.BotID)
	if err != nil {
		return Request{}, err
	}
	sessionID, err := db.ParseUUID(input.SessionID)
	if err != nil {
		return Request{}, err
	}
	toolCallID := strings.TrimSpace(input.ToolCallID)
	if toolCallID == "" {
		return Request{}, errors.New("tool_call_id is required")
	}
	toolName := strings.TrimSpace(input.ToolName)
	if toolName == "" {
		toolName = ToolNameAskUser
	}
	if toolName != ToolNameAskUser {
		return Request{}, fmt.Errorf("unsupported user input tool %q", toolName)
	}
	uiPayload, err := ParseAskUserPayload(input.Input)
	if err != nil {
		return Request{}, err
	}
	rawInput, err := marshalObject(input.Input)
	if err != nil {
		return Request{}, err
	}
	uiPayloadJSON, err := json.Marshal(uiPayload)
	if err != nil {
		return Request{}, err
	}
	providerMetadata, err := marshalObject(input.ProviderMetadata)
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
	params := sqlc.CreateUserInputRequestParams{
		BotID:                        botID,
		SessionID:                    sessionID,
		RouteID:                      optionalUUID(input.RouteID),
		ChannelIdentityID:            channelIdentityID,
		ToolCallID:                   toolCallID,
		ToolName:                     toolName,
		InputJson:                    rawInput,
		UiPayloadJson:                uiPayloadJSON,
		ProviderMetadata:             providerMetadata,
		RequestedByChannelIdentityID: requestedByID,
		PersistTurnID:                optionalUUID(input.PersistTurnID),
		SourcePlatform:               strings.TrimSpace(input.SourcePlatform),
		ReplyTarget:                  strings.TrimSpace(input.ReplyTarget),
		ConversationType:             strings.TrimSpace(input.ConversationType),
		ExpiresAt:                    optionalTime(input.ExpiresAt),
	}
	var row sqlc.UserInputRequest
	if params.PersistTurnID.Valid {
		row, err = s.queries.CreateUserInputRequestForTurn(ctx, sqlc.CreateUserInputRequestForTurnParams{
			BotID:                        params.BotID,
			SessionID:                    params.SessionID,
			RouteID:                      params.RouteID,
			ChannelIdentityID:            params.ChannelIdentityID,
			ToolCallID:                   params.ToolCallID,
			ToolName:                     params.ToolName,
			InputJson:                    params.InputJson,
			UiPayloadJson:                params.UiPayloadJson,
			ProviderMetadata:             params.ProviderMetadata,
			RequestedByChannelIdentityID: params.RequestedByChannelIdentityID,
			SourcePlatform:               params.SourcePlatform,
			ReplyTarget:                  params.ReplyTarget,
			ConversationType:             params.ConversationType,
			ExpiresAt:                    params.ExpiresAt,
			PersistTurnID:                params.PersistTurnID,
		})
	} else {
		row, err = s.queries.CreateUserInputRequest(ctx, params)
	}
	if err != nil {
		if errors.Is(mapLookupErr(err), ErrNotFound) {
			var existing sqlc.UserInputRequest
			var getErr error
			if params.PersistTurnID.Valid {
				existing, getErr = s.queries.GetUserInputRequestBySessionToolCallTurn(ctx, sqlc.GetUserInputRequestBySessionToolCallTurnParams{
					SessionID:     sessionID,
					ToolCallID:    toolCallID,
					PersistTurnID: params.PersistTurnID,
				})
			} else {
				existing, getErr = s.queries.GetUserInputRequestBySessionToolCall(ctx, sqlc.GetUserInputRequestBySessionToolCallParams{
					SessionID:  sessionID,
					ToolCallID: toolCallID,
				})
			}
			if getErr == nil {
				existingReq := requestFromRow(existing)
				if existingReq.Status != StatusPending {
					return Request{}, ErrAlreadyDecided
				}
				return existingReq, nil
			}
		}
		return Request{}, mapLookupErr(err)
	}
	return requestFromRow(row), nil
}

func (s *Service) ResolveTarget(ctx context.Context, input ResolveInput) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("user input queries not configured")
	}
	botID, err := db.ParseUUID(input.BotID)
	if err != nil {
		return Request{}, err
	}
	explicit := strings.TrimSpace(input.ExplicitID)
	if strings.TrimSpace(input.SessionID) == "" && explicit != "" {
		if parsed, err := db.ParseUUID(explicit); err == nil {
			row, err := s.queries.GetUserInputRequest(ctx, parsed)
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
			row, err := s.queries.GetPendingUserInputBySessionShortID(ctx, sqlc.GetPendingUserInputBySessionShortIDParams{
				BotID:     botID,
				SessionID: sessionID,
				ShortID:   int32(shortID), //nolint:gosec // user-facing request numbers are small positive integers.
			})
			return requestFromRowOrErr(row, err)
		}
		if parsed, err := db.ParseUUID(explicit); err == nil {
			baseHeadTurnID, err := parseOptionalUUID(input.BaseHeadTurnID)
			if err != nil {
				return Request{}, err
			}
			var row sqlc.UserInputRequest
			if baseHeadTurnID.Valid {
				row, err = s.queries.GetPendingUserInputByBaseHeadRequestID(ctx, sqlc.GetPendingUserInputByBaseHeadRequestIDParams{
					ID:             parsed,
					BotID:          botID,
					SessionID:      sessionID,
					BaseHeadTurnID: baseHeadTurnID,
				})
			} else {
				row, err = s.queries.GetPendingUserInputByVisibleRequestID(ctx, sqlc.GetPendingUserInputByVisibleRequestIDParams{
					ID:        parsed,
					BotID:     botID,
					SessionID: sessionID,
				})
			}
			if err != nil {
				return Request{}, mapLookupErr(err)
			}
			return requestFromRow(row), nil
		}
		return Request{}, ErrNotFound
	}
	if replyID := strings.TrimSpace(input.ReplyExternalMessageID); replyID != "" {
		row, err := s.queries.GetPendingUserInputByReplyMessage(ctx, sqlc.GetPendingUserInputByReplyMessageParams{
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
	row, err := s.queries.GetLatestPendingUserInputBySession(ctx, sqlc.GetLatestPendingUserInputBySessionParams{
		BotID:     botID,
		SessionID: sessionID,
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) Get(ctx context.Context, requestID string) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("user input queries not configured")
	}
	id, err := db.ParseUUID(requestID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.GetUserInputRequest(ctx, id)
	return requestFromRowOrErr(row, err)
}

// WaitForResponse blocks until the request leaves pending. Resolution inside
// this process arrives via the Submit/Cancel/Fail broadcast; the slow ticker
// is only a safety net for transitions this process cannot observe (another
// node, manual DB changes, time-based expiry).
func (s *Service) WaitForResponse(ctx context.Context, requestID string) (Request, error) {
	release := s.RegisterWaiter(requestID)
	defer release()
	return s.waitForResponse(ctx, requestID)
}

// WaitForRegisteredResponse waits like WaitForResponse but assumes the caller
// already registered with RegisterWaiter before announcing the request.
func (s *Service) WaitForRegisteredResponse(ctx context.Context, requestID string) (Request, error) {
	return s.waitForResponse(ctx, requestID)
}

func (s *Service) waitForResponse(ctx context.Context, requestID string) (Request, error) {
	poll := func(ctx context.Context) (Request, bool, error) {
		req, err := s.Get(ctx, requestID)
		if err != nil {
			return Request{}, false, err
		}
		if req.Status != StatusPending {
			return req, true, nil
		}
		return Request{}, false, nil
	}
	req, err := s.waiter.Await(ctx, requestID, decision.DefaultFallbackInterval, poll)
	if err != nil && ctx.Err() != nil {
		return s.resolvedAfterContextDone(ctx, requestID)
	}
	return req, err
}

func (s *Service) resolvedAfterContextDone(ctx context.Context, requestID string) (Request, error) {
	// A resolution may have committed before its notification was delivered.
	// Prefer the answer over the caller's cancellation.
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if req, err := s.Get(finalCtx, requestID); err == nil && req.Status != StatusPending {
		return req, nil
	}
	return Request{}, ctx.Err()
}

func (s *Service) Submit(ctx context.Context, input SubmitInput) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("user input queries not configured")
	}
	id, err := db.ParseUUID(input.RequestID)
	if err != nil {
		return Request{}, err
	}
	actorID, err := s.optionalChannelIdentityUUID(ctx, input.ActorChannelIdentityID)
	if err != nil {
		return Request{}, err
	}
	req, err := s.Get(ctx, input.RequestID)
	if err != nil {
		return Request{}, err
	}
	if req.Status != StatusPending {
		return Request{}, ErrAlreadyDecided
	}
	result, err := submittedResult(req.UIPayload, input.Answers)
	if err != nil {
		return Request{}, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.SubmitUserInputRequest(ctx, sqlc.SubmitUserInputRequestParams{
		ID:                           id,
		ResultJson:                   resultJSON,
		RespondedByChannelIdentityID: actorID,
	})
	return s.resolveAndNotify(ctx, input.RequestID, row, err)
}

func (s *Service) Cancel(ctx context.Context, input CancelInput) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("user input queries not configured")
	}
	id, err := db.ParseUUID(input.RequestID)
	if err != nil {
		return Request{}, err
	}
	actorID, err := s.optionalChannelIdentityUUID(ctx, input.ActorChannelIdentityID)
	if err != nil {
		return Request{}, err
	}
	result := canceledResult(input.Reason)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.CancelUserInputRequest(ctx, sqlc.CancelUserInputRequestParams{
		ID:                           id,
		ResultJson:                   resultJSON,
		RespondedByChannelIdentityID: actorID,
	})
	return s.resolveAndNotify(ctx, input.RequestID, row, err)
}

func (s *Service) CancelPendingForSession(ctx context.Context, botID, sessionID, reason string) ([]Request, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("user input queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	resultJSON, err := json.Marshal(canceledResult(reason))
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.CancelPendingUserInputsBySession(ctx, sqlc.CancelPendingUserInputsBySessionParams{
		BotID:      pgBotID,
		SessionID:  pgSessionID,
		ResultJson: resultJSON,
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

func (s *Service) Fail(ctx context.Context, requestID string, result map[string]any) (Request, error) {
	if s == nil || s.queries == nil {
		return Request{}, errors.New("user input queries not configured")
	}
	id, err := db.ParseUUID(requestID)
	if err != nil {
		return Request{}, err
	}
	if result == nil {
		result = map[string]any{"status": StatusFailed}
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.FailUserInputRequest(ctx, sqlc.FailUserInputRequestParams{
		ID:         id,
		ResultJson: resultJSON,
	})
	return s.resolveAndNotify(ctx, requestID, row, err)
}

func (s *Service) UpdatePromptMessage(ctx context.Context, requestID, promptMessageID, externalID string) (Request, error) {
	id, err := db.ParseUUID(requestID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.UpdateUserInputPromptMessage(ctx, sqlc.UpdateUserInputPromptMessageParams{
		ID:                      id,
		PromptMessageID:         optionalUUID(promptMessageID),
		PromptExternalMessageID: strings.TrimSpace(externalID),
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) UpdateAssistantMessage(ctx context.Context, requestID, messageID string) (Request, error) {
	id, err := db.ParseUUID(requestID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.UpdateUserInputAssistantMessage(ctx, sqlc.UpdateUserInputAssistantMessageParams{
		ID:                 id,
		AssistantMessageID: optionalUUID(messageID),
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) UpdateToolResultMessage(ctx context.Context, requestID, messageID string) (Request, error) {
	id, err := db.ParseUUID(requestID)
	if err != nil {
		return Request{}, err
	}
	row, err := s.queries.UpdateUserInputToolResultMessage(ctx, sqlc.UpdateUserInputToolResultMessageParams{
		ID:                  id,
		ToolResultMessageID: optionalUUID(messageID),
	})
	return requestFromRowOrErr(row, err)
}

func (s *Service) ListPendingBySession(ctx context.Context, botID, sessionID string) ([]Request, error) {
	return s.listBySession(ctx, botID, sessionID, true)
}

func (s *Service) ListBySession(ctx context.Context, botID, sessionID string) ([]Request, error) {
	return s.listBySession(ctx, botID, sessionID, false)
}

func (s *Service) ListBySessionToolCalls(ctx context.Context, botID, sessionID string, toolCallIDs []string, turnIDs []string) ([]Request, error) {
	toolCallIDs = compactNonEmptyStrings(toolCallIDs)
	if len(toolCallIDs) == 0 {
		return nil, nil
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	pgTurnIDs, err := parseUUIDList(turnIDs)
	if err != nil {
		return nil, err
	}
	if len(pgTurnIDs) == 0 {
		pgTurnIDs = []pgtype.UUID{{Valid: false}}
	}
	rows, err := s.queries.ListUserInputsBySessionToolCalls(ctx, sqlc.ListUserInputsBySessionToolCallsParams{
		BotID:       pgBotID,
		SessionID:   pgSessionID,
		ToolCallIds: toolCallIDs,
		TurnIds:     pgTurnIDs,
	})
	if err != nil {
		return nil, err
	}
	result := make([]Request, 0, len(rows))
	for _, row := range rows {
		result = append(result, requestFromRow(row))
	}
	return result, nil
}

func (s *Service) ListBySessionTurnGraph(ctx context.Context, botID, sessionID string) ([]Request, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListUserInputsBySessionTurnGraph(ctx, sqlc.ListUserInputsBySessionTurnGraphParams{
		BotID:     pgBotID,
		SessionID: pgSessionID,
	})
	if err != nil {
		return nil, err
	}
	result := make([]Request, 0, len(rows))
	for _, row := range rows {
		result = append(result, requestFromRow(row))
	}
	return result, nil
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
	var rows []sqlc.UserInputRequest
	if pendingOnly {
		rows, err = s.queries.ListPendingUserInputsBySession(ctx, sqlc.ListPendingUserInputsBySessionParams{
			BotID:     pgBotID,
			SessionID: pgSessionID,
		})
	} else {
		rows, err = s.queries.ListUserInputsBySession(ctx, sqlc.ListUserInputsBySessionParams{
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

func DeferredMetadata(req Request) map[string]any {
	return map[string]any{
		"kind":            DeferredKind,
		"user_input_id":   req.ID,
		"short_id":        req.ShortID,
		"status":          req.Status,
		"tool_call_id":    req.ToolCallID,
		"tool_name":       req.ToolName,
		"ui_payload":      req.UIPayload,
		"persist_turn_id": req.PersistTurnID,
	}
}

func compactNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func parseUUIDList(values []string) ([]pgtype.UUID, error) {
	values = compactNonEmptyStrings(values)
	result := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		id, err := db.ParseUUID(value)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

// submittedResult validates the user's answers against the stored payload and
// builds the tool result returned to the model. Every question must be answered.
func submittedResult(payload UIPayload, answers []QuestionAnswer) (map[string]any, error) {
	if len(payload.Questions) == 0 {
		return nil, errors.New("user input request has no questions")
	}
	byQuestion := make(map[string]QuestionAnswer, len(answers))
	for _, answer := range answers {
		id := strings.TrimSpace(answer.QuestionID)
		if id == "" {
			return nil, errors.New("answers[].question_id is required")
		}
		if _, ok := payload.Question(id); !ok {
			return nil, fmt.Errorf("unknown question %q", id)
		}
		if _, dup := byQuestion[id]; dup {
			return nil, fmt.Errorf("duplicate answer for question %q", id)
		}
		byQuestion[id] = answer
	}

	resultAnswers := make([]map[string]any, 0, len(payload.Questions))
	for _, question := range payload.Questions {
		answer, ok := byQuestion[question.ID]
		if !ok {
			return nil, fmt.Errorf("missing answer for question %q", question.ID)
		}
		entry, err := answerEntry(question, answer)
		if err != nil {
			return nil, err
		}
		resultAnswers = append(resultAnswers, entry)
	}
	return map[string]any{
		"status":      StatusSubmitted,
		"answers":     resultAnswers,
		"instruction": submitInstruction,
	}, nil
}

func answerEntry(question UIQuestion, answer QuestionAnswer) (map[string]any, error) {
	entry := map[string]any{
		"question_id": question.ID,
		"question":    question.Text,
	}
	optionIDs := cleanIDs(answer.OptionIDs)
	customText := strings.TrimSpace(answer.CustomText)
	text := strings.TrimSpace(answer.Text)

	if question.Kind == QuestionKindText {
		if len(optionIDs) > 0 || customText != "" {
			return nil, fmt.Errorf("question %q is free text and does not accept option selections", question.ID)
		}
		if text == "" {
			return nil, fmt.Errorf("question %q requires a text answer", question.ID)
		}
		entry["text"] = text
		return entry, nil
	}

	if text != "" {
		return nil, fmt.Errorf("question %q is a select question; use option_ids or custom_text", question.ID)
	}
	if customText != "" && !question.AllowCustom {
		return nil, fmt.Errorf("question %q does not allow a custom answer", question.ID)
	}
	if question.Kind == QuestionKindSingleSelect {
		if len(optionIDs) > 1 {
			return nil, fmt.Errorf("question %q accepts exactly one option", question.ID)
		}
		if len(optionIDs) == 1 && customText != "" {
			return nil, fmt.Errorf("question %q accepts either one option or a custom answer, not both", question.ID)
		}
	}
	if len(optionIDs) == 0 && customText == "" {
		return nil, fmt.Errorf("question %q requires a selection", question.ID)
	}

	selected := make([]map[string]any, 0, len(optionIDs))
	seen := make(map[string]struct{}, len(optionIDs))
	for _, id := range optionIDs {
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("question %q selects option %q more than once", question.ID, id)
		}
		seen[id] = struct{}{}
		option, ok := question.Option(id)
		if !ok {
			return nil, fmt.Errorf("question %q has no option %q", question.ID, id)
		}
		selected = append(selected, map[string]any{"id": option.ID, "label": option.Label})
	}
	if len(selected) > 0 {
		entry["selected"] = selected
	}
	if customText != "" {
		entry["custom_text"] = customText
	}
	return entry, nil
}

func canceledResult(reason string) map[string]any {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "user_canceled"
	}
	return map[string]any{
		"status":      StatusCanceled,
		"reason":      reason,
		"instruction": cancelInstruction,
	}
}

func IsACPMCPRequest(req Request) bool {
	if req.ProviderMetadata == nil {
		return false
	}
	return strings.TrimSpace(stringValue(req.ProviderMetadata["source"])) == ProviderSourceACPMCP
}

func cleanIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func requestFromRowOrErr(row sqlc.UserInputRequest, err error) (Request, error) {
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

func requestFromRow(row sqlc.UserInputRequest) Request {
	req := Request{
		ID:                      uuid.UUID(row.ID.Bytes).String(),
		BotID:                   uuid.UUID(row.BotID.Bytes).String(),
		SessionID:               uuid.UUID(row.SessionID.Bytes).String(),
		ToolCallID:              strings.TrimSpace(row.ToolCallID),
		ToolName:                strings.TrimSpace(row.ToolName),
		ShortID:                 int(row.ShortID),
		Status:                  strings.TrimSpace(row.Status),
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
	if row.PersistTurnID.Valid {
		req.PersistTurnID = uuid.UUID(row.PersistTurnID.Bytes).String()
	}
	if row.RespondedAt.Valid {
		responded := row.RespondedAt.Time
		req.RespondedAt = &responded
	}
	if row.CanceledAt.Valid {
		canceled := row.CanceledAt.Time
		req.CanceledAt = &canceled
	}
	if row.ExpiresAt.Valid {
		expires := row.ExpiresAt.Time
		req.ExpiresAt = &expires
	}
	// Present overdue pending rows as expired even before any sweeper runs;
	// the SQL pending/submit guards enforce the same boundary transactionally.
	if req.Status == StatusPending && req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		req.Status = StatusExpired
	}
	_ = json.Unmarshal(row.InputJson, &req.Input)
	req.UIPayload = PayloadFromStored(row.UiPayloadJson)
	_ = json.Unmarshal(row.ResultJson, &req.Result)
	_ = json.Unmarshal(row.ProviderMetadata, &req.ProviderMetadata)
	return req
}

func marshalObject(value any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	if data, ok := value.([]byte); ok {
		if len(data) == 0 {
			return []byte("{}"), nil
		}
		return data, nil
	}
	if text, ok := value.(string); ok {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return []byte("{}"), nil
		}
		return []byte(trimmed), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []byte("{}"), nil
	}
	return data, nil
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

func parseOptionalUUID(value string) (pgtype.UUID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(trimmed)
}

func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil || value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
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
