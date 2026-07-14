package flow

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/runtimefence"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
)

const (
	liveApprovalBotID     = "11111111-1111-1111-1111-111111111111"
	liveApprovalSessionID = "22222222-2222-2222-2222-222222222222"
	liveApprovalActorID   = "77777777-7777-7777-7777-777777777777"
)

func authorizeLiveApprovalResolver(resolver *Resolver, creatorID string, permissions ...string) {
	resolver.sessionService = &fakeBackgroundSessionService{getFn: func(context.Context, string) (session.Session, error) {
		return session.Session{
			ID: liveApprovalSessionID, BotID: liveApprovalBotID, Type: session.TypeChat,
			SessionMode: session.TypeChat, RuntimeType: session.RuntimeModel, CreatedByUserID: creatorID,
		}, nil
	}}
	values := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		values[liveApprovalBotID+":"+liveApprovalActorID+":"+permission] = true
	}
	resolver.botPermissions = &fakeBotPermissionChecker{values: values}
}

type liveApprovalQueries struct {
	dbstore.Queries
	row          sqlc.ToolApprovalRequest
	approveCalls int
}

type terminalReplayFailureQueries struct {
	dbstore.Queries
	row   sqlc.ToolApprovalRequest
	calls int
	err   error
}

func (q *terminalReplayFailureQueries) GetToolApprovalRequest(context.Context, pgtype.UUID) (sqlc.ToolApprovalRequest, error) {
	q.calls++
	if q.calls == 1 {
		return q.row, nil
	}
	return sqlc.ToolApprovalRequest{}, q.err
}

func (q *liveApprovalQueries) GetToolApprovalRequest(context.Context, pgtype.UUID) (sqlc.ToolApprovalRequest, error) {
	return q.row, nil
}

func (q *liveApprovalQueries) ApproveToolApprovalRequest(context.Context, sqlc.ApproveToolApprovalRequestParams) (sqlc.ToolApprovalRequest, error) {
	q.approveCalls++
	approved := q.row
	approved.Status = toolapproval.StatusApproved
	return approved, nil
}

func TestRespondToolApprovalWithLiveWaiterOnlyResolvesDecision(t *testing.T) {
	approvalID := "33333333-3333-3333-3333-333333333333"
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID:         flowTestUUID(approvalID),
		BotID:      flowTestUUID("11111111-1111-1111-1111-111111111111"),
		SessionID:  flowTestUUID("22222222-2222-2222-2222-222222222222"),
		ToolCallID: "call-1",
		ToolName:   "exec",
		ToolInput:  []byte(`{"command":"true"}`),
		Status:     toolapproval.StatusPending,
	}}
	service := toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil)
	release := service.RegisterWaiter(approvalID)
	defer release()
	resolver := &Resolver{}
	resolver.SetToolApprovalService(service)
	authorizeLiveApprovalResolver(resolver, liveApprovalActorID, bots.PermissionChat, bots.PermissionWorkspaceExec)
	events := make(chan WSStreamEvent, 2)

	if err := resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID:       liveApprovalBotID,
		SessionID:   liveApprovalSessionID,
		ApprovalID:  approvalID,
		Decision:    "approve",
		ActorUserID: liveApprovalActorID,
	}, events); err != nil {
		t.Fatalf("RespondToolApproval() error = %v", err)
	}
	if queries.approveCalls != 1 {
		t.Fatalf("approve calls = %d, want 1", queries.approveCalls)
	}
	if len(events) != 2 {
		t.Fatalf("ack event count = %d, want 2", len(events))
	}
}

func TestRespondToolApprovalResolveOnlyDoesNotRequireLocalWaiter(t *testing.T) {
	approvalID := "44444444-4444-4444-4444-444444444444"
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID:         flowTestUUID(approvalID),
		BotID:      flowTestUUID("11111111-1111-1111-1111-111111111111"),
		SessionID:  flowTestUUID("22222222-2222-2222-2222-222222222222"),
		ToolCallID: "call-remote",
		ToolName:   "exec",
		ToolInput:  []byte(`{"command":"true"}`),
		Status:     toolapproval.StatusPending,
	}}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	authorizeLiveApprovalResolver(resolver, liveApprovalActorID, bots.PermissionChat, bots.PermissionWorkspaceExec)

	if err := resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID:       liveApprovalBotID,
		SessionID:   liveApprovalSessionID,
		ApprovalID:  approvalID,
		Decision:    "approve",
		ResolveOnly: true,
		ActorUserID: liveApprovalActorID,
	}, nil); err != nil {
		t.Fatalf("RespondToolApproval(resolve only) error = %v", err)
	}
	if queries.approveCalls != 1 {
		t.Fatalf("approve calls = %d, want 1", queries.approveCalls)
	}
}

