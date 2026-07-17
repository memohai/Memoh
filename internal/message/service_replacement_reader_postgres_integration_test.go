package message

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestPostgresPersistReplacementRoundKeepsFirstAssistantReadableAfterUserDecorations(t *testing.T) {
	tests := []struct {
		name                     string
		roles                    []string
		existingRequestMessageID bool
		assistantIndex           int
	}{
		{
			name:                     "retry",
			roles:                    []string{"user", "assistant"},
			existingRequestMessageID: true,
			assistantIndex:           1,
		},
		{
			name:           "edit",
			roles:          []string{"user", "user", "assistant"},
			assistantIndex: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx := beginPostgresMessageTestTx(t, ctx)
			setupPostgresMessageTestFixtures(t, ctx, tx)
			queries := &postgresAtomicReplacementQueries{
				Queries: postgresstore.NewQueries(dbsqlc.New(tx)),
				tx:      tx,
			}
			svc := NewService(nil, queries)
			oldUser := persistPostgresReplacementMessage(t, ctx, svc, "user", "old request", false)
			oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old response", false)
			oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
			if err != nil {
				t.Fatalf("get old turn: %v", err)
			}

			inputs := make([]PersistInput, 0, len(tt.roles))
			for i, role := range tt.roles {
				inputs = append(inputs, PersistInput{
					BotID:           postgresMessageTestBotID,
					SessionID:       postgresMessageTestSessionID,
					Role:            role,
					Content:         []byte(`{"role":"` + role + `","content":"replacement ` + string(rune('a'+i)) + `"}`),
					SkipHistoryTurn: true,
				})
			}
			existingRequestID := ""
			if tt.existingRequestMessageID {
				existingRequestID = oldUser.ID
			}
			persisted, err := svc.PersistReplacementRound(ctx, ReplacementRoundRequest{
				Messages:                 inputs,
				OldTurnID:                oldTurn.ID,
				ExistingRequestMessageID: existingRequestID,
				Reason:                   tt.name,
			})
			if err != nil {
				t.Fatalf("persist replacement round: %v", err)
			}
			requestID := oldUser.ID
			if !tt.existingRequestMessageID {
				requestID = persisted[0].ID
			}
			firstAssistantID := persisted[tt.assistantIndex].ID

			continuedUser, err := svc.Persist(ctx, PersistInput{
				BotID:                postgresMessageTestBotID,
				SessionID:            postgresMessageTestSessionID,
				Role:                 "user",
				Content:              []byte(`{"role":"user","content":"continued"}`),
				TurnRequestMessageID: requestID,
				ContinueHistoryTurn:  true,
			})
			if err != nil {
				t.Fatalf("persist continued user: %v", err)
			}
			followupAssistant, err := svc.Persist(ctx, PersistInput{
				BotID:                postgresMessageTestBotID,
				SessionID:            postgresMessageTestSessionID,
				Role:                 "assistant",
				Content:              []byte(`{"role":"assistant","content":"continued response"}`),
				TurnRequestMessageID: requestID,
			})
			if err != nil {
				t.Fatalf("persist continued assistant: %v", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE bot_history_messages
				SET created_at = created_at + interval '1 hour'
				WHERE id = $1
			`, firstAssistantID); err != nil {
				t.Fatalf("move first assistant timestamp after followup: %v", err)
			}
			visible, err := svc.ListBySession(ctx, postgresMessageTestSessionID)
			if err != nil {
				t.Fatalf("list visible messages: %v", err)
			}
			wantIDs := []string{requestID}
			persistedStart := 0
			if !tt.existingRequestMessageID {
				persistedStart = 1
			}
			for _, message := range persisted[persistedStart:] {
				wantIDs = append(wantIDs, message.ID)
			}
			wantIDs = append(wantIDs, continuedUser.ID, followupAssistant.ID)
			if len(visible) != len(wantIDs) {
				t.Fatalf("visible messages = %d, want %d", len(visible), len(wantIDs))
			}
			for i, message := range visible {
				if message.ID != wantIDs[i] {
					t.Fatalf("visible message %d = %s, want %s", i, message.ID, wantIDs[i])
				}
			}

			for _, messageID := range []string{requestID, firstAssistantID, continuedUser.ID, followupAssistant.ID} {
				turn, getErr := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, messageID)
				if getErr != nil {
					t.Fatalf("get visible turn for %s: %v", messageID, getErr)
				}
				if turn.RequestMessageID != requestID || turn.AssistantMessageID != firstAssistantID {
					t.Fatalf("turn anchors for %s = %s/%s, want %s/%s", messageID, turn.RequestMessageID, turn.AssistantMessageID, requestID, firstAssistantID)
				}
			}
			latest, err := svc.GetLatestVisibleTurnBySession(ctx, postgresMessageTestSessionID)
			if err != nil {
				t.Fatalf("get latest visible turn: %v", err)
			}
			if latest.RequestMessageID != requestID || latest.AssistantMessageID != firstAssistantID {
				t.Fatalf("latest turn anchors = %s/%s, want %s/%s", latest.RequestMessageID, latest.AssistantMessageID, requestID, firstAssistantID)
			}
			wantTurnMessages := make([]postgresReplacementTurnMessage, 0, len(visible))
			for i, message := range visible {
				wantTurnMessages = append(wantTurnMessages, postgresReplacementTurnMessage{
					ID:       message.ID,
					Role:     message.Role,
					Sequence: int64(i + 1),
				})
			}
			assertPostgresReplacementTurnOrder(t, ctx, tx, latest.ID, wantTurnMessages...)

			sqlQueries := dbsqlc.New(tx)
			latestTurnID := mustTestUUID(t, latest.ID)
			byID, err := sqlQueries.GetHistoryTurnByID(ctx, dbsqlc.GetHistoryTurnByIDParams{
				OldTurnID: latestTurnID,
				SessionID: mustTestUUID(t, postgresMessageTestSessionID),
			})
			if err != nil {
				t.Fatalf("get history turn by id: %v", err)
			}
			if got := byID.AssistantMessageID.String(); got != firstAssistantID {
				t.Fatalf("history turn assistant = %s, want %s", got, firstAssistantID)
			}
			listed, err := sqlQueries.ListHistoryTurnsByBot(ctx, mustTestUUID(t, postgresMessageTestBotID))
			if err != nil {
				t.Fatalf("list history turns: %v", err)
			}
			foundLatest := false
			for _, turn := range listed {
				if turn.ID != latestTurnID {
					continue
				}
				foundLatest = true
				if got := turn.AssistantMessageID.String(); got != firstAssistantID {
					t.Fatalf("listed history turn assistant = %s, want %s", got, firstAssistantID)
				}
			}
			if !foundLatest {
				t.Fatalf("latest history turn %s was not listed", latest.ID)
			}
			superseded, err := sqlQueries.SupersedeHistoryTurn(ctx, dbsqlc.SupersedeHistoryTurnParams{
				SessionID:          mustTestUUID(t, postgresMessageTestSessionID),
				SupersededByTurnID: mustTestUUID(t, "99999999-9999-9999-9999-999999999999"),
				SupersededAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				SupersededReason:   pgtype.Text{String: "test", Valid: true},
				OldTurnID:          latestTurnID,
			})
			if err != nil {
				t.Fatalf("supersede history turn: %v", err)
			}
			if got := superseded.AssistantMessageID.String(); got != firstAssistantID {
				t.Fatalf("superseded history turn assistant = %s, want %s", got, firstAssistantID)
			}
		})
	}
}

func TestPostgresReplaceHistoryTurnRejectsLaterAssistantAnchor(t *testing.T) {
	ctx := context.Background()
	tx, svc := newPostgresReplacementOrderService(t, ctx)
	request := persistPostgresReplacementMessage(t, ctx, svc, "user", "request", false)
	oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old response", false)
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	decoration := persistPostgresReplacementMessage(t, ctx, svc, "user", "decoration", true)
	firstAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "first response", true)
	laterAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "later response", true)

	_, err = dbsqlc.New(tx).ReplaceHistoryTurn(ctx, dbsqlc.ReplaceHistoryTurnParams{
		SessionID: mustTestUUID(t, postgresMessageTestSessionID),
		OldTurnID: mustTestUUID(t, oldTurn.ID),
		ReplacementMessageIds: []pgtype.UUID{
			mustTestUUID(t, request.ID),
			mustTestUUID(t, decoration.ID),
			mustTestUUID(t, firstAssistant.ID),
			mustTestUUID(t, laterAssistant.ID),
		},
		RequestMessageID:   mustTestUUID(t, request.ID),
		AssistantMessageID: mustTestUUID(t, laterAssistant.ID),
		SupersededAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		SupersededReason:   pgtype.Text{String: "retry", Valid: true},
	})
	if err == nil {
		t.Fatal("ReplaceHistoryTurn() error = nil, want later assistant anchor rejection")
	}
	assertPostgresVisibleMessageIDs(t, ctx, svc, request.ID, oldAssistant.ID)
	for _, messageID := range []string{decoration.ID, firstAssistant.ID, laterAssistant.ID} {
		assertPostgresReplacementMessageUnlinked(t, ctx, tx, messageID)
	}
}
