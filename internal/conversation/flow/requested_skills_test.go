package flow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/slash"
)

func requestedSkillGuardRequest(sessionID string) conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:     "bot-1",
		SessionID: sessionID,
		RequestedSkills: []conversation.RequestedSkillContext{{
			Name:    "alpha",
			Content: "skill content",
		}},
	}
}

func TestRejectRequestedSkillsAllowsModelChatSession(t *testing.T) {
	resolver := &Resolver{
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return session.Session{
					ID:          sessionID,
					BotID:       "bot-1",
					Type:        session.TypeChat,
					SessionMode: session.TypeChat,
					RuntimeType: session.RuntimeModel,
				}, nil
			},
		},
	}

	req := requestedSkillGuardRequest("session-1")
	req.InjectCh = make(chan conversation.InjectMessage)
	if err := resolver.rejectRequestedSkillsIfUnsupportedContext(context.Background(), req); err != nil {
		t.Fatalf("rejectRequestedSkillsIfUnsupportedContext() error = %v, want nil", err)
	}
}

func TestRejectRequestedSkillsAllowsPersistedSkillActivation(t *testing.T) {
	resolver := &Resolver{
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return session.Session{
					ID:          sessionID,
					BotID:       "bot-1",
					Type:        session.TypeChat,
					SessionMode: session.TypeChat,
					RuntimeType: session.RuntimeModel,
				}, nil
			},
		},
	}

	req := requestedSkillGuardRequest("session-1")
	req.UserMessagePersisted = true
	req.UserMessageKind = conversation.UserMessageKindSkillActivation
	if err := resolver.rejectRequestedSkillsIfUnsupportedContext(context.Background(), req); err != nil {
		t.Fatalf("rejectRequestedSkillsIfUnsupportedContext() error = %v, want nil", err)
	}
}

func TestRejectReservedSkillMetadataIfPresentRejectsAttachments(t *testing.T) {
	req := conversation.ChatRequest{
		Attachments: []conversation.ChatAttachment{{
			Type:     "image",
			Metadata: map[string]any{"model_requested_skills": []string{"alpha"}},
		}},
	}
	err := rejectReservedSkillMetadataIfPresent(req)
	var slashErr slash.Error
	if !errors.As(err, &slashErr) || slashErr.Code != slash.CodeReservedSkillMetadata {
		t.Fatalf("err = %#v, want %s", err, slash.CodeReservedSkillMetadata)
	}
}

func TestRejectReservedSkillMetadataIfPresentRejectsMessageContentPartMetadata(t *testing.T) {
	content, err := json.Marshal([]conversation.ContentPart{{
		Type:     "text",
		Text:     "hello",
		Metadata: map[string]any{"requested_skills": []string{"alpha"}},
	}})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	req := conversation.ChatRequest{
		Messages: []conversation.ModelMessage{{
			Role:    "user",
			Content: content,
		}},
	}

	err = rejectReservedSkillMetadataIfPresent(req)
	var slashErr slash.Error
	if !errors.As(err, &slashErr) || slashErr.Code != slash.CodeReservedSkillMetadata {
		t.Fatalf("err = %#v, want %s", err, slash.CodeReservedSkillMetadata)
	}
}

func TestRejectRequestedSkillsRejectsUnsupportedContexts(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		mutate    func(*conversation.ChatRequest)
		session   session.Session
	}{
		{
			name:      "missing session",
			sessionID: "",
		},
		{
			name:      "discuss session",
			sessionID: "session-1",
			session: session.Session{
				ID:          "session-1",
				BotID:       "bot-1",
				Type:        session.TypeDiscuss,
				SessionMode: session.TypeDiscuss,
				RuntimeType: session.RuntimeModel,
			},
		},
		{
			name:      "schedule run mode",
			sessionID: "session-1",
			mutate: func(req *conversation.ChatRequest) {
				req.SessionType = session.TypeSchedule
			},
			session: session.Session{
				ID:          "session-1",
				BotID:       "bot-1",
				Type:        session.TypeChat,
				SessionMode: session.TypeChat,
				RuntimeType: session.RuntimeModel,
			},
		},
		{
			name:      "already persisted user message",
			sessionID: "session-1",
			mutate: func(req *conversation.ChatRequest) {
				req.UserMessagePersisted = true
			},
			session: session.Session{
				ID:          "session-1",
				BotID:       "bot-1",
				Type:        session.TypeChat,
				SessionMode: session.TypeChat,
				RuntimeType: session.RuntimeModel,
			},
		},
		{
			name:      "ACP runtime",
			sessionID: "session-1",
			session: session.Session{
				ID:          "session-1",
				BotID:       "bot-1",
				Type:        session.TypeChat,
				SessionMode: session.TypeChat,
				RuntimeType: session.RuntimeACPAgent,
			},
		},
		{
			name:      "legacy ACP session",
			sessionID: "session-1",
			session: session.Session{
				ID:    "session-1",
				BotID: "bot-1",
				Type:  session.TypeACPAgent,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &Resolver{
				sessionService: &fakeBackgroundSessionService{
					getFn: func(_ context.Context, sessionID string) (session.Session, error) {
						if sessionID != tc.sessionID {
							t.Fatalf("session id = %q, want %q", sessionID, tc.sessionID)
						}
						return tc.session, nil
					},
				},
			}
			if tc.sessionID == "" {
				resolver.sessionService = nil
			}

			req := requestedSkillGuardRequest(tc.sessionID)
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			err := resolver.rejectRequestedSkillsIfUnsupportedContext(context.Background(), req)
			var slashErr slash.Error
			if !errors.As(err, &slashErr) || slashErr.Code != slash.CodeUnsupportedSkillSlashContext {
				t.Fatalf("error = %v, want %s", err, slash.CodeUnsupportedSkillSlashContext)
			}
		})
	}
}

func TestResolveRejectsRequestedSkillsBeforeModelResolution(t *testing.T) {
	resolver := &Resolver{}
	_, err := resolver.resolve(context.Background(), conversation.ChatRequest{
		BotID:       "bot-1",
		ChatID:      "chat-1",
		SessionID:   "session-1",
		SessionType: session.TypeSchedule,
		Query:       "scheduled prompt",
		RequestedSkills: []conversation.RequestedSkillContext{{
			Name:    "alpha",
			Content: "skill content",
		}},
	})
	var slashErr slash.Error
	if !errors.As(err, &slashErr) || slashErr.Code != slash.CodeUnsupportedSkillSlashContext {
		t.Fatalf("resolve() error = %v, want %s", err, slash.CodeUnsupportedSkillSlashContext)
	}
}

func TestStreamChatWSRejectsACPRequestedSkillsBeforePool(t *testing.T) {
	pool := &recordingACPPrompter{}
	resolver := &Resolver{
		acpPool:        pool,
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
	}

	err := resolver.StreamChatWS(
		context.Background(),
		requestedSkillGuardRequest("session-1"),
		make(chan WSStreamEvent, 1),
		make(chan struct{}),
	)
	var slashErr slash.Error
	if !errors.As(err, &slashErr) || slashErr.Code != slash.CodeUnsupportedSkillSlashContext {
		t.Fatalf("StreamChatWS() error = %v, want %s", err, slash.CodeUnsupportedSkillSlashContext)
	}
	if pool.calls != 0 {
		t.Fatalf("ACP pool calls = %d, want 0", pool.calls)
	}
}