func TestRespondToolApprovalResolveOnlyReconcilesCommittedRetry(t *testing.T) {
	approvalID := "45454545-4545-4545-4545-454545454545"
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID: flowTestUUID(approvalID), BotID: flowTestUUID(liveApprovalBotID), SessionID: flowTestUUID(liveApprovalSessionID),
		ToolCallID: "call-committed", ToolName: "exec", ToolInput: []byte(`{"command":"true"}`),
		Status: toolapproval.StatusApproved, DecisionReason: "looks safe",
	}}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	authorizeLiveApprovalResolver(resolver, liveApprovalActorID, bots.PermissionChat, bots.PermissionWorkspaceExec)

	err := resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID: liveApprovalBotID, SessionID: liveApprovalSessionID, ApprovalID: approvalID,
		Decision: "approve", Reason: "looks safe", ResolveOnly: true, ActorUserID: liveApprovalActorID,
	}, nil)
	if err != nil {
		t.Fatalf("same committed retry error = %v", err)
	}
	if queries.approveCalls != 0 {
		t.Fatalf("same committed retry executed approval %d time(s)", queries.approveCalls)
	}

	err = resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID: liveApprovalBotID, SessionID: liveApprovalSessionID, ApprovalID: approvalID,
		Decision: "reject", Reason: "looks safe", ResolveOnly: true, ActorUserID: liveApprovalActorID,
	}, nil)
	if !errors.Is(err, toolapproval.ErrAlreadyDecided) {
		t.Fatalf("conflicting committed retry error = %v, want ErrAlreadyDecided", err)
	}
}

func TestRespondToolApprovalCommittedRetryPropagatesLookupFailure(t *testing.T) {
	approvalID := "46464646-4646-4646-4646-464646464646"
	lookupErr := errors.New("approval database unavailable")
	queries := &terminalReplayFailureQueries{row: sqlc.ToolApprovalRequest{
		ID: flowTestUUID(approvalID), BotID: flowTestUUID(liveApprovalBotID), SessionID: flowTestUUID(liveApprovalSessionID),
		ToolCallID: "call-lookup-failure", ToolName: "exec", Status: toolapproval.StatusApproved,
	}, err: lookupErr}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	authorizeLiveApprovalResolver(resolver, liveApprovalActorID, bots.PermissionChat, bots.PermissionWorkspaceExec)

	err := resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID: liveApprovalBotID, SessionID: liveApprovalSessionID, ApprovalID: approvalID,
		Decision: "approve", ResolveOnly: true, ActorUserID: liveApprovalActorID,
	}, nil)
	if !errors.Is(err, lookupErr) {
		t.Fatalf("committed retry lookup error = %v, want %v", err, lookupErr)
	}
}

func TestPrepareToolApprovalResponseDoesNotResolveDecision(t *testing.T) {
	t.Parallel()

	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		approvalID = "55555555-5555-5555-5555-555555555555"
	)
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID:         flowTestUUID(approvalID),
		BotID:      flowTestUUID(botID),
		SessionID:  flowTestUUID(sessionID),
		ToolCallID: "call-prepare",
		ToolName:   "exec",
		ToolInput:  []byte(`{"command":"true"}`),
		Status:     toolapproval.StatusPending,
	}}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	authorizeLiveApprovalResolver(resolver, liveApprovalActorID, bots.PermissionChat, bots.PermissionWorkspaceExec)

	prepared, err := resolver.PrepareToolApprovalResponse(context.Background(), ToolApprovalResponseInput{
		BotID: botID, SessionID: sessionID, ApprovalID: approvalID, Decision: "approve", ActorUserID: liveApprovalActorID,
	})
	if err != nil {
		t.Fatalf("PrepareToolApprovalResponse() error = %v", err)
	}
	if prepared != (runtimefence.PreservedDecision{Kind: runtimefence.DecisionToolApproval, ID: approvalID, BotID: botID, SessionID: sessionID}) {
		t.Fatalf("prepared decision = %#v", prepared)
	}
	if queries.approveCalls != 0 {
		t.Fatalf("prepare resolved approval %d time(s)", queries.approveCalls)
	}
}

