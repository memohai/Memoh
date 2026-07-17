package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

func TestHandleReplyDoesNotConsumeContextWhenRoundPersistenceFails(t *testing.T) {
	t.Parallel()

	messages, err := json.Marshal([]sdk.Message{sdk.AssistantMessage("reply")})
	if err != nil {
		t.Fatalf("marshal terminal messages: %v", err)
	}
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{ModelID: "model-1"},
		storeRoundErr: errors.New("persist failed"),
	}
	cursorStore := &fakeDiscussCursorStore{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, CursorStore: cursorStore})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:                  "bot",
		SessionID:              "session",
		PersistedUserMessageID: "user-message",
	}}
	rc := RenderedContext{{
		EventCursor: 100,
		MessageID:   "new",
		Content:     []RenderedContentPiece{{Type: "text", Text: "retry me"}},
	}}
	agent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{
		Type:     agentpkg.EventAgentEnd,
		Messages: messages,
	}}}

	result := driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if sess.lastProcessedCursor != 0 {
		t.Fatalf("persistence failure consumed cursor %d", sess.lastProcessedCursor)
	}
	if resolver.storeRoundCursor.Position.EventCursor != 100 {
		t.Fatalf("persistence failure attempted cursor %d, want 100", resolver.storeRoundCursor.Position.EventCursor)
	}
	if cursorStore.upsertCursor.EventCursor != 0 {
		t.Fatalf("persistence failure separately stored cursor %d", cursorStore.upsertCursor.EventCursor)
	}
	if result != discussReplyRetry {
		t.Fatalf("persistence failure result = %v, want retry", result)
	}
}

func TestHandleReplyStoresResponseAndCursorAtomically(t *testing.T) {
	t.Parallel()

	messages, err := json.Marshal([]sdk.Message{sdk.AssistantMessage("reply")})
	if err != nil {
		t.Fatalf("marshal terminal messages: %v", err)
	}
	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{ModelID: "model-1"}}
	cursorStore := &fakeDiscussCursorStore{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, CursorStore: cursorStore})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:                  "bot",
		SessionID:              "session",
		RouteID:                "route",
		CurrentPlatform:        "telegram",
		PersistedUserMessageID: "user-message",
	}}
	rc := RenderedContext{{
		ReceivedAtMs: 90,
		EventCursor:  100,
		MessageID:    "new",
		Content:      []RenderedContentPiece{{Type: "text", Text: "persist me"}},
	}}
	agent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{
		Type:     agentpkg.EventAgentEnd,
		Messages: messages,
	}}}

	result := driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if result != discussReplyComplete || sess.lastProcessedCursor != 100 {
		t.Fatalf("reply result/cursor = %v/%d, want complete/100", result, sess.lastProcessedCursor)
	}
	if got := resolver.storeRoundCursor; got.Position.EventCursor != 100 || got.Position.SourceCursor != 90 || got.ScopeKey != "route:route" {
		t.Fatalf("atomic round cursor = %#v", got)
	}
	if cursorStore.upsertCursor.EventCursor != 0 {
		t.Fatalf("response path separately upserted cursor %d", cursorStore.upsertCursor.EventCursor)
	}
}

