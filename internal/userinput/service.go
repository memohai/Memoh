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
)

const (
	submitInstruction = "The user submitted this answer for the current ask_user request. Use it only to resolve that specific question. If the user later asks for another choice, quiz, or decision, call ask_user again before grading or continuing."
	cancelInstruction = "The user canceled this input request. Do not ask the same question again; continue with a reasonable choice from the available context or briefly explain the next step."
)

type Service struct {
	queries dbstore.Queries
}

func NewService(_ *slog.Logger, queries dbstore.Queries) *Service {
	return &Service{
		queries: queries,
	}
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
	if err := ValidateAskUserInput(input.Input); err != nil {
		return Request{}, err
	}
	rawInput, err := marshalObject(input.Input)
	if err != nil {
		return Request{}, err
	}
	uiPayload := normalizeUIPayload(input.Input)
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
	row, err := s.queries.CreateUserInputRequest(ctx, sqlc.CreateUserInputRequestParams{
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
		SourcePlatform:               strings.TrimSpace(input.SourcePlatform),
		ReplyTarget:                  strings.TrimSpace(input.ReplyTarget),
		ConversationType:             strings.TrimSpace(input.ConversationType),
		ExpiresAt:                    optionalTime(input.ExpiresAt),
	})
	return requestFromRowOrErr(row, err)
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
			row, err := s.queries.GetUserInputRequest(ctx, parsed)
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
	result, err := submittedResult(input)
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
	return requestFromRowOrErr(row, err)
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
	return requestFromRowOrErr(row, err)
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
	return requestFromRowOrErr(row, err)
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
		"kind":          DeferredKind,
		"user_input_id": req.ID,
		"short_id":      req.ShortID,
		"status":        req.Status,
		"tool_call_id":  req.ToolCallID,
		"tool_name":     req.ToolName,
		"ui_payload":    req.UIPayload,
	}
}

func submittedResult(input SubmitInput) (map[string]any, error) {
	userResponse := make(map[string]any)
	for key, value := range input.RawUserResponse {
		userResponse[key] = value
	}
	if strings.TrimSpace(input.OptionID) != "" {
		userResponse["option_id"] = strings.TrimSpace(input.OptionID)
	}
	if input.OptionValue != nil {
		userResponse["value"] = input.OptionValue
	}
	if input.Answer != nil {
		userResponse["answer"] = input.Answer
	}
	if len(userResponse) == 0 {
		return nil, errors.New("answer, option_id, value, or user_response is required")
	}
	return map[string]any{
		"status":        StatusSubmitted,
		"user_response": userResponse,
		"instruction":   submitInstruction,
	}, nil
}

func canceledResult(reason string) map[string]any {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "user_canceled"
	}
	return map[string]any{
		"status": StatusCanceled,
		"user_response": map[string]any{
			"canceled": true,
			"reason":   reason,
		},
		"instruction": cancelInstruction,
	}
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
	if row.RespondedAt.Valid {
		responded := row.RespondedAt.Time
		req.RespondedAt = &responded
	}
	if row.CanceledAt.Valid {
		canceled := row.CanceledAt.Time
		req.CanceledAt = &canceled
	}
	_ = json.Unmarshal(row.InputJson, &req.Input)
	_ = json.Unmarshal(row.UiPayloadJson, &req.UIPayload)
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
