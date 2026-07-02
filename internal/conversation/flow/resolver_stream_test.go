package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type recordingMessageService struct {
	persisted     []messagepkg.PersistInput
	activeBot     []string
	activeSession []string
	activeTurn    []string
}

func (s *recordingMessageService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persisted = append(s.persisted, input)
	turnID := strings.TrimSpace(input.TurnID)
	if turnID == "" && strings.TrimSpace(input.SessionID) != "" {
		turnID = "33333333-3333-3333-3333-333333333333"
	}
	return messagepkg.Message{ID: "message-id", Role: input.Role, TurnID: turnID}, nil
}

func (*recordingMessageService) List(context.Context, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *recordingMessageService) ListActiveSince(_ context.Context, botID string, _ time.Time) ([]messagepkg.Message, error) {
	s.activeBot = append(s.activeBot, botID)
	return nil, nil
}

func (*recordingMessageService) ListLatest(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBefore(context.Context, string, time.Time, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBySession(context.Context, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *recordingMessageService) ListActiveSinceBySession(_ context.Context, sessionID string, _ time.Time) ([]messagepkg.Message, error) {
	s.activeSession = append(s.activeSession, sessionID)
	return nil, nil
}

func (s *recordingMessageService) ListActiveSinceByTurn(_ context.Context, turnID string, _ time.Time) ([]messagepkg.Message, error) {
	s.activeTurn = append(s.activeTurn, turnID)
	return nil, nil
}

func (*recordingMessageService) ListLatestBySession(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBeforeBySession(context.Context, string, time.Time, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) LocateByExternalIDBySession(context.Context, string, string, int32, int32) (messagepkg.LocateResult, error) {
	return messagepkg.LocateResult{}, nil
}

func (*recordingMessageService) DeleteByBot(context.Context, string) error {
	return nil
}

func (*recordingMessageService) DeleteBySession(context.Context, string) error {
	return nil
}

func (*recordingMessageService) LinkAssets(context.Context, string, []messagepkg.AssetRef) error {
	return nil
}

type recordingTxMessageService struct {
	recordingMessageService
	txPersisted []messagepkg.PersistInput
	published   []messagepkg.Message
}

func (s *recordingTxMessageService) PersistWithQueries(_ context.Context, _ dbstore.Queries, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.txPersisted = append(s.txPersisted, input)
	return messagepkg.Message{
		ID:        "message-id-" + input.Role,
		BotID:     input.BotID,
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Role:      input.Role,
	}, nil
}

func (s *recordingTxMessageService) PublishMessageCreated(message messagepkg.Message) {
	s.published = append(s.published, message)
}

type fakeTurnStore struct {
	dbstore.Queries
	session          dbsqlc.BotSession
	rewrite          dbsqlc.GetVisibleUserMessageTurnForRewriteRow
	sessionHeadErr   error
	historyTurn      dbsqlc.BotHistoryTurn
	historyTurnErr   error
	turnPath         []dbsqlc.BotHistoryTurn
	turnPathErr      error
	updateDefaultErr error
	createdTurns     []dbsqlc.CreateHistoryTurnParams
	createdHeads     []dbsqlc.CreateSessionTurnHeadParams
	replacedHeads    []dbsqlc.ReplaceSessionTurnHeadParams
	updatedDefault   []dbsqlc.UpdateSessionDefaultHeadTurnIfValidParams
	nextTurnByte     byte
	txCount          int
}

func (s *fakeTurnStore) RunInTx(_ context.Context, fn func(dbstore.Queries) error) error {
	s.txCount++
	return fn(s)
}

func (s *fakeTurnStore) CreateHistoryTurn(_ context.Context, arg dbsqlc.CreateHistoryTurnParams) (dbsqlc.BotHistoryTurn, error) {
	s.createdTurns = append(s.createdTurns, arg)
	s.nextTurnByte++
	return dbsqlc.BotHistoryTurn{ID: testUUID(s.nextTurnByte)}, nil
}

func (s *fakeTurnStore) GetSessionByID(context.Context, pgtype.UUID) (dbsqlc.BotSession, error) {
	return s.session, nil
}

func (s *fakeTurnStore) GetHistoryTurnByID(_ context.Context, id pgtype.UUID) (dbsqlc.BotHistoryTurn, error) {
	if s.historyTurnErr != nil {
		return dbsqlc.BotHistoryTurn{}, s.historyTurnErr
	}
	if s.historyTurn.ID.Valid {
		return s.historyTurn, nil
	}
	return dbsqlc.BotHistoryTurn{ID: id, BotID: s.session.BotID, OwnerSessionID: s.session.ID}, nil
}

func (s *fakeTurnStore) ListHistoryTurnPathFromHead(_ context.Context, headTurnID pgtype.UUID) ([]dbsqlc.BotHistoryTurn, error) {
	if s.turnPathErr != nil {
		return nil, s.turnPathErr
	}
	if s.turnPath != nil {
		return s.turnPath, nil
	}
	return []dbsqlc.BotHistoryTurn{{ID: headTurnID, BotID: s.session.BotID, OwnerSessionID: s.session.ID}}, nil
}

func (s *fakeTurnStore) CreateSessionTurnHead(_ context.Context, arg dbsqlc.CreateSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error) {
	s.createdHeads = append(s.createdHeads, arg)
	return dbsqlc.BotSessionTurnHead{SessionID: arg.SessionID, HeadTurnID: arg.HeadTurnID}, nil
}

func (s *fakeTurnStore) GetSessionTurnHead(_ context.Context, arg dbsqlc.GetSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error) {
	if s.sessionHeadErr != nil {
		return dbsqlc.BotSessionTurnHead{}, s.sessionHeadErr
	}
	return dbsqlc.BotSessionTurnHead{SessionID: arg.SessionID, HeadTurnID: arg.HeadTurnID}, nil
}

func (s *fakeTurnStore) GetVisibleUserMessageTurnForRewrite(context.Context, dbsqlc.GetVisibleUserMessageTurnForRewriteParams) (dbsqlc.GetVisibleUserMessageTurnForRewriteRow, error) {
	return s.rewrite, nil
}

func (s *fakeTurnStore) ReplaceSessionTurnHead(_ context.Context, arg dbsqlc.ReplaceSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error) {
	s.replacedHeads = append(s.replacedHeads, arg)
	return dbsqlc.BotSessionTurnHead{SessionID: arg.TargetSessionID, HeadTurnID: arg.NewHeadTurnID}, nil
}

func (s *fakeTurnStore) UpdateSessionDefaultHeadTurnIfValid(_ context.Context, arg dbsqlc.UpdateSessionDefaultHeadTurnIfValidParams) (dbsqlc.BotSession, error) {
	if s.updateDefaultErr != nil {
		return dbsqlc.BotSession{}, s.updateDefaultErr
	}
	s.updatedDefault = append(s.updatedDefault, arg)
	return dbsqlc.BotSession{ID: arg.ID, Type: "chat", DefaultHeadTurnID: arg.DefaultHeadTurnID}, nil
}

func testUUID(lastByte byte) pgtype.UUID {
	id := pgtype.UUID{Valid: true}
	id.Bytes[15] = lastByte
	return id
}

func TestResolveTurnRunParentNormalFirstTurnUsesEmptyContext(t *testing.T) {
	t.Parallel()

	store := &fakeTurnStore{}
	mode, parent, baseHead, origin, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{}, nil)
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if mode != TurnRunModeNormal {
		t.Fatalf("mode = %q, want %q", mode, TurnRunModeNormal)
	}
	if parent.Valid || baseHead.HeadTurnID.Valid {
		t.Fatalf("parent=%v baseHead=%v, want both invalid", parent, baseHead)
	}
	if origin.Kind != conversation.TurnOriginMessage || origin.TurnID != "" || origin.RequestGroupID != "" {
		t.Fatalf("origin = %#v, want plain message origin", origin)
	}
	if scope := contextScopeFromParentTurn(parent); scope.Kind != ContextScopeEmpty {
		t.Fatalf("scope = %#v, want empty", scope)
	}
}

func TestResolveTurnRunParentNormalLaterTurnUsesSessionHead(t *testing.T) {
	t.Parallel()

	head := testUUID(9)
	store := &fakeTurnStore{session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: head}}
	mode, parent, baseHead, _, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{}, nil)
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if mode != TurnRunModeNormal {
		t.Fatalf("mode = %q, want %q", mode, TurnRunModeNormal)
	}
	if parent != head || baseHead.HeadTurnID != head {
		t.Fatalf("parent=%v baseHead=%v, want head=%v", parent, baseHead, head)
	}
	scope := contextScopeFromParentTurn(parent)
	if scope.Kind != ContextScopeTurnHead || scope.TurnID != head.String() {
		t.Fatalf("scope = %#v, want turn head %s", scope, head.String())
	}
}

