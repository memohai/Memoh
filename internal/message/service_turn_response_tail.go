package message

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/runtimefence"
)

type atomicTransactionalQueries interface {
	transactionalQueries
	SupportsTransactions() bool
}

type turnResponseDeliveryCompleter interface {
	CompleteSessionEventDeliveryWithResponse(context.Context, sqlc.CompleteSessionEventDeliveryWithResponseParams) (int64, error)
}

type turnResponseDeliveryClaimLocker interface {
	LockSessionEventDeliveryClaim(context.Context, sqlc.LockSessionEventDeliveryClaimParams) (bool, error)
}

func lockDeliveryClaims(
	ctx context.Context,
	queries dbstore.Queries,
	botID pgtype.UUID,
	sessionID pgtype.UUID,
	claims []sqlc.CompleteSessionEventDeliveryParams,
) error {
	if len(claims) == 0 {
		return nil
	}
	locker, ok := queries.(turnResponseDeliveryClaimLocker)
	if !ok {
		return errors.New("message store does not support delivery claim locking")
	}
	ordered := append([]sqlc.CompleteSessionEventDeliveryParams(nil), claims...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].EventID.String() < ordered[j].EventID.String()
	})
	for i, claim := range ordered {
		locked, err := locker.LockSessionEventDeliveryClaim(ctx, sqlc.LockSessionEventDeliveryClaimParams{
			EventID:    claim.EventID,
			BotID:      botID,
			SessionID:  sessionID,
			ClaimToken: claim.ClaimToken,
		})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !locked {
			return fmt.Errorf("delivery claim %d is stale", i)
		}
		if err != nil {
			return fmt.Errorf("lock delivery claim %d: %w", i, err)
		}
	}
	return nil
}

func (s *DBService) inAtomicTransaction(ctx context.Context, botID, sessionID string, fn func(dbstore.Queries) error) error {
	txer, ok := s.queries.(atomicTransactionalQueries)
	if !ok || !txer.SupportsTransactions() {
		return errors.New("message store does not support atomic transactions")
	}
	if _, fenced := runtimefence.FromContext(ctx); fenced {
		return runtimefence.InTransaction(ctx, s.queries, botID, sessionID, fn)
	}
	return txer.InTx(ctx, fn)
}

type ReplacementRoundRequest struct {
	Messages                 []PersistInput
	OldTurnID                string
	ExistingRequestMessageID string
	Reason                   string
}

