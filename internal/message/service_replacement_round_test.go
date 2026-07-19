package message

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestPersistReplacementRoundRollsBackWhenReplaceFails(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("replace history turn")
	queries := &replacementRoundQueries{replaceErr: wantErr}
	publisher := &recordingPublisher{}
	service := NewService(nil, queries, publisher)

	_, err := service.PersistReplacementRound(context.Background(), ReplacementRoundRequest{
		Messages:                 replacementRoundInputs("assistant"),
		OldTurnID:                "33333333-3333-3333-3333-333333333333",
		ExistingRequestMessageID: "44444444-4444-4444-4444-444444444444",
		Reason:                   "retry",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("PersistReplacementRound() error = %v, want %v", err, wantErr)
	}
	if queries.txCalls != 1 {
		t.Fatalf("transaction calls = %d, want 1", queries.txCalls)
	}
	if len(queries.committedRoles) != 0 || queries.committedReplacement != nil {
		t.Fatalf("committed messages/replacement = %#v/%#v, want rollback", queries.committedRoles, queries.committedReplacement)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0", len(publisher.events))
	}
}

func TestPersistReplacementRoundCommitsExactRetryAndEditSequences(t *testing.T) {
	t.Parallel()

	const existingRequestID = "44444444-4444-4444-4444-444444444444"
	tests := []struct {
		name                     string
		roles                    []string
		existingRequestMessageID string
		wantMessageIDs           []string
		wantAssistantMessageID   string
	}{
		{
			name:                     "single assistant retry",
			roles:                    []string{"assistant"},
			existingRequestMessageID: existingRequestID,
			wantMessageIDs:           []string{existingRequestID, mixedRoundMessageID(1)},
			wantAssistantMessageID:   mixedRoundMessageID(1),
		},
		{
			name:                     "retry with user decoration before assistant",
			roles:                    []string{"user", "assistant"},
			existingRequestMessageID: existingRequestID,
			wantMessageIDs:           []string{existingRequestID, mixedRoundMessageID(1), mixedRoundMessageID(2)},
			wantAssistantMessageID:   mixedRoundMessageID(2),
		},
		{
			name:                   "edit with tool tail",
			roles:                  []string{"user", "assistant", "tool", "assistant"},
			wantMessageIDs:         []string{mixedRoundMessageID(1), mixedRoundMessageID(2), mixedRoundMessageID(3), mixedRoundMessageID(4)},
			wantAssistantMessageID: mixedRoundMessageID(2),
		},
		{
			name:                   "edit with user decoration before assistant",
			roles:                  []string{"user", "user", "assistant"},
			wantMessageIDs:         []string{mixedRoundMessageID(1), mixedRoundMessageID(2), mixedRoundMessageID(3)},
			wantAssistantMessageID: mixedRoundMessageID(3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			queries := &replacementRoundQueries{}
			publisher := &recordingPublisher{}
			service := NewService(nil, queries, publisher)

			messages, err := service.PersistReplacementRound(context.Background(), ReplacementRoundRequest{
				Messages:                 replacementRoundInputs(tt.roles...),
				OldTurnID:                "33333333-3333-3333-3333-333333333333",
				ExistingRequestMessageID: tt.existingRequestMessageID,
				Reason:                   "replace",
			})
			if err != nil {
				t.Fatalf("PersistReplacementRound() error = %v", err)
			}
			if len(messages) != len(tt.roles) || fmt.Sprint(queries.committedRoles) != fmt.Sprint(tt.roles) {
				t.Fatalf("persisted/committed roles = %d/%v, want %d/%v", len(messages), queries.committedRoles, len(tt.roles), tt.roles)
			}
			if queries.committedReplacement == nil {
				t.Fatal("replacement was not committed")
			}
			gotMessageIDs := make([]string, 0, len(queries.committedReplacement.ReplacementMessageIds))
			for _, id := range queries.committedReplacement.ReplacementMessageIds {
				gotMessageIDs = append(gotMessageIDs, id.String())
			}
			if fmt.Sprint(gotMessageIDs) != fmt.Sprint(tt.wantMessageIDs) {
				t.Fatalf("replacement message ids = %v, want %v", gotMessageIDs, tt.wantMessageIDs)
			}
			if got := queries.committedReplacement.AssistantMessageID.String(); got != tt.wantAssistantMessageID {
				t.Fatalf("replacement assistant id = %q, want %q", got, tt.wantAssistantMessageID)
			}
			if len(publisher.events) != 0 {
				t.Fatalf("published events = %d, want flow-owned publication", len(publisher.events))
			}
		})
	}
}

func TestPersistReplacementRoundRejectsInvalidPreAssistantMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		roles                    []string
		existingRequestMessageID string
		wantErr                  string
	}{
		{
			name:                     "retry without assistant",
			roles:                    []string{"user"},
			existingRequestMessageID: "44444444-4444-4444-4444-444444444444",
			wantErr:                  "replacement round requires an assistant message",
		},
		{
			name:                     "retry tool before assistant",
			roles:                    []string{"tool", "assistant"},
			existingRequestMessageID: "44444444-4444-4444-4444-444444444444",
			wantErr:                  "replacement message 0 before the first assistant must be a user message",
		},
		{
			name:    "edit without assistant",
			roles:   []string{"user", "user"},
			wantErr: "replacement round requires an assistant message",
		},
		{
			name:    "edit tool before assistant",
			roles:   []string{"user", "tool", "assistant"},
			wantErr: "replacement message 1 before the first assistant must be a user message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			queries := &replacementRoundQueries{}
			service := NewService(nil, queries, &recordingPublisher{})

			_, err := service.PersistReplacementRound(context.Background(), ReplacementRoundRequest{
				Messages:                 replacementRoundInputs(tt.roles...),
				OldTurnID:                "33333333-3333-3333-3333-333333333333",
				ExistingRequestMessageID: tt.existingRequestMessageID,
				Reason:                   "replace",
			})
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("PersistReplacementRound() error = %v, want %q", err, tt.wantErr)
			}
			if queries.txCalls != 0 {
				t.Fatalf("transaction calls = %d, want fail closed before persistence", queries.txCalls)
			}
		})
	}
}

