package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	agentpkg "github.com/memohai/memoh/internal/agent"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/userinput"
)

type modelStreamQueries struct {
	*modelSelectionFakeQueries
}

func (*modelStreamQueries) GetSettingsByBotID(context.Context, pgtype.UUID) (sqlc.GetSettingsByBotIDRow, error) {
	return sqlc.GetSettingsByBotIDRow{}, nil
}

func (*modelStreamQueries) GetBotByID(context.Context, pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return sqlc.GetBotByIDRow{}, pgx.ErrNoRows
}

type modelStreamMessageService struct {
	*recordingMessageService
	persisted       atomic.Bool
	persistStarted  chan struct{}
	persistRelease  <-chan struct{}
	persistOnce     sync.Once
	failFirst       error
	persistAttempts atomic.Int32
}

func (s *modelStreamMessageService) awaitPersist() error {
	if s.persistStarted != nil {
		s.persistOnce.Do(func() { close(s.persistStarted) })
	}
	if s.persistRelease != nil {
		<-s.persistRelease
	}
	if s.persistAttempts.Add(1) == 1 && s.failFirst != nil {
		return s.failFirst
	}
	return nil
}

func (s *modelStreamMessageService) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	if err := s.awaitPersist(); err != nil {
		return messagepkg.Message{}, err
	}
	message, err := s.recordingMessageService.Persist(ctx, input)
	if err == nil {
		s.persisted.Store(true)
	}
	return message, err
}

func (s *modelStreamMessageService) PersistRound(ctx context.Context, inputs []messagepkg.PersistInput, options messagepkg.RoundPersistenceOptions) ([]messagepkg.Message, bool, error) {
	if err := s.awaitPersist(); err != nil {
		return nil, true, err
	}
	messages, handled, err := s.recordingMessageService.PersistRound(ctx, inputs, options)
	if err == nil {
		s.persisted.Store(true)
	}
	return messages, handled, err
}

func (*modelStreamMessageService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, errors.New("history unavailable")
}

func TestStreamChatReportsTerminalPersistenceFailure(t *testing.T) {
	wantErr := errors.New("persist model response")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":null}]}\n\n"+
				"data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"+
				"data: [DONE]\n\n",
		)
	}))
	defer server.Close()

	provider := modelSelectionProviderRow(t, "00000000-0000-0000-0000-000000000701", string(models.ClientTypeOpenAICompletions), true)
	provider.Config = []byte(fmt.Sprintf(`{"api_key":"test-key","base_url":%q}`, server.URL))
	model := modelSelectionModelRow(
		t,
		"00000000-0000-0000-0000-000000000702",
		"stream-persistence-model",
		provider.ID,
		models.ModelTypeChat,
		true,
	)
	queries := &modelStreamQueries{modelSelectionFakeQueries: &modelSelectionFakeQueries{
		models:   map[string]sqlc.Model{model.ModelID: model},
		provider: provider,
	}}
	logger := slog.New(slog.DiscardHandler)
	resolver := NewResolver(
		logger,
		models.NewService(logger, queries),
		queries,
		nil,
		&modelStreamMessageService{recordingMessageService: &recordingMessageService{persistErr: wantErr}},
		settings.NewService(logger, queries, nil, nil),
		nil,
		agentpkg.New(agentpkg.Deps{}),
		time.UTC,
		time.Second,
	)

	chunks, errs := resolver.StreamChat(context.Background(), conversation.ChatRequest{
		BotID:               storeRoundBotID,
		ChatID:              "chat-1",
		SessionID:           "33333333-3333-3333-3333-333333333333",
		Query:               "hello",
		Model:               model.ModelID,
		SkipTitleGeneration: true,
	})
	events := drainStreamChunks(t, chunks)
	if !containsStreamEvent(events, agentpkg.EventAgentEnd) {
		t.Fatalf("events = %#v, want terminal event", events)
	}
	if err := <-errs; !errors.Is(err, wantErr) {
		t.Fatalf("StreamChat() error = %v, want %v", err, wantErr)
	}
}