// PersistTurnResponseTail writes multiple assistant/tool messages for one
// already-persisted user request in a single transaction.
func (s *DBService) PersistTurnResponseTail(ctx context.Context, inputs []PersistInput) ([]Message, error) {
	if err := validateTurnResponseTail(inputs); err != nil {
		return nil, err
	}
	if s == nil || s.queries == nil {
		return nil, errors.New("message service is not configured")
	}

	persisted := make([]Message, 0, len(inputs))
	if err := s.inAtomicTransaction(ctx, inputs[0].BotID, inputs[0].SessionID, func(queries dbstore.Queries) error {
		txService := *s
		txService.queries = queries
		for i, input := range inputs {
			message, err := txService.persist(ctx, input)
			if err != nil {
				return fmt.Errorf("persist tail message %d: %w", i, err)
			}
			persisted = append(persisted, message)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	for _, message := range persisted {
		s.publishMessageCreated(message)
	}
	return persisted, nil
}

func (s *DBService) PersistTurnResponseWithCursor(
	ctx context.Context,
	inputs []PersistInput,
	cursor DiscussCursorUpdate,
) ([]Message, error) {
	if err := validateTurnResponseWithCursor(inputs, cursor); err != nil {
		return nil, err
	}
	deliveryClaims, err := parseDeliveryClaims(cursor.DeliveryClaims)
	if err != nil {
		return nil, err
	}
	responseEvidenceIndex := lastResponseMessageIndex(inputs)
	if len(deliveryClaims) > 0 && responseEvidenceIndex < 0 {
		return nil, errors.New("delivery claims require an assistant or tool response")
	}
	if s == nil || s.queries == nil {
		return nil, errors.New("message service is not configured")
	}
	pgSessionID, err := dbpkg.ParseUUID(cursor.SessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid discuss cursor session id: %w", err)
	}
	pgRouteID := pgtype.UUID{}
	if strings.TrimSpace(cursor.RouteID) != "" {
		pgRouteID, err = dbpkg.ParseUUID(cursor.RouteID)
		if err != nil {
			return nil, fmt.Errorf("invalid discuss cursor route id: %w", err)
		}
	}
	scopeKey := strings.TrimSpace(cursor.ScopeKey)
	if scopeKey == "" {
		scopeKey = "default"
	}

	persisted := make([]Message, 0, len(inputs))
	if err := s.inAtomicTransaction(ctx, inputs[0].BotID, inputs[0].SessionID, func(queries dbstore.Queries) error {
		txService := *s
		txService.queries = queries
		requestMessageID := strings.TrimSpace(inputs[0].TurnRequestMessageID)
		for i, input := range inputs {
			input.TurnRequestMessageID = requestMessageID
			message, persistErr := txService.persist(ctx, input)
			if persistErr != nil {
				return fmt.Errorf("persist response message %d: %w", i, persistErr)
			}
			persisted = append(persisted, message)
			if strings.EqualFold(strings.TrimSpace(input.Role), "user") && !input.ContinueHistoryTurn {
				requestMessageID = message.ID
			}
		}
		if _, upsertErr := queries.UpsertSessionDiscussCursor(ctx, sqlc.UpsertSessionDiscussCursorParams{
			SessionID:           pgSessionID,
			ScopeKey:            scopeKey,
			RouteID:             pgRouteID,
			Source:              strings.TrimSpace(cursor.Source),
			ConsumedCursor:      cursor.ConsumedCursor,
			ConsumedEventCursor: cursor.ConsumedEventCursor,
		}); upsertErr != nil {
			return fmt.Errorf("persist discuss cursor: %w", upsertErr)
		}
		if len(deliveryClaims) > 0 {
			responseMessageID, parseErr := dbpkg.ParseUUID(persisted[responseEvidenceIndex].ID)
			if parseErr != nil {
				return fmt.Errorf("invalid durable response message id: %w", parseErr)
			}
			completer, ok := queries.(turnResponseDeliveryCompleter)
			if !ok {
				return errors.New("message store does not support atomic delivery completion")
			}
			for i, claim := range deliveryClaims {
				rows, completeErr := completer.CompleteSessionEventDeliveryWithResponse(ctx, sqlc.CompleteSessionEventDeliveryWithResponseParams{
					EventID:           claim.EventID,
					ClaimToken:        claim.ClaimToken,
					ResponseMessageID: responseMessageID,
				})
				if completeErr != nil {
					return fmt.Errorf("complete event delivery %d: %w", i, completeErr)
				}
				if rows != 1 {
					return fmt.Errorf("complete event delivery %d: no durable completion evidence", i)
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	for _, message := range persisted {
		s.publishMessageCreated(message)
	}
	return persisted, nil
}

func lastResponseMessageIndex(inputs []PersistInput) int {
	for i := len(inputs) - 1; i >= 0; i-- {
		role := strings.ToLower(strings.TrimSpace(inputs[i].Role))
		if role == "assistant" || role == "tool" {
			return i
		}
	}
	return -1
}

func validateClaimBackedRound(inputs []PersistInput) error {
	if lastResponseMessageIndex(inputs) < 0 {
		return errors.New("delivery claims require an assistant or tool response")
	}
	hasRequestBoundary := false
	for i, input := range inputs {
		if input.SkipHistoryTurn {
			return fmt.Errorf("delivery claim message %d skips history turn", i)
		}
		role := strings.ToLower(strings.TrimSpace(input.Role))
		if role == "user" && !input.ContinueHistoryTurn {
			hasRequestBoundary = true
		}
		if (role == "assistant" || role == "tool") && strings.TrimSpace(input.TurnRequestMessageID) != "" {
			hasRequestBoundary = true
		}
	}
	if !hasRequestBoundary {
		return errors.New("delivery claims require a durable request boundary")
	}
	return nil
}

func parseDeliveryClaims(claims []DeliveryClaim) ([]sqlc.CompleteSessionEventDeliveryParams, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	params := make([]sqlc.CompleteSessionEventDeliveryParams, 0, len(claims))
	seen := make(map[string]struct{}, len(claims))
	for i, claim := range claims {
		eventID, err := dbpkg.ParseUUID(strings.TrimSpace(claim.EventID))
		if err != nil {
			return nil, fmt.Errorf("invalid delivery claim %d event id: %w", i, err)
		}
		claimToken, err := dbpkg.ParseUUID(strings.TrimSpace(claim.ClaimToken))
		if err != nil {
			return nil, fmt.Errorf("invalid delivery claim %d token", i)
		}
		canonicalEventID := eventID.String()
		if _, ok := seen[canonicalEventID]; ok {
			return nil, fmt.Errorf("duplicate delivery claim event %q", canonicalEventID)
		}
		seen[canonicalEventID] = struct{}{}
		params = append(params, sqlc.CompleteSessionEventDeliveryParams{
			EventID:    eventID,
			ClaimToken: claimToken,
		})
	}
	return params, nil
}

func (s *DBService) PersistReplacementRound(ctx context.Context, input ReplacementRoundRequest) ([]Message, error) {
	if err := validateReplacementRound(input); err != nil {
		return nil, err
	}
	if s == nil || s.queries == nil {
		return nil, errors.New("message service is not configured")
	}
	persisted, handled, err := s.PersistRound(ctx, input.Messages, RoundPersistenceOptions{Replacement: &TurnReplacement{
		OldTurnID:        input.OldTurnID,
		RequestMessageID: input.ExistingRequestMessageID,
		Reason:           input.Reason,
	}})
	if err != nil {
		return nil, err
	}
	if !handled {
		return nil, errors.New("message service does not support atomic replacement round persistence")
	}
	return persisted, nil
}

func validateReplacementRound(input ReplacementRoundRequest) error {
	if len(input.Messages) == 0 {
		return errors.New("replacement round requires messages")
	}
	if strings.TrimSpace(input.OldTurnID) == "" {
		return errors.New("replacement round requires an old turn id")
	}
	botID := strings.TrimSpace(input.Messages[0].BotID)
	sessionID := strings.TrimSpace(input.Messages[0].SessionID)
	if botID == "" || sessionID == "" {
		return errors.New("replacement round requires bot and session ids")
	}
	for i, messageInput := range input.Messages {
		if strings.TrimSpace(messageInput.BotID) != botID || strings.TrimSpace(messageInput.SessionID) != sessionID {
			return fmt.Errorf("replacement message %d belongs to a different bot or session", i)
		}
		if !messageInput.SkipHistoryTurn {
			return fmt.Errorf("replacement message %d must skip history turn linkage", i)
		}
	}
	if strings.TrimSpace(input.ExistingRequestMessageID) == "" && !strings.EqualFold(strings.TrimSpace(input.Messages[0].Role), "user") {
		return errors.New("edit replacement must start with a user message")
	}
	assistantIndex := firstReplacementAssistantIndex(input.Messages)
	if assistantIndex < 0 {
		return errors.New("replacement round requires an assistant message")
	}
	for i := 0; i < assistantIndex; i++ {
		if !strings.EqualFold(strings.TrimSpace(input.Messages[i].Role), "user") {
			return fmt.Errorf("replacement message %d before the first assistant must be a user message", i)
		}
	}
	return nil
}

func firstReplacementAssistantIndex(messages []PersistInput) int {
	for i, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			return i
		}
	}
	return -1
}

func validateRound(inputs []PersistInput) error {
	if len(inputs) < 2 {
		return errors.New("round requires at least two messages")
	}
	botID := strings.TrimSpace(inputs[0].BotID)
	sessionID := strings.TrimSpace(inputs[0].SessionID)
	if botID == "" || sessionID == "" {
		return errors.New("round requires bot and session ids")
	}
	for i, input := range inputs {
		if input.ContinueHistoryTurn && !strings.EqualFold(strings.TrimSpace(input.Role), "user") {
			return fmt.Errorf("round message %d cannot continue a history turn", i)
		}
		if strings.TrimSpace(input.BotID) != botID || strings.TrimSpace(input.SessionID) != sessionID {
			return fmt.Errorf("round message %d belongs to a different bot or session", i)
		}
	}
	return nil
}

func validateTurnResponseTail(inputs []PersistInput) error {
	if len(inputs) < 2 {
		return errors.New("turn response tail requires at least two messages")
	}
	botID := strings.TrimSpace(inputs[0].BotID)
	sessionID := strings.TrimSpace(inputs[0].SessionID)
	requestID := strings.TrimSpace(inputs[0].TurnRequestMessageID)
	if botID == "" || sessionID == "" || requestID == "" {
		return errors.New("turn response tail requires bot, session, and request message ids")
	}
	for i, input := range inputs {
		role := strings.ToLower(strings.TrimSpace(input.Role))
		if role != "assistant" && role != "tool" {
			return fmt.Errorf("turn response tail message %d has unsupported role %q", i, input.Role)
		}
		if input.SkipHistoryTurn {
			return fmt.Errorf("turn response tail message %d skips history turn", i)
		}
		if strings.TrimSpace(input.BotID) != botID ||
			strings.TrimSpace(input.SessionID) != sessionID ||
			strings.TrimSpace(input.TurnRequestMessageID) != requestID {
			return fmt.Errorf("turn response tail message %d belongs to a different turn", i)
		}
	}
	return nil
}

func validateTurnResponseWithCursor(inputs []PersistInput, cursor DiscussCursorUpdate) error {
	if len(inputs) == 0 {
		return errors.New("turn response with cursor requires at least one message")
	}
	if strings.TrimSpace(cursor.SessionID) == "" || cursor.ConsumedEventCursor <= 0 {
		return errors.New("turn response cursor requires session and event cursor")
	}
	requestID := strings.TrimSpace(inputs[0].TurnRequestMessageID)
	botID := strings.TrimSpace(inputs[0].BotID)
	sessionID := strings.TrimSpace(inputs[0].SessionID)
	if botID == "" || sessionID == "" || requestID == "" || sessionID != strings.TrimSpace(cursor.SessionID) {
		return errors.New("turn response cursor does not match a durable request turn")
	}
	for i, input := range inputs {
		role := strings.ToLower(strings.TrimSpace(input.Role))
		if role != "user" && role != "assistant" && role != "tool" {
			return fmt.Errorf("turn response message %d has unsupported role %q", i, input.Role)
		}
		if input.SkipHistoryTurn || strings.TrimSpace(input.BotID) != botID ||
			strings.TrimSpace(input.SessionID) != sessionID {
			return fmt.Errorf("turn response message %d belongs to a different turn", i)
		}
		if input.ContinueHistoryTurn && role != "user" {
			return fmt.Errorf("turn response message %d cannot continue a history turn", i)
		}
	}
	return nil
}