func TestResolveTurnRunParentNonChatIgnoresRequestedVariantHead(t *testing.T) {
	t.Parallel()

	defaultHead := testUUID(9)
	requestedHead := testUUID(10)
	store := &fakeTurnStore{session: dbsqlc.BotSession{Type: sessionpkg.TypeDiscuss, DefaultHeadTurnID: defaultHead}}
	mode, parent, baseHead, _, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{
		BaseHeadTurnID: requestedHead.String(),
	}, nil)
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if mode != TurnRunModeNormal {
		t.Fatalf("mode = %q, want %q", mode, TurnRunModeNormal)
	}
	if parent != defaultHead || baseHead.HeadTurnID != defaultHead {
		t.Fatalf("parent=%v baseHead=%v, want default head=%v", parent, baseHead, defaultHead)
	}
	if len(store.replacedHeads) != 0 || len(store.createdHeads) != 0 {
		t.Fatalf("turn head mutations during resolve: replaced=%#v created=%#v", store.replacedHeads, store.createdHeads)
	}
}

func TestResolveTurnRunParentRewriteFirstTurnUsesEmptyContext(t *testing.T) {
	t.Parallel()

	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: testUUID(7)},
		rewrite: dbsqlc.GetVisibleUserMessageTurnForRewriteRow{
			ParentTurnID: pgtype.UUID{},
		},
	}
	mode, parent, baseHead, origin, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{
		RewriteTargetMessageID: "00000000-0000-0000-0000-000000000123",
	}, nil)
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if mode != TurnRunModeRewrite {
		t.Fatalf("mode = %q, want %q", mode, TurnRunModeRewrite)
	}
	if origin.Kind != conversation.TurnOriginEdit {
		t.Fatalf("origin kind = %q, want %q", origin.Kind, conversation.TurnOriginEdit)
	}
	if parent.Valid {
		t.Fatalf("parent = %v, want invalid for first-turn rewrite", parent)
	}
	if !baseHead.HeadTurnID.Valid || baseHead.HeadTurnID != testUUID(7) {
		t.Fatalf("baseHead = %v, want current session head", baseHead)
	}
	if scope := contextScopeFromParentTurn(parent); scope.Kind != ContextScopeEmpty {
		t.Fatalf("scope = %#v, want empty", scope)
	}
}