func TestDiscussRestartTrustsCursorCommittedWithResponse(t *testing.T) {
	t.Parallel()

	messages, err := json.Marshal([]sdk.Message{sdk.AssistantMessage("reply")})
	if err != nil {
		t.Fatalf("marshal terminal messages: %v", err)
	}
	state := &restartDiscussCursorState{}
	firstStore := &restartDiscussCursorStore{state: state, upsertErr: errors.New("legacy cursor write failed")}
	firstResolver := &restartRoundResolver{
		fakeRunConfigResolver: fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{ModelID: "model-1"}},
		state:                 state,
	}
	firstDriver := NewDiscussDriver(DiscussDriverDeps{Resolver: firstResolver, CursorStore: firstStore})
	config := DiscussSessionConfig{
		BotID:                  "bot",
		SessionID:              "session",
		RouteID:                "route",
		CurrentPlatform:        "telegram",
		PersistedUserMessageID: "user-message",
	}
	rc := RenderedContext{{
		ReceivedAtMs: 90,
		EventCursor:  100,
		MessageID:    "new",
		Content:      []RenderedContentPiece{{Type: "text", Text: "persist me"}},
	}}
	firstAgent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{Type: agentpkg.EventAgentEnd, Messages: messages}}}
	if result := firstDriver.handleReplyWithAgent(
		context.Background(),
		&discussSession{config: config},
		rc,
		firstDriver.logger,
		firstAgent,
	); result != discussReplyComplete {
		t.Fatalf("first reply result = %v, want complete", result)
	}
	if firstStore.upsertCalls != 0 || firstResolver.storeCalls != 1 {
		t.Fatalf("first cursor/store calls = %d/%d, want 0/1", firstStore.upsertCalls, firstResolver.storeCalls)
	}

	secondStore := &restartDiscussCursorStore{state: state}
	secondResolver := &restartRoundResolver{state: state}
	secondDriver := NewDiscussDriver(DiscussDriverDeps{Resolver: secondResolver, CursorStore: secondStore})
	secondAgent := &fakeDiscussStreamer{}
	if result := secondDriver.handleReplyWithAgent(
		context.Background(),
		&discussSession{config: config},
		rc,
		secondDriver.logger,
		secondAgent,
	); result != discussReplyComplete {
		t.Fatalf("restart reply result = %v, want complete", result)
	}
	if secondAgent.lastConfig != nil || secondResolver.storeCalls != 0 {
		t.Fatalf("restart repeated agent/store = %t/%d, want false/0", secondAgent.lastConfig != nil, secondResolver.storeCalls)
	}
}

func TestHandleReplyAdvancesCursorForEmptyTerminalResponse(t *testing.T) {
	t.Parallel()

	messages, err := json.Marshal([]sdk.Message{sdk.AssistantMessage("")})
	if err != nil {
		t.Fatalf("marshal terminal messages: %v", err)
	}
	resolver := &fakeRunConfigResolver{storeRoundNoResponse: true}
	cursorStore := &fakeDiscussCursorStore{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, CursorStore: cursorStore})
	sess := &discussSession{config: DiscussSessionConfig{
		BotID:                  "bot",
		SessionID:              "session",
		PersistedUserMessageID: "user-message",
	}}
	rc := RenderedContext{{EventCursor: 100, MessageID: "new", Content: []RenderedContentPiece{{Type: "text", Text: "hello"}}}}
	agent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{Type: agentpkg.EventAgentEnd, Messages: messages}}}

	result := driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if result != discussReplyComplete || sess.lastProcessedCursor != 100 {
		t.Fatalf("empty terminal result/cursor = %v/%d, want complete/100", result, sess.lastProcessedCursor)
	}
	if cursorStore.upsertCursor.EventCursor != 100 {
		t.Fatalf("empty terminal durable cursor = %d, want 100", cursorStore.upsertCursor.EventCursor)
	}
}

type restartDiscussCursorState struct {
	position DiscussCursorPosition
}

type restartDiscussCursorStore struct {
	state       *restartDiscussCursorState
	upsertErr   error
	upsertCalls int
}

func (s *restartDiscussCursorStore) GetDiscussCursor(context.Context, string, string) (DiscussCursorPosition, error) {
	return s.state.position, nil
}

func (s *restartDiscussCursorStore) UpsertDiscussCursor(context.Context, string, string, string, string, DiscussCursorPosition) error {
	s.upsertCalls++
	return s.upsertErr
}

type restartRoundResolver struct {
	fakeRunConfigResolver
	state      *restartDiscussCursorState
	storeCalls int
}

func (r *restartRoundResolver) StoreRoundWithCursor(
	_ context.Context,
	_, _, _, _, _ string,
	_ []sdk.Message,
	_ string,
	cursor DiscussCursorCommit,
) (bool, error) {
	r.storeCalls++
	r.state.position = cursor.Position
	return true, nil
}