func replacementRoundInputs(roles ...string) []PersistInput {
	inputs := make([]PersistInput, 0, len(roles))
	for _, role := range roles {
		inputs = append(inputs, PersistInput{
			BotID:           "11111111-1111-1111-1111-111111111111",
			SessionID:       "22222222-2222-2222-2222-222222222222",
			Role:            role,
			Content:         []byte(`{"role":"` + role + `","content":"replacement"}`),
			SessionMode:     "chat",
			RuntimeType:     "model",
			SkipHistoryTurn: true,
		})
	}
	return inputs
}

type replacementRoundQueries struct {
	dbstore.Queries
	replaceErr           error
	txCalls              int
	committedRoles       []string
	committedReplacement *sqlc.ReplaceHistoryTurnParams
}

func (*replacementRoundQueries) SupportsTransactions() bool { return true }

func (q *replacementRoundQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.txCalls++
	tx := &replacementRoundTxQueries{replaceErr: q.replaceErr}
	if err := fn(tx); err != nil {
		return err
	}
	q.committedRoles = append(q.committedRoles, tx.stagedRoles...)
	q.committedReplacement = tx.stagedReplacement
	return nil
}

type replacementRoundTxQueries struct {
	dbstore.Queries
	replaceErr        error
	stagedRoles       []string
	stagedReplacement *sqlc.ReplaceHistoryTurnParams
}

func (q *replacementRoundTxQueries) CreateMessage(_ context.Context, arg sqlc.CreateMessageParams) (sqlc.CreateMessageRow, error) {
	q.stagedRoles = append(q.stagedRoles, arg.Role)
	return sqlc.CreateMessageRow{
		ID:        testMessageUUID(mixedRoundMessageID(len(q.stagedRoles))),
		BotID:     arg.BotID,
		SessionID: arg.SessionID,
		Role:      arg.Role,
		Content:   arg.Content,
		CreatedAt: pgtype.Timestamptz{Time: time.Unix(int64(len(q.stagedRoles)), 0), Valid: true},
	}, nil
}

func (q *replacementRoundTxQueries) ReplaceHistoryTurn(_ context.Context, arg sqlc.ReplaceHistoryTurnParams) (sqlc.ReplaceHistoryTurnRow, error) {
	q.stagedReplacement = &arg
	if q.replaceErr != nil {
		return sqlc.ReplaceHistoryTurnRow{}, q.replaceErr
	}
	return sqlc.ReplaceHistoryTurnRow{
		ID:                 testMessageUUID("55555555-5555-5555-5555-555555555555"),
		BotID:              arg.ReplacementMessageIds[0],
		SessionID:          arg.SessionID,
		RequestMessageID:   arg.RequestMessageID,
		AssistantMessageID: arg.AssistantMessageID,
		CreatedAt:          pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true},
		UpdatedAt:          pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true},
	}, nil
}