func TestResolveTurnRunParentNonChatRejectsRewrite(t *testing.T) {
	t.Parallel()

	currentHead := testUUID(8)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: sessionpkg.TypeDiscuss, DefaultHeadTurnID: currentHead},
		rewrite: dbsqlc.GetVisibleUserMessageTurnForRewriteRow{
			ParentTurnID: testUUID(4),
		},
	}
	_, _, _, _, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{
		RewriteTargetMessageID: "00000000-0000-0000-0000-000000000123",
	}, nil)
	if err == nil {
		t.Fatalf("resolveTurnRunParent() error = nil, want non-chat variant error")
	}
	if !strings.Contains(err.Error(), "turn variants are only supported for chat sessions") {
		t.Fatalf("resolveTurnRunParent() error = %v, want non-chat variant error", err)
	}
}

func TestResolveTurnRunParentRewriteMiddleTurnUsesTargetParent(t *testing.T) {
	t.Parallel()

	currentHead := testUUID(8)
	targetParent := testUUID(4)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: currentHead},
		rewrite: dbsqlc.GetVisibleUserMessageTurnForRewriteRow{
			ParentTurnID: targetParent,
		},
	}
	mode, parent, baseHead, origin, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{
		RewriteTargetMessageID: "00000000-0000-0000-0000-000000000123",
	}, nil)
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if mode != TurnRunModeRewrite {
		t.Fatalf("mode = %q, want %q", mode, TurnRunModeRewrite)
	}
	if origin.Kind != conversation.TurnOriginEdit {
		t.Fatalf("origin kind = %q, want %q", origin.Kind, conversation.TurnOriginEdit)
	}
	if parent != targetParent {
		t.Fatalf("parent = %v, want target parent %v", parent, targetParent)
	}
	if baseHead.HeadTurnID != currentHead {
		t.Fatalf("baseHead = %v, want current head %v", baseHead, currentHead)
	}
	scope := contextScopeFromParentTurn(parent)
	if scope.Kind != ContextScopeTurnHead || scope.TurnID != targetParent.String() {
		t.Fatalf("scope = %#v, want target parent turn head %s", scope, targetParent.String())
	}
}

func TestResolveTurnRunParentRewriteUsesResolvedTurnAnchor(t *testing.T) {
	t.Parallel()

	currentHead := testUUID(8)
	targetParent := testUUID(4)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: currentHead},
		rewrite: dbsqlc.GetVisibleUserMessageTurnForRewriteRow{
			ParentTurnID: testUUID(99),
		},
	}
	mode, parent, baseHead, origin, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{}, &conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleUser,
		MessageID:      "message-1",
		TurnID:         testUUID(3).String(),
		ParentTurnID:   targetParent.String(),
		BaseHeadTurnID: currentHead.String(),
	})
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if mode != TurnRunModeRewrite {
		t.Fatalf("mode = %q, want %q", mode, TurnRunModeRewrite)
	}
	if parent != targetParent {
		t.Fatalf("parent = %v, want anchor parent %v", parent, targetParent)
	}
	if baseHead.HeadTurnID != currentHead {
		t.Fatalf("baseHead = %v, want anchor base head %v", baseHead, currentHead)
	}
	// An anchor without an explicit origin kind defaults to edit and records
	// the anchored turn as the origin turn.
	if origin.Kind != conversation.TurnOriginEdit || origin.TurnID != testUUID(3).String() {
		t.Fatalf("origin = %#v, want edit origin anchored on %s", origin, testUUID(3).String())
	}
}

func TestResolveTurnRunParentRetryAnchorKeepsRequestGroup(t *testing.T) {
	t.Parallel()

	currentHead := testUUID(8)
	targetParent := testUUID(4)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: currentHead},
	}
	_, _, _, origin, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{}, &conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleUser,
		MessageID:      "message-1",
		TurnID:         testUUID(3).String(),
		ParentTurnID:   targetParent.String(),
		BaseHeadTurnID: currentHead.String(),
		OriginKind:     conversation.TurnOriginRetry,
		RequestGroupID: testUUID(5).String(),
	})
	if err != nil {
		t.Fatalf("resolveTurnRunParent() error = %v", err)
	}
	if origin.Kind != conversation.TurnOriginRetry {
		t.Fatalf("origin kind = %q, want %q", origin.Kind, conversation.TurnOriginRetry)
	}
	if origin.TurnID != testUUID(3).String() {
		t.Fatalf("origin turn = %q, want %q", origin.TurnID, testUUID(3).String())
	}
	if origin.RequestGroupID != testUUID(5).String() {
		t.Fatalf("origin request group = %q, want %q", origin.RequestGroupID, testUUID(5).String())
	}
}

func TestResolveTurnRunParentRewriteAnchorRejectsInvalidBaseHead(t *testing.T) {
	t.Parallel()

	currentHead := testUUID(8)
	targetParent := testUUID(4)
	store := &fakeTurnStore{
		session:        dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: currentHead},
		sessionHeadErr: pgx.ErrNoRows,
	}
	_, _, _, _, err := (*Resolver)(nil).resolveTurnRunParent(context.Background(), store, testUUID(1), conversation.ChatRequest{}, &conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleUser,
		MessageID:      "message-1",
		TurnID:         testUUID(3).String(),
		ParentTurnID:   targetParent.String(),
		BaseHeadTurnID: currentHead.String(),
	})
	if err == nil {
		t.Fatalf("resolveTurnRunParent() error = nil, want invalid base head error")
	}
	if !strings.Contains(err.Error(), "base head is not valid for this session") {
		t.Fatalf("resolveTurnRunParent() error = %v, want invalid base head", err)
	}
}

