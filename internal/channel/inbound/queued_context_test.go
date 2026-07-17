package inbound

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/acl"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type queuedContextKey struct{}

type queuedIdentityACL struct {
	allowedChannelIdentityID string
	calls                    []queuedIdentityACLCall
}

type queuedIdentityACLCall struct {
	channelIdentityID string
	contextOwner      string
	contextErr        error
}

type queuedRevokedACL struct {
	calls int
}

type queuedRevokedPermission struct {
	calls int
}

func (p *queuedRevokedPermission) HasBotPermission(context.Context, string, string, string) (bool, error) {
	p.calls++
	return p.calls == 1, nil
}

func (a *queuedRevokedACL) Evaluate(context.Context, acl.EvaluateRequest) (bool, error) {
	a.calls++
	return a.calls == 1, nil
}

func (a *queuedIdentityACL) Evaluate(ctx context.Context, req acl.EvaluateRequest) (bool, error) {
	a.calls = append(a.calls, queuedIdentityACLCall{
		channelIdentityID: req.ChannelIdentityID,
		contextOwner:      ctx.Value(queuedContextKey{}).(string),
		contextErr:        ctx.Err(),
	})
	return req.ChannelIdentityID == a.allowedChannelIdentityID, nil
}

func TestDrainQueueUsesQueuedTaskContextAndIdentity(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	aclService := &queuedIdentityACL{allowedChannelIdentityID: "queued-channel-identity"}
	processor.SetACLService(aclService)
	dispatcher.MarkActive("route")

	queuedCtx, cancelQueued := context.WithCancel(WithIdentityState(
		context.WithValue(context.Background(), queuedContextKey{}, "queued"),
		IdentityState{Identity: InboundIdentity{
			BotID:             deliveryBotID,
			ChannelIdentityID: "queued-channel-identity",
			UserID:            "queued-user",
			DisplayName:       "Queued User",
			ForceReply:        true,
		}},
	))
	if err := processor.HandleInbound(queuedCtx, deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	cancelQueued()

	activeCtx := WithIdentityState(
		context.WithValue(context.Background(), queuedContextKey{}, "active"),
		IdentityState{Identity: InboundIdentity{
			BotID:             deliveryBotID,
			ChannelIdentityID: "active-channel-identity",
			UserID:            "active-user",
			DisplayName:       "Active User",
			ForceReply:        true,
		}},
	)
	processor.drainQueue(activeCtx, "route")

	if len(aclService.calls) != 2 {
		t.Fatalf("ACL calls = %d, want enqueue and replay", len(aclService.calls))
	}
	for i, call := range aclService.calls {
		if call.channelIdentityID != "queued-channel-identity" || call.contextOwner != "queued" || call.contextErr != nil {
			t.Fatalf("ACL call %d = %+v, want queued identity/context without cancellation", i, call)
		}
	}
	if gateway.gotReq.UserID != "queued-user" ||
		gateway.gotReq.SourceChannelIdentityID != "queued-channel-identity" ||
		gateway.gotReq.DisplayName != "Queued User" {
		t.Fatalf("queued ChatRequest identity = user:%q channel:%q name:%q",
			gateway.gotReq.UserID,
			gateway.gotReq.SourceChannelIdentityID,
			gateway.gotReq.DisplayName,
		)
	}
}

func TestQueuedReplayTerminatesWhenACLWasRevokedAfterEnqueue(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	aclService := &queuedRevokedACL{}
	processor.SetACLService(aclService)
	dispatcher.MarkActive("route")
	sender := &fakeReplySender{}
	queuedCtx := WithIdentityState(context.Background(), IdentityState{Identity: InboundIdentity{
		BotID:             deliveryBotID,
		ChannelIdentityID: "queued-channel-identity",
		UserID:            "queued-user",
		DisplayName:       "Queued User",
		ForceReply:        true,
	}})

	if err := processor.HandleInbound(queuedCtx, deliveryConfig(), queuedDeliveryMessage(), sender); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	processor.drainQueue(context.Background(), "route")

	if aclService.calls != 2 {
		t.Fatalf("ACL calls = %d, want enqueue and replay", aclService.calls)
	}
	if writer.calls != 1 {
		t.Fatalf("history writes = %d, want only the queued request", writer.calls)
	}
	if writer.completionCalls != 1 {
		t.Fatalf("pending completion calls = %d, want one terminal completion", writer.completionCalls)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("ACL-revoked queued delivery was not completed")
	}
	if dispatcher.IsActive("route") {
		t.Fatal("ACL-revoked queued delivery kept the route active")
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("ACL-revoked queued delivery reached gateway with query %q", gateway.gotReq.Query)
	}
}

func TestQueuedReplayTerminatesWhenACPAccessWasRevokedAfterEnqueue(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	session := SessionResult{
		ID:                    deliverySessionID,
		Type:                  sessionpkg.TypeACPAgent,
		Runtime:               sessionpkg.RuntimeACPAgent,
		RuntimeOwnerAccountID: "queued-user",
	}
	processor.SetSessionEnsurer(&fakeSessionEnsurer{
		activeSession: session,
		sessions:      map[string]SessionResult{deliverySessionID: session},
	})
	permission := &queuedRevokedPermission{}
	processor.SetBotPermissionChecker(permission)
	dispatcher.MarkActive("route")
	sender := &fakeReplySender{}
	queuedCtx := WithIdentityState(context.Background(), IdentityState{Identity: InboundIdentity{
		BotID:             deliveryBotID,
		ChannelIdentityID: "queued-channel-identity",
		UserID:            "queued-user",
		DisplayName:       "Queued User",
		ForceReply:        true,
	}})

	if err := processor.HandleInbound(queuedCtx, deliveryConfig(), queuedDeliveryMessage(), sender); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	processor.drainQueue(context.Background(), "route")

	if permission.calls != 2 {
		t.Fatalf("permission calls = %d, want enqueue and replay", permission.calls)
	}
	if writer.calls != 1 || writer.completionCalls != 1 {
		t.Fatalf("history writes/completions = %d/%d, want 1/1", writer.calls, writer.completionCalls)
	}
	if !queries.deliveryIsCompleted() || dispatcher.IsActive("route") {
		t.Fatalf("ACP-revoked queued delivery completed/active = %t/%t, want true/false", queries.deliveryIsCompleted(), dispatcher.IsActive("route"))
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("ACP-revoked queued delivery reached gateway with query %q", gateway.gotReq.Query)
	}
}

func TestQueuedReplayCompletesBeforeBestEffortACPFeedback(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	timeline := make([]string, 0, 2)
	baseWriter := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	writer := &orderedCompletionWriter{base: baseWriter, timeline: &timeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	processor.queueRetryDelay = func(int) time.Duration { return time.Hour }
	session := SessionResult{
		ID:                    deliverySessionID,
		Type:                  sessionpkg.TypeACPAgent,
		Runtime:               sessionpkg.RuntimeACPAgent,
		RuntimeOwnerAccountID: "queued-user",
	}
	processor.SetSessionEnsurer(&fakeSessionEnsurer{
		activeSession: session,
		sessions:      map[string]SessionResult{deliverySessionID: session},
	})
	permission := &queuedRevokedPermission{}
	processor.SetBotPermissionChecker(permission)
	dispatcher.MarkActive("route")
	sender := &queuedLifecycleSender{timeline: &timeline, failSend: true}
	queuedCtx := WithIdentityState(context.Background(), IdentityState{Identity: InboundIdentity{
		BotID:             deliveryBotID,
		ChannelIdentityID: "queued-channel-identity",
		UserID:            "queued-user",
		DisplayName:       "Queued User",
		ForceReply:        true,
	}})

	if err := processor.HandleInbound(queuedCtx, deliveryConfig(), queuedDeliveryMessage(), sender); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	processor.drainQueue(context.Background(), "route")

	if !reflect.DeepEqual(timeline, []string{"complete", "send"}) {
		t.Fatalf("terminal ACP replay lifecycle = %#v, want durable completion before best-effort feedback", timeline)
	}
	if writer.base.completionCalls != 1 || !queries.deliveryIsCompleted() {
		t.Fatalf("pending completion calls/completed = %d/%t, want 1/true", writer.base.completionCalls, queries.deliveryIsCompleted())
	}
	if dispatcher.IsActive("route") {
		t.Fatal("failed ACP feedback kept the route active")
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("ACP-revoked queued delivery reached gateway with query %q", gateway.gotReq.Query)
	}
}

func TestQueuedReplayDoesNotRepeatACPFeedbackAfterTerminalCompletion(t *testing.T) {
	queries := &durableEventQueries{completionFailures: 1}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	timeline := make([]string, 0, 4)
	baseWriter := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	writer := &orderedCompletionWriter{base: baseWriter, timeline: &timeline}
	processor, dispatcher, _ := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	processor.queueRetryDelay = func(int) time.Duration { return time.Millisecond }
	session := SessionResult{
		ID:                    deliverySessionID,
		Type:                  sessionpkg.TypeACPAgent,
		Runtime:               sessionpkg.RuntimeACPAgent,
		RuntimeOwnerAccountID: "queued-user",
	}
	processor.SetSessionEnsurer(&fakeSessionEnsurer{
		activeSession: session,
		sessions:      map[string]SessionResult{deliverySessionID: session},
	})
	processor.SetBotPermissionChecker(&queuedRevokedPermission{})
	dispatcher.MarkActive("route")
	sender := &queuedLifecycleSender{timeline: &timeline}
	queuedCtx := WithIdentityState(context.Background(), IdentityState{Identity: InboundIdentity{
		BotID:             deliveryBotID,
		ChannelIdentityID: "queued-channel-identity",
		UserID:            "queued-user",
		DisplayName:       "Queued User",
		ForceReply:        true,
	}})

	if err := processor.HandleInbound(queuedCtx, deliveryConfig(), queuedDeliveryMessage(), sender); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	processor.drainQueue(context.Background(), "route")
	waitForDeliveryCompletion(t, processor, queries, deliveryEventID)
	processor.Close()

	feedbackSends := 0
	for _, event := range timeline {
		if event == "send" {
			feedbackSends++
		}
	}
	if feedbackSends != 1 {
		t.Fatalf("ACP feedback sends = %d, want 1 across delivery-completion retry; lifecycle = %#v", feedbackSends, timeline)
	}
	if writer.base.completionCalls != 2 {
		t.Fatalf("terminal history completion calls = %d, want idempotent retry", writer.base.completionCalls)
	}
	if dispatcher.IsActive("route") {
		t.Fatal("retried terminal ACP delivery kept the route active")
	}
}