func TestPrepareToolApprovalResponseReturnsCanonicalScopeAfterDecision(t *testing.T) {
	const (
		botID      = "11111111-1111-1111-1111-111111111111"
		sessionID  = "22222222-2222-2222-2222-222222222222"
		approvalID = "55555555-5555-5555-5555-555555555555"
	)
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID: flowTestUUID(approvalID), BotID: flowTestUUID(botID), SessionID: flowTestUUID(sessionID),
		ToolCallID: "call-decided", ToolName: "exec", ToolInput: []byte(`{"command":"true"}`), Status: toolapproval.StatusApproved,
	}}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	authorizeLiveApprovalResolver(resolver, liveApprovalActorID, bots.PermissionChat, bots.PermissionWorkspaceExec)
	if _, err := resolver.PrepareToolApprovalResponse(context.Background(), ToolApprovalResponseInput{
		BotID: botID, ApprovalID: approvalID, Decision: "approve", ActorUserID: liveApprovalActorID,
	}); !errors.Is(err, toolapproval.ErrNotFound) {
		t.Fatalf("pending-only prepare error = %v, want ErrNotFound", err)
	}

	prepared, err := resolver.PrepareToolApprovalResponseTarget(context.Background(), ToolApprovalResponseInput{
		BotID: botID, ApprovalID: approvalID, Decision: "approve", ActorUserID: liveApprovalActorID,
	})
	if err != nil {
		t.Fatalf("PrepareToolApprovalResponseTarget() error = %v", err)
	}
	want := runtimefence.PreservedDecision{Kind: runtimefence.DecisionToolApproval, ID: approvalID, BotID: botID, SessionID: sessionID}
	if prepared != want {
		t.Fatalf("prepared decision = %#v, want %#v", prepared, want)
	}
}

func TestRespondToolApprovalRejectsActorWithoutSessionAccess(t *testing.T) {
	t.Parallel()

	const approvalID = "88888888-8888-8888-8888-888888888888"
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID: flowTestUUID(approvalID), BotID: flowTestUUID(liveApprovalBotID), SessionID: flowTestUUID(liveApprovalSessionID),
		ToolCallID: "call-forbidden", ToolName: "exec", ToolInput: []byte(`{"command":"true"}`), Status: toolapproval.StatusPending,
	}}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	authorizeLiveApprovalResolver(resolver, "99999999-9999-9999-9999-999999999999", bots.PermissionChat)

	err := resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID: liveApprovalBotID, SessionID: liveApprovalSessionID, ApprovalID: approvalID,
		Decision: "approve", ActorUserID: liveApprovalActorID,
	}, nil)
	if !errors.Is(err, toolapproval.ErrForbidden) {
		t.Fatalf("response error = %v, want tool approval forbidden", err)
	}
	if queries.approveCalls != 0 {
		t.Fatalf("forbidden actor approved request %d time(s)", queries.approveCalls)
	}
}

func TestRespondToolApprovalFencedACPRequestWithoutWaiterFailsClosed(t *testing.T) {
	t.Parallel()

	approvalID := "66666666-6666-6666-6666-666666666666"
	queries := &liveApprovalQueries{row: sqlc.ToolApprovalRequest{
		ID:                  flowTestUUID(approvalID),
		BotID:               flowTestUUID("11111111-1111-1111-1111-111111111111"),
		SessionID:           flowTestUUID("22222222-2222-2222-2222-222222222222"),
		ToolCallID:          "call-fenced",
		ToolName:            "exec",
		ToolInput:           []byte(`{"command":"true"}`),
		Status:              toolapproval.StatusPending,
		RuntimeFencingToken: pgtype.Int8{Int64: 7, Valid: true},
	}}
	resolver := &Resolver{}
	resolver.SetToolApprovalService(toolapproval.NewService(slog.New(slog.DiscardHandler), queries, nil))
	resolver.botPermissions = &fakeBotPermissionChecker{values: map[string]bool{
		"11111111-1111-1111-1111-111111111111:" + testACPUserInputOwnerID + ":" + bots.PermissionWorkspaceExec: true,
	}}
	resolver.sessionService = &fakeBackgroundSessionService{getFn: func(context.Context, string) (session.Session, error) {
		return session.Session{
			ID: "22222222-2222-2222-2222-222222222222", BotID: "11111111-1111-1111-1111-111111111111",
			Type: session.TypeACPAgent, RuntimeType: session.RuntimeACPAgent,
			RuntimeMetadata: map[string]any{"runtime_owner_account_id": testACPUserInputOwnerID},
		}, nil
	}}
	err := resolver.RespondToolApproval(context.Background(), ToolApprovalResponseInput{
		BotID: "11111111-1111-1111-1111-111111111111", SessionID: "22222222-2222-2222-2222-222222222222",
		ApprovalID: approvalID, Decision: "approve", ActorUserID: testACPUserInputOwnerID,
	}, nil)
	if !errors.Is(err, ErrRuntimeDecisionOwnerUnavailable) {
		t.Fatalf("respond fenced orphan error = %v, want ErrRuntimeDecisionOwnerUnavailable", err)
	}
	if queries.approveCalls != 0 {
		t.Fatalf("fenced orphan approved %d time(s)", queries.approveCalls)
	}
}