func TestApplyVariantTransitionReplaceBaseHeadReplacesBaseHead(t *testing.T) {
	t.Parallel()

	store := &fakeTurnStore{}
	resolver := &Resolver{queries: store}
	sessionID := testUUID(1).String()
	oldHead := testUUID(2)
	newHead := testUUID(3)

	run := TurnRun{
		Mode:          TurnRunModeNormal,
		PersistTurnID: newHead.String(),
		Variant: VariantTransition{
			Action:         VariantTransitionReplaceBaseHead,
			SessionID:      sessionID,
			BaseHeadTurnID: oldHead.String(),
		},
	}
	err := resolver.applyVariantTransition(context.Background(), &run, "")
	if err != nil {
		t.Fatalf("applyVariantTransition() error = %v", err)
	}
	if store.txCount != 1 {
		t.Fatalf("txCount = %d, want 1", store.txCount)
	}
	if len(store.replacedHeads) != 1 {
		t.Fatalf("replacedHeads = %d, want 1", len(store.replacedHeads))
	}
	if got := store.replacedHeads[0]; got.OldHeadTurnID != oldHead || got.NewHeadTurnID != newHead {
		t.Fatalf("replace params = %#v, want old=%v new=%v", got, oldHead, newHead)
	}
	if len(store.createdHeads) != 0 {
		t.Fatalf("createdHeads = %d, want 0", len(store.createdHeads))
	}
	if len(store.updatedDefault) != 1 || store.updatedDefault[0].DefaultHeadTurnID != newHead {
		t.Fatalf("updatedDefault = %#v, want new head", store.updatedDefault)
	}
}

func TestApplyVariantTransitionCreateVariantCreatesSiblingHead(t *testing.T) {
	t.Parallel()

	store := &fakeTurnStore{}
	resolver := &Resolver{queries: store}
	sessionID := testUUID(1).String()
	baseHead := testUUID(2)
	newHead := testUUID(3)

	run := TurnRun{
		Mode:          TurnRunModeRewrite,
		PersistTurnID: newHead.String(),
		Variant: VariantTransition{
			Action:         VariantTransitionCreateSibling,
			SessionID:      sessionID,
			BaseHeadTurnID: baseHead.String(),
		},
	}
	err := resolver.applyVariantTransition(context.Background(), &run, "")
	if err != nil {
		t.Fatalf("applyVariantTransition() error = %v", err)
	}
	if store.txCount != 1 {
		t.Fatalf("txCount = %d, want 1", store.txCount)
	}
	if len(store.createdHeads) != 1 || store.createdHeads[0].HeadTurnID != newHead {
		t.Fatalf("createdHeads = %#v, want new head", store.createdHeads)
	}
	if len(store.replacedHeads) != 0 {
		t.Fatalf("replacedHeads = %d, want 0", len(store.replacedHeads))
	}
	if len(store.updatedDefault) != 1 || store.updatedDefault[0].DefaultHeadTurnID != newHead {
		t.Fatalf("updatedDefault = %#v, want new head", store.updatedDefault)
	}
}

func TestContinuationTurnRunDoesNotMoveSessionHead(t *testing.T) {
	t.Parallel()

	store := &fakeTurnStore{}
	resolver := &Resolver{queries: store}
	sessionID := testUUID(1).String()
	turnID := testUUID(2)

	run := continuationTurnRun(sessionID, turnID.String())
	err := resolver.applyVariantTransition(context.Background(), &run, "")
	if err != nil {
		t.Fatalf("applyVariantTransition() error = %v", err)
	}
	if store.txCount != 0 {
		t.Fatalf("txCount = %d, want 0", store.txCount)
	}
	if run.PersistTurnID != turnID.String() {
		t.Fatalf("PersistTurnID = %q, want %q", run.PersistTurnID, turnID.String())
	}
	if run.Context.Kind != ContextScopeTurnHead || run.Context.TurnID != turnID.String() {
		t.Fatalf("Context = %#v, want same turn context", run.Context)
	}
	if len(store.createdHeads) != 0 {
		t.Fatalf("createdHeads = %d, want 0", len(store.createdHeads))
	}
	if len(store.replacedHeads) != 0 {
		t.Fatalf("replacedHeads = %d, want 0", len(store.replacedHeads))
	}
	if len(store.updatedDefault) != 0 {
		t.Fatalf("updatedDefault = %d, want 0", len(store.updatedDefault))
	}
}