func TestStreamChatPublishesAskUserOnlyAfterTerminalPersistence(t *testing.T) {
	ambiguousErr := errors.New("ambiguous commit result")
	for _, tt := range []struct {
		name         string
		persistErr   error
		failFirst    error
		workspaceErr error
		idleTimeout  time.Duration
		persistDelay time.Duration
		wantPrompt   bool
		wantPersist  bool
		wantAttempts int32
		wantErr      error
	}{
		{name: "persisted response", wantPrompt: true, wantPersist: true, wantAttempts: 1},
		{name: "terminal persistence outlives idle timeout", idleTimeout: 250 * time.Millisecond, persistDelay: 500 * time.Millisecond, wantPrompt: true, wantPersist: true, wantAttempts: 1},
		{name: "safe transient persistence failure", failFirst: dbstore.MarkPersistenceRetrySafe(errors.New("transient persist failure")), wantPrompt: true, wantPersist: true, wantAttempts: 2},
		{name: "ambiguous persistence failure", failFirst: ambiguousErr, wantAttempts: 1, wantErr: ambiguousErr},
		{name: "persistence failure", persistErr: errors.New("persist ask_user response"), wantAttempts: 1},
		{name: "post commit workspace failure", workspaceErr: errors.New("persist workspace target"), wantPrompt: true, wantPersist: true, wantAttempts: 1},
		{name: "transient then post commit failure", failFirst: dbstore.MarkPersistenceRetrySafe(errors.New("transient persist failure")), workspaceErr: errors.New("persist workspace target"), wantPrompt: true, wantPersist: true, wantAttempts: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newAskUserStreamServer()
			defer server.Close()

			provider := modelSelectionProviderRow(t, "00000000-0000-0000-0000-000000000711", string(models.ClientTypeOpenAICompletions), true)
			provider.Config = []byte(fmt.Sprintf(`{"api_key":"test-key","base_url":%q}`, server.URL))
			model := modelSelectionModelRow(
				t,
				"00000000-0000-0000-0000-000000000712",
				"ask-user-stream-persistence-model",
				provider.ID,
				models.ModelTypeChat,
				true,
			)
			model.Config = []byte(`{"compatibilities":["tool-call"]}`)
			queries := &modelStreamQueries{modelSelectionFakeQueries: &modelSelectionFakeQueries{
				models:   map[string]sqlc.Model{model.ModelID: model},
				provider: provider,
			}}
			messages := &modelStreamMessageService{
				recordingMessageService: &recordingMessageService{persistErr: tt.persistErr},
				failFirst:               tt.failFirst,
			}
			var persistenceStarted <-chan struct{}
			var terminalPersistStarted <-chan struct{}
			var releasePersistence func()
			if tt.persistErr == nil {
				started := make(chan struct{})
				release := make(chan struct{})
				var releaseOnce sync.Once
				releasePersistence = func() { releaseOnce.Do(func() { close(release) }) }
				defer releasePersistence()
				messages.persistStarted = started
				messages.persistRelease = release
				if tt.idleTimeout > 0 {
					terminalPersistStarted = started
				} else {
					persistenceStarted = started
				}
			}
			inputs := &fakeUserInputService{createFn: func(input userinput.CreatePendingInput) (userinput.Request, error) {
				payload, err := userinput.ParseAskUserPayload(input.Input)
				if err != nil {
					return userinput.Request{}, err
				}
				raw, _ := input.Input.(map[string]any)
				return userinput.Request{
					ID:         "00000000-0000-4000-8000-000000000713",
					BotID:      input.BotID,
					SessionID:  input.SessionID,
					ToolCallID: input.ToolCallID,
					ToolName:   input.ToolName,
					ShortID:    1,
					Status:     userinput.StatusPending,
					Input:      raw,
					UIPayload:  payload,
				}, nil
			}}
			logger := slog.New(slog.DiscardHandler)
			agent := agentpkg.New(agentpkg.Deps{})
			agent.SetToolProviders([]agenttools.ToolProvider{agenttools.NewAskUserProvider(logger)})
			resolver := NewResolver(
				logger,
				models.NewService(logger, queries),
				queries,
				nil,
				messages,
				settings.NewService(logger, queries, nil, nil),
				nil,
				agent,
				time.UTC,
				time.Second,
			)
			resolver.userInput = inputs
			if tt.idleTimeout > 0 {
				resolver.idleTimeoutOptions = &idleTimeoutOptions{baseTimeout: tt.idleTimeout}
			}
			if tt.workspaceErr != nil {
				resolver.sessionService = streamWorkspaceFailureSessionService(tt.workspaceErr)
			}

			req := conversation.ChatRequest{
				BotID:                storeRoundBotID,
				ChatID:               "chat-1",
				SessionID:            "33333333-3333-3333-3333-333333333333",
				Query:                "ask me",
				Model:                model.ModelID,
				SessionType:          "chat",
				UserMessagePersisted: tt.idleTimeout > 0,
				SkipTitleGeneration:  true,
			}
			if tt.workspaceErr != nil {
				req.WorkspaceTarget = &conversation.WorkspaceTarget{TargetID: "workspace-1", Kind: "local"}
			}
			chunks, errs := resolver.StreamChat(context.Background(), req)
			if terminalPersistStarted != nil {
				go func() {
					<-terminalPersistStarted
					time.Sleep(tt.persistDelay)
					releasePersistence()
				}()
			}
			var events []agentpkg.StreamEvent
			if persistenceStarted != nil {
				deadline := time.NewTimer(time.Second)
				defer deadline.Stop()
			waitForPersistence:
				for {
					select {
					case <-persistenceStarted:
						break waitForPersistence
					case chunk, ok := <-chunks:
						if !ok {
							t.Fatal("stream ended before ask_user terminal persistence")
						}
						var event agentpkg.StreamEvent
						if err := json.Unmarshal(chunk, &event); err != nil {
							t.Fatalf("decode stream chunk: %v", err)
						}
						if event.Type == agentpkg.EventUserInputRequest || event.ToolName == userinput.ToolNameAskUser {
							t.Fatalf("ask_user event %q was published before terminal persistence started", event.Type)
						}
						events = append(events, event)
					case <-deadline.C:
						t.Fatal("timed out waiting for ask_user terminal persistence")
					}
				}
				blocked := time.NewTimer(25 * time.Millisecond)
				defer blocked.Stop()
				select {
				case chunk, ok := <-chunks:
					if !ok {
						t.Fatal("stream ended while terminal persistence was blocked")
					}
					var event agentpkg.StreamEvent
					if err := json.Unmarshal(chunk, &event); err != nil {
						t.Fatalf("decode blocked stream chunk: %v", err)
					}
					t.Fatalf("event %q was published while terminal persistence was blocked", event.Type)
				case <-blocked.C:
				}
				releasePersistence()
			}
			for chunk := range chunks {
				var event agentpkg.StreamEvent
				if err := json.Unmarshal(chunk, &event); err != nil {
					t.Fatalf("decode stream chunk: %v", err)
				}
				if (event.Type == agentpkg.EventUserInputRequest || event.ToolName == userinput.ToolNameAskUser) && !messages.persisted.Load() {
					t.Fatalf("ask_user event %q was published before its terminal response persisted", event.Type)
				}
				events = append(events, event)
			}
			promptSeen := false
			for _, event := range events {
				if event.Type != agentpkg.EventUserInputRequest {
					continue
				}
				promptSeen = true
			}
			if promptSeen != tt.wantPrompt || messages.persisted.Load() != tt.wantPersist {
				t.Fatalf("prompt/persisted/create = %t/%t/%d, want %t/%t; events=%#v", promptSeen, messages.persisted.Load(), inputs.createCalls, tt.wantPrompt, tt.wantPersist, events)
			}
			if attempts := messages.persistAttempts.Load(); attempts != tt.wantAttempts {
				t.Fatalf("persistence attempts = %d, want %d", attempts, tt.wantAttempts)
			}
			for _, input := range messages.recordingMessageService.persisted {
				if input.Role == "tool" {
					t.Fatalf("durable ask_user was closed by fallback tool result: %#v", messages.recordingMessageService.persisted)
				}
			}
			err := <-errs
			wantErr := tt.wantErr
			if wantErr == nil {
				wantErr = tt.persistErr
			}
			if wantErr == nil && err != nil {
				t.Fatalf("StreamChat() error = %v", err)
			}
			if wantErr != nil && !errors.Is(err, wantErr) {
				t.Fatalf("StreamChat() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestStreamChatWSPublishesAskUserOnlyAfterTerminalPersistence(t *testing.T) {
	ambiguousErr := errors.New("ambiguous commit result")
	for _, tt := range []struct {
		name              string
		persistErr        error
		failFirst         error
		workspaceErr      error
		idleTimeout       time.Duration
		persistDelay      time.Duration
		wantPrompt        bool
		wantPersist       bool
		wantAttempts      int32
		wantErr           error
		abortAfterPersist bool
	}{
		{name: "persisted response", wantPrompt: true, wantPersist: true, wantAttempts: 1},
		{name: "terminal persistence outlives idle timeout", idleTimeout: 250 * time.Millisecond, persistDelay: 500 * time.Millisecond, wantPrompt: true, wantPersist: true, wantAttempts: 1},
		{name: "safe transient persistence failure", failFirst: dbstore.MarkPersistenceRetrySafe(errors.New("transient persist failure")), wantPrompt: true, wantPersist: true, wantAttempts: 2},
		{name: "ambiguous persistence failure", failFirst: ambiguousErr, wantAttempts: 1, wantErr: ambiguousErr},
		{name: "persistence failure", persistErr: errors.New("persist ask_user response"), wantAttempts: 1},
		{name: "post commit workspace failure", workspaceErr: errors.New("persist workspace target"), wantPrompt: true, wantPersist: true, wantAttempts: 1},
		{name: "transient then post commit failure", failFirst: dbstore.MarkPersistenceRetrySafe(errors.New("transient persist failure")), workspaceErr: errors.New("persist workspace target"), wantPrompt: true, wantPersist: true, wantAttempts: 2},
		{name: "abort after persistence", wantPersist: true, wantAttempts: 1, abortAfterPersist: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newAskUserStreamServer()
			defer server.Close()

			provider := modelSelectionProviderRow(t, "00000000-0000-0000-0000-000000000721", string(models.ClientTypeOpenAICompletions), true)
			provider.Config = []byte(fmt.Sprintf(`{"api_key":"test-key","base_url":%q}`, server.URL))
			model := modelSelectionModelRow(t, "00000000-0000-0000-0000-000000000722", "ask-user-ws-persistence-model", provider.ID, models.ModelTypeChat, true)
			model.Config = []byte(`{"compatibilities":["tool-call"]}`)
			queries := &modelStreamQueries{modelSelectionFakeQueries: &modelSelectionFakeQueries{
				models: map[string]sqlc.Model{model.ModelID: model}, provider: provider,
			}}
			messages := &modelStreamMessageService{
				recordingMessageService: &recordingMessageService{persistErr: tt.persistErr},
				failFirst:               tt.failFirst,
			}
			var terminalPersistStarted <-chan struct{}
			var releaseTerminalPersistence func()
			if tt.idleTimeout > 0 {
				started := make(chan struct{})
				release := make(chan struct{})
				var releaseOnce sync.Once
				terminalPersistStarted = started
				releaseTerminalPersistence = func() { releaseOnce.Do(func() { close(release) }) }
				messages.persistStarted = started
				messages.persistRelease = release
				defer releaseTerminalPersistence()
			}
			inputs := &fakeUserInputService{createFn: func(input userinput.CreatePendingInput) (userinput.Request, error) {
				payload, err := userinput.ParseAskUserPayload(input.Input)
				if err != nil {
					return userinput.Request{}, err
				}
				raw, _ := input.Input.(map[string]any)
				return userinput.Request{
					ID: "00000000-0000-4000-8000-000000000723", BotID: input.BotID, SessionID: input.SessionID,
					ToolCallID: input.ToolCallID, ToolName: input.ToolName, ShortID: 1, Status: userinput.StatusPending,
					Input: raw, UIPayload: payload,
				}, nil
			}}
			logger := slog.New(slog.DiscardHandler)
			agent := agentpkg.New(agentpkg.Deps{})
			agent.SetToolProviders([]agenttools.ToolProvider{agenttools.NewAskUserProvider(logger)})
			resolver := NewResolver(logger, models.NewService(logger, queries), queries, nil, messages,
				settings.NewService(logger, queries, nil, nil), nil, agent, time.UTC, time.Second)
			resolver.userInput = inputs
			if tt.idleTimeout > 0 {
				resolver.idleTimeoutOptions = &idleTimeoutOptions{baseTimeout: tt.idleTimeout}
			}
			if tt.workspaceErr != nil {
				resolver.sessionService = streamWorkspaceFailureSessionService(tt.workspaceErr)
			}

			eventCh := make(chan WSStreamEvent)
			done := make(chan error, 1)
			var abortCh chan struct{}
			streamCtx := context.Background()
			cancelStream := func() {}
			if tt.abortAfterPersist {
				abortCh = make(chan struct{})
				streamCtx, cancelStream = context.WithCancel(streamCtx)
			}
			defer cancelStream()
			go func() {
				req := conversation.ChatRequest{
					BotID: storeRoundBotID, ChatID: "chat-1", SessionID: "33333333-3333-3333-3333-333333333333",
					Query: "ask me", Model: model.ModelID, SessionType: "chat", UserMessagePersisted: tt.idleTimeout > 0, SkipTitleGeneration: true,
				}
				if tt.workspaceErr != nil {
					req.WorkspaceTarget = &conversation.WorkspaceTarget{TargetID: "workspace-1", Kind: "local"}
				}
				if tt.abortAfterPersist {
					_, err := resolver.streamChatWSResultWithHooks(streamCtx, req, eventCh, abortCh, nil, func(context.Context, []messagepkg.Message) error {
						close(abortCh)
						cancelStream()
						return nil
					})
					done <- err
				} else {
					done <- resolver.StreamChatWS(streamCtx, req, eventCh, nil)
				}
				close(eventCh)
			}()
			if terminalPersistStarted != nil {
				go func() {
					<-terminalPersistStarted
					time.Sleep(tt.persistDelay)
					releaseTerminalPersistence()
				}()
			}
			promptSeen := false
			for data := range eventCh {
				var event agentpkg.StreamEvent
				if err := json.Unmarshal(data, &event); err != nil {
					t.Fatalf("decode ws event: %v", err)
				}
				if event.Type == agentpkg.EventUserInputRequest || event.ToolName == userinput.ToolNameAskUser {
					if !messages.persisted.Load() {
						t.Fatalf("ask_user event %q was published before its terminal response persisted", event.Type)
					}
				}
				if event.Type == agentpkg.EventUserInputRequest {
					promptSeen = true
				}
			}
			if promptSeen != tt.wantPrompt || messages.persisted.Load() != tt.wantPersist {
				t.Fatalf("prompt/persisted = %t/%t, want %t/%t", promptSeen, messages.persisted.Load(), tt.wantPrompt, tt.wantPersist)
			}
			if attempts := messages.persistAttempts.Load(); attempts != tt.wantAttempts {
				t.Fatalf("persistence attempts = %d, want %d", attempts, tt.wantAttempts)
			}
			for _, input := range messages.recordingMessageService.persisted {
				if input.Role == "tool" {
					t.Fatalf("durable ask_user was closed by fallback tool result: %#v", messages.recordingMessageService.persisted)
				}
			}
			err := <-done
			wantErr := tt.wantErr
			if wantErr == nil {
				wantErr = tt.persistErr
			}
			if wantErr == nil && err != nil {
				t.Fatalf("StreamChatWS() error = %v", err)
			}
			if wantErr != nil && !errors.Is(err, wantErr) {
				t.Fatalf("StreamChatWS() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func streamWorkspaceFailureSessionService(wantErr error) *fakeBackgroundSessionService {
	return &fakeBackgroundSessionService{
		getFn: func(context.Context, string) (sessionpkg.Session, error) {
			return sessionpkg.Session{Metadata: map[string]any{}}, nil
		},
		updateMetadataFn: func(context.Context, string, map[string]any) (sessionpkg.Session, error) {
			return sessionpkg.Session{}, wantErr
		},
	}
}

func newAskUserStreamServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"ask-1\",\"type\":\"function\",\"function\":{\"name\":\"ask_user\",\"arguments\":\"{\\\"questions\\\":[{\\\"text\\\":\\\"Continue?\\\",\\\"kind\\\":\\\"text\\\"}]}\"}}]},\"finish_reason\":null}]}\n\n"+
				"data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"+
				"data: [DONE]\n\n",
		)
	}))
}