func TestValidateContinuationTurnAcceptsPendingOwnedTurn(t *testing.T) {
	t.Parallel()

	sessionID := testUUID(1)
	botID := testUUID(2)
	persistTurnID := testUUID(3)
	baseHeadTurnID := testUUID(4)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{ID: sessionID, BotID: botID, Type: "chat"},
		historyTurn: dbsqlc.BotHistoryTurn{
			ID:             persistTurnID,
			BotID:          botID,
			OwnerSessionID: sessionID,
		},
		turnPath: []dbsqlc.BotHistoryTurn{
			{ID: baseHeadTurnID, BotID: botID, OwnerSessionID: sessionID, ParentTurnID: persistTurnID},
			{ID: persistTurnID, BotID: botID, OwnerSessionID: sessionID},
		},
	}
	resolver := &Resolver{queries: store}

	if err := resolver.validateBaseContinuationTurnHead(context.Background(), sessionID.String(), persistTurnID.String(), baseHeadTurnID.String(), "tool approval"); err != nil {
		t.Fatalf("validateBaseContinuationTurnHead() error = %v", err)
	}
	if len(store.createdHeads) != 0 || len(store.replacedHeads) != 0 {
		t.Fatalf("validation mutated heads: created=%#v replaced=%#v", store.createdHeads, store.replacedHeads)
	}
}

func TestValidateContinuationTurnRejectsPendingTurnOutsideSelectedHeadPath(t *testing.T) {
	t.Parallel()

	sessionID := testUUID(1)
	botID := testUUID(2)
	persistTurnID := testUUID(3)
	baseHeadTurnID := testUUID(4)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{ID: sessionID, BotID: botID, Type: "chat"},
		historyTurn: dbsqlc.BotHistoryTurn{
			ID:             persistTurnID,
			BotID:          botID,
			OwnerSessionID: sessionID,
		},
		turnPath: []dbsqlc.BotHistoryTurn{
			{ID: baseHeadTurnID, BotID: botID, OwnerSessionID: sessionID},
		},
	}
	resolver := &Resolver{queries: store}

	err := resolver.validateBaseContinuationTurnHead(context.Background(), sessionID.String(), persistTurnID.String(), baseHeadTurnID.String(), "tool approval")
	if err == nil {
		t.Fatal("validateBaseContinuationTurnHead() error = nil, want selected path rejection")
	}
	if !strings.Contains(err.Error(), "tool approval turn is no longer active for the requested conversation version") {
		t.Fatalf("validateBaseContinuationTurnHead() error = %v, want selected path rejection", err)
	}
}

func TestValidateContinuationTurnRejectsForeignPendingTurn(t *testing.T) {
	t.Parallel()

	sessionID := testUUID(1)
	botID := testUUID(2)
	foreignSessionID := testUUID(4)
	persistTurnID := testUUID(3)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{ID: sessionID, BotID: botID, Type: "chat"},
		historyTurn: dbsqlc.BotHistoryTurn{
			ID:             persistTurnID,
			BotID:          botID,
			OwnerSessionID: foreignSessionID,
		},
	}
	resolver := &Resolver{queries: store}

	err := resolver.validateBaseContinuationTurnHead(context.Background(), sessionID.String(), persistTurnID.String(), persistTurnID.String(), "user input")
	if err == nil {
		t.Fatalf("validateBaseContinuationTurnHead() error = nil, want foreign turn rejection")
	}
	if !strings.Contains(err.Error(), "pending turn is no longer active for this session") {
		t.Fatalf("validateBaseContinuationTurnHead() error = %v, want ownership rejection", err)
	}
}

func TestTurnRunAllowsPipelineContextOnlyForWholeSessionScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  TurnRun
		want bool
	}{
		{name: "empty", run: TurnRun{Context: TurnContextScope{Kind: ContextScopeEmpty}}, want: false},
		{name: "turn head", run: TurnRun{Context: TurnContextScope{Kind: ContextScopeTurnHead, TurnID: "turn-1"}}, want: false},
		{name: "session head", run: TurnRun{Context: TurnContextScope{Kind: ContextScopeSessionHead}}, want: true},
		{name: "bot history", run: TurnRun{Context: TurnContextScope{Kind: ContextScopeBotHistory}}, want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := turnRunAllowsPipelineContext(tt.run); got != tt.want {
				t.Fatalf("turnRunAllowsPipelineContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTurnRunViewHeadTurnIDFollowsContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  TurnRun
		want string
	}{
		{
			name: "turn head context uses context turn",
			run: TurnRun{
				Context: TurnContextScope{Kind: ContextScopeTurnHead, TurnID: "parent-turn"},
				Variant: VariantTransition{
					BaseHeadTurnID: "leaf-turn",
				},
			},
			want: "parent-turn",
		},
		{
			name: "empty context clears view",
			run: TurnRun{
				Context: TurnContextScope{Kind: ContextScopeEmpty},
				Variant: VariantTransition{
					BaseHeadTurnID: "leaf-turn",
				},
			},
			want: "",
		},
		{
			name: "session context falls back to base",
			run: TurnRun{
				Context: TurnContextScope{Kind: ContextScopeSessionHead},
				Variant: VariantTransition{
					BaseHeadTurnID: "leaf-turn",
				},
			},
			want: "leaf-turn",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.run.ViewHeadTurnID(); got != tt.want {
				t.Fatalf("ViewHeadTurnID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadMessagesForTurnRunUsesExplicitEmptyContext(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	loaded, err := resolver.loadMessagesForTurnRun(context.Background(), conversation.ChatRequest{
		BotID:     "bot-1",
		ChatID:    "bot-1",
		SessionID: "session-1",
	}, TurnRun{Context: TurnContextScope{Kind: ContextScopeEmpty}}, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadMessagesForTurnRun() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("loaded %d messages, want 0", len(loaded))
	}
	if len(messages.activeSession) != 0 || len(messages.activeTurn) != 0 || len(messages.activeBot) != 0 {
		t.Fatalf("empty context should not load history, got session=%v turn=%v bot=%v", messages.activeSession, messages.activeTurn, messages.activeBot)
	}
}

func TestLoadMessagesForTurnRunUsesTurnHeadContext(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, err := resolver.loadMessagesForTurnRun(context.Background(), conversation.ChatRequest{
		BotID:     "bot-1",
		ChatID:    "bot-1",
		SessionID: "session-1",
	}, TurnRun{Context: TurnContextScope{Kind: ContextScopeTurnHead, TurnID: "turn-1"}}, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadMessagesForTurnRun() error = %v", err)
	}
	if len(messages.activeTurn) != 1 || messages.activeTurn[0] != "turn-1" {
		t.Fatalf("turn context calls = %v, want [turn-1]", messages.activeTurn)
	}
	if len(messages.activeSession) != 0 || len(messages.activeBot) != 0 {
		t.Fatalf("turn context should not load fallback history, got session=%v bot=%v", messages.activeSession, messages.activeBot)
	}
}

func TestLoadMessagesForTurnRunRejectsEmptyTurnHead(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, err := resolver.loadMessagesForTurnRun(context.Background(), conversation.ChatRequest{
		BotID:     "bot-1",
		ChatID:    "bot-1",
		SessionID: "session-1",
	}, TurnRun{Context: TurnContextScope{Kind: ContextScopeTurnHead}}, defaultMaxContextMinutes)
	if err == nil {
		t.Fatalf("loadMessagesForTurnRun() error = nil, want error")
	}
	if len(messages.activeSession) != 0 || len(messages.activeTurn) != 0 || len(messages.activeBot) != 0 {
		t.Fatalf("empty turn context should not load fallback history, got session=%v turn=%v bot=%v", messages.activeSession, messages.activeTurn, messages.activeBot)
	}
}

func TestLoadMessagesForTurnRunUsesSessionHeadContext(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, err := resolver.loadMessagesForTurnRun(context.Background(), conversation.ChatRequest{
		BotID:     "bot-1",
		ChatID:    "bot-1",
		SessionID: "session-1",
	}, TurnRun{Context: TurnContextScope{Kind: ContextScopeSessionHead}}, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadMessagesForTurnRun() error = %v", err)
	}
	if len(messages.activeSession) != 1 || messages.activeSession[0] != "session-1" {
		t.Fatalf("session context calls = %v, want [session-1]", messages.activeSession)
	}
}

func TestPersistPartialResultDoesNotStoreUserOnlyFailure(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	run := legacyTurnRun(conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1"})

	resolver.persistPartialResult(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		&run,
		resolvedContext{},
		nil,
		0,
		false,
		true,
	)

	if len(messages.persisted) != 0 {
		t.Fatalf("expected failed stream not to persist user-only history, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotSkipsUserOnlySnapshot(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	run := legacyTurnRun(conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1"})

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		&run,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.UserMessage("hello")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected user-only terminal snapshot not to persist, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotSkipsEmptyAssistantSnapshot(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	run := legacyTurnRun(conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1"})

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		&run,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected empty assistant terminal snapshot not to persist, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotSkipsAbortedSnapshotBeforeVisibleOutput(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	run := legacyTurnRun(conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1"})

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		&run,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("openai: stream failed: lookup api.deepseek.com: no such host")},
			aborted:     true,
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected aborted startup snapshot not to persist, got %#v", messages.persisted)
	}
}

func TestHasVisibleAgentStreamOutputIgnoresLifecycleOnlyEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event agentpkg.StreamEvent
		want  bool
	}{
		{
			name:  "text start is lifecycle only",
			event: agentpkg.StreamEvent{Type: agentpkg.EventTextStart},
			want:  false,
		},
		{
			name:  "text end is lifecycle only",
			event: agentpkg.StreamEvent{Type: agentpkg.EventTextEnd},
			want:  false,
		},
		{
			name:  "blank text delta is lifecycle only",
			event: agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "  "},
			want:  false,
		},
		{
			name:  "text delta is visible",
			event: agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "hello"},
			want:  true,
		},
		{
			name:  "tool call is visible",
			event: agentpkg.StreamEvent{Type: agentpkg.EventToolCallStart, ToolCallID: "call-1"},
			want:  true,
		},
		{
			name:  "empty attachment event is lifecycle only",
			event: agentpkg.StreamEvent{Type: agentpkg.EventAttachment},
			want:  false,
		},
		{
			name:  "attachment is visible",
			event: agentpkg.StreamEvent{Type: agentpkg.EventAttachment, Attachments: []agentpkg.FileAttachment{{Name: "a.txt"}}},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasVisibleAgentStreamOutput(tt.event); got != tt.want {
				t.Fatalf("hasVisibleAgentStreamOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStoreRoundUsesPinnedTurnForEveryMessage(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:     "bot-1",
		SessionID: "session-1",
		Query:     "hello",
	}
	run := TurnRun{
		Mode:          TurnRunModeNormal,
		PersistTurnID: "turn-1",
	}
	err := resolver.storeRoundWithOptions(context.Background(), req, &run, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("thinking")},
		{Role: "tool", ToolCallID: "tool-1", Content: conversation.NewTextContent("ok")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{AllowPendingToolCalls: true})
	if err != nil {
		t.Fatalf("storeRoundWithOptions() error = %v", err)
	}
	if len(messages.persisted) != 4 {
		t.Fatalf("persisted %d messages, want 4", len(messages.persisted))
	}
	for i, persisted := range messages.persisted {
		if persisted.TurnID != "turn-1" {
			t.Fatalf("persisted[%d].TurnID = %q, want turn-1", i, persisted.TurnID)
		}
	}
}

func TestStoreRoundMaterializesPlannedTurnForEveryMessage(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	store := &fakeTurnStore{}
	resolver := &Resolver{
		queries:        store,
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:     testUUID(1).String(),
		SessionID: testUUID(2).String(),
		Query:     "hello",
	}
	run, err := resolver.prepareTurnRun(context.Background(), req)
	if err != nil {
		t.Fatalf("prepareTurnRun() error = %v", err)
	}
	if len(store.createdTurns) != 0 {
		t.Fatalf("prepareTurnRun created %d turns, want 0", len(store.createdTurns))
	}

	err = resolver.storeRoundWithOptions(context.Background(), req, &run, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeRoundWithOptions() error = %v", err)
	}
	if len(store.createdTurns) != 1 {
		t.Fatalf("createdTurns = %d, want 1", len(store.createdTurns))
	}
	if run.PersistTurnID == "" {
		t.Fatalf("run.PersistTurnID is empty after store")
	}
	if len(messages.persisted) != 2 {
		t.Fatalf("persisted %d messages, want 2", len(messages.persisted))
	}
	for i, persisted := range messages.persisted {
		if persisted.TurnID != run.PersistTurnID {
			t.Fatalf("persisted[%d].TurnID = %q, want %q", i, persisted.TurnID, run.PersistTurnID)
		}
	}
}

func TestStoreRoundAndVariantTransitionShareTransaction(t *testing.T) {
	t.Parallel()

	messages := &recordingTxMessageService{}
	oldHead := testUUID(7)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: oldHead},
	}
	resolver := &Resolver{
		queries:        store,
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:          testUUID(1).String(),
		SessionID:      testUUID(2).String(),
		BaseHeadTurnID: oldHead.String(),
		Query:          "hello",
	}
	run, err := resolver.prepareTurnRun(context.Background(), req)
	if err != nil {
		t.Fatalf("prepareTurnRun() error = %v", err)
	}

	stored, err := resolver.storeRoundAndApplyVariantTransition(context.Background(), req, &run, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeRoundAndApplyVariantTransition() error = %v", err)
	}
	if store.txCount != 1 {
		t.Fatalf("txCount = %d, want 1", store.txCount)
	}
	if len(store.createdTurns) != 1 {
		t.Fatalf("createdTurns = %d, want 1", len(store.createdTurns))
	}
	if stored.TurnID == "" || run.PersistTurnID != stored.TurnID {
		t.Fatalf("stored turn=%q run turn=%q, want same non-empty", stored.TurnID, run.PersistTurnID)
	}
	if len(messages.txPersisted) != 2 {
		t.Fatalf("txPersisted = %d, want 2", len(messages.txPersisted))
	}
	for i, persisted := range messages.txPersisted {
		if persisted.TurnID != stored.TurnID {
			t.Fatalf("txPersisted[%d].TurnID = %q, want %q", i, persisted.TurnID, stored.TurnID)
		}
	}
	if len(store.replacedHeads) != 1 || store.replacedHeads[0].OldHeadTurnID != oldHead {
		t.Fatalf("replacedHeads = %#v, want old head replacement", store.replacedHeads)
	}
	if len(store.updatedDefault) != 1 || store.updatedDefault[0].DefaultHeadTurnID.String() != stored.TurnID {
		t.Fatalf("updatedDefault = %#v, want stored turn", store.updatedDefault)
	}
	if len(messages.published) != 2 {
		t.Fatalf("published = %d, want 2", len(messages.published))
	}
}

func TestStoreRoundAndVariantTransitionRewriteCreatesSiblingTurn(t *testing.T) {
	t.Parallel()

	messages := &recordingTxMessageService{}
	baseHead := testUUID(7)
	targetParent := testUUID(4)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: baseHead},
	}
	resolver := &Resolver{
		queries:        store,
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:          testUUID(1).String(),
		SessionID:      testUUID(2).String(),
		BaseHeadTurnID: baseHead.String(),
		Query:          "same request",
	}
	run, err := resolver.prepareTurnRunWithRewriteAnchor(context.Background(), req, &conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleUser,
		MessageID:      testUUID(3).String(),
		TurnID:         baseHead.String(),
		ParentTurnID:   targetParent.String(),
		BaseHeadTurnID: baseHead.String(),
	})
	if err != nil {
		t.Fatalf("prepareTurnRunWithRewriteAnchor() error = %v", err)
	}
	if run.Mode != TurnRunModeRewrite {
		t.Fatalf("mode = %q, want rewrite", run.Mode)
	}
	if run.Turn.ParentTurnID != targetParent.String() {
		t.Fatalf("parent turn = %q, want target parent %q", run.Turn.ParentTurnID, targetParent.String())
	}
	if run.Variant.Action != VariantTransitionCreateSibling {
		t.Fatalf("transition action = %q, want create sibling", run.Variant.Action)
	}

	stored, err := resolver.storeRoundAndApplyVariantTransition(context.Background(), req, &run, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("same request")},
		{Role: "assistant", Content: conversation.NewTextContent("second reply")},
	}, "", storeRoundOptions{})
	if err != nil {
		t.Fatalf("storeRoundAndApplyVariantTransition() error = %v", err)
	}
	if len(store.createdTurns) != 1 {
		t.Fatalf("createdTurns = %d, want 1", len(store.createdTurns))
	}
	if store.createdTurns[0].ParentTurnID != targetParent {
		t.Fatalf("created turn parent = %v, want %v", store.createdTurns[0].ParentTurnID, targetParent)
	}
	if len(store.createdHeads) != 1 || store.createdHeads[0].HeadTurnID.String() != stored.TurnID {
		t.Fatalf("createdHeads = %#v, want stored turn head %q", store.createdHeads, stored.TurnID)
	}
	if len(store.replacedHeads) != 0 {
		t.Fatalf("replacedHeads = %#v, want none for rewrite sibling", store.replacedHeads)
	}
	if len(store.updatedDefault) != 1 || store.updatedDefault[0].DefaultHeadTurnID.String() != stored.TurnID {
		t.Fatalf("updatedDefault = %#v, want stored turn", store.updatedDefault)
	}
}

func TestStoreRoundAndVariantTransitionFailureDoesNotPublishOrKeepRolledBackTurn(t *testing.T) {
	t.Parallel()

	pointerErr := errors.New("head update failed")
	messages := &recordingTxMessageService{}
	oldHead := testUUID(7)
	store := &fakeTurnStore{
		session:          dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: oldHead},
		updateDefaultErr: pointerErr,
	}
	resolver := &Resolver{
		queries:        store,
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:          testUUID(1).String(),
		SessionID:      testUUID(2).String(),
		BaseHeadTurnID: oldHead.String(),
		Query:          "hello",
	}
	run, err := resolver.prepareTurnRun(context.Background(), req)
	if err != nil {
		t.Fatalf("prepareTurnRun() error = %v", err)
	}

	_, err = resolver.storeRoundAndApplyVariantTransition(context.Background(), req, &run, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err == nil {
		t.Fatalf("storeRoundAndApplyVariantTransition() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "update default head") {
		t.Fatalf("error = %v, want update default head context", err)
	}
	if run.PersistTurnID != "" {
		t.Fatalf("run.PersistTurnID = %q, want cleared after rollback", run.PersistTurnID)
	}
	if len(messages.published) != 0 {
		t.Fatalf("published = %d, want 0", len(messages.published))
	}
}

func TestStoreRoundAndVariantTransitionRequiresTransaction(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	oldHead := testUUID(7)
	store := &fakeTurnStore{
		session: dbsqlc.BotSession{Type: "chat", DefaultHeadTurnID: oldHead},
	}
	resolver := &Resolver{
		queries:        store,
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:          testUUID(1).String(),
		SessionID:      testUUID(2).String(),
		BaseHeadTurnID: oldHead.String(),
		Query:          "hello",
	}
	run, err := resolver.prepareTurnRun(context.Background(), req)
	if err != nil {
		t.Fatalf("prepareTurnRun() error = %v", err)
	}

	_, err = resolver.storeRoundAndApplyVariantTransition(context.Background(), req, &run, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("hello")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})
	if err == nil {
		t.Fatalf("storeRoundAndApplyVariantTransition() error = nil, want transaction support failure")
	}
	if !strings.Contains(err.Error(), "transaction-bound turn persistence is required") {
		t.Fatalf("error = %v, want transaction support context", err)
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("persisted = %d, want 0", len(messages.persisted))
	}
	if len(store.createdTurns) != 0 {
		t.Fatalf("createdTurns = %d, want 0", len(store.createdTurns))
	}
}

func TestEnsurePersistTurnIsIdempotentForRun(t *testing.T) {
	t.Parallel()

	store := &fakeTurnStore{}
	resolver := &Resolver{queries: store}
	run := TurnRun{
		Mode: TurnRunModeNormal,
		Turn: TurnPersistencePlan{
			BotID:          testUUID(1).String(),
			OwnerSessionID: testUUID(2).String(),
		},
	}

	first, err := resolver.ensurePersistTurn(context.Background(), &run)
	if err != nil {
		t.Fatalf("ensurePersistTurn() first error = %v", err)
	}
	second, err := resolver.ensurePersistTurn(context.Background(), &run)
	if err != nil {
		t.Fatalf("ensurePersistTurn() second error = %v", err)
	}
	if first == "" || second == "" || first != second {
		t.Fatalf("turn ids first=%q second=%q, want same non-empty id", first, second)
	}
	if len(store.createdTurns) != 1 {
		t.Fatalf("createdTurns = %d, want 1", len(store.createdTurns))
	}
}

func TestPersistTerminalSnapshotStoresAssistantOutput(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	run := legacyTurnRun(conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1"})

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		&run,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("partial answer")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 2 {
		t.Fatalf("expected user and assistant messages to persist, got %#v", messages.persisted)
	}
	if messages.persisted[0].Role != "user" || messages.persisted[1].Role != "assistant" {
		t.Fatalf("unexpected persisted roles: %q, %q", messages.persisted[0].Role, messages.persisted[1].Role)
	}
}

func TestPersistTerminalSnapshotSkipsUserWhenPipelineContextContainsCurrentMessage(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	run := legacyTurnRun(conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1"})

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "---\nmessage-id: tg-1\nchannel: telegram\n---\n@memoh1bot ping",
		},
		&run,
		resolvedContext{
			userMessageAlreadyInContext: true,
		},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("pong")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 1 {
		t.Fatalf("expected only assistant output to persist, got %#v", messages.persisted)
	}
	if messages.persisted[0].Role != "assistant" {
		t.Fatalf("unexpected persisted role: %q", messages.persisted[0].Role)
	}
}
