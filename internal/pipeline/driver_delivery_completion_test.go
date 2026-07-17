package pipeline

import (
	"context"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type deliveryClaimResolver struct {
	*fakeRunConfigResolver
	commits chan DiscussCursorCommit
}

func (r *deliveryClaimResolver) StoreRoundWithCursor(
	_ context.Context,
	_, _, _, _, _ string,
	_ []sdk.Message,
	_ string,
	commit DiscussCursorCommit,
) (bool, error) {
	r.commits <- commit
	return true, nil
}

type deliveryClaimRuntime struct {
	requests chan conversation.ChatRequest
}

func (r *deliveryClaimRuntime) StreamChat(
	_ context.Context,
	req conversation.ChatRequest,
) (<-chan conversation.StreamChunk, <-chan error) {
	r.requests <- req
	chunks := make(chan conversation.StreamChunk, 1)
	errs := make(chan error)
	chunks <- conversation.StreamChunk(`{"type":"agent_end","metadata":{"discuss_cursor_committed":true}}`)
	close(chunks)
	close(errs)
	return chunks, errs
}

func TestDiscussEventDeliveryClaimsRequireEveryLease(t *testing.T) {
	t.Parallel()

	first := claimDiscussDelivery(t, "33333333-3333-3333-3333-333333333333", 10)
	second := claimDiscussDelivery(t, "44444444-4444-4444-4444-444444444444", 20)

	claims, err := discussEventDeliveryClaims([]DiscussEventDelivery{first, second})
	if err != nil {
		t.Fatalf("discussEventDeliveryClaims() error = %v", err)
	}
	if len(claims) != 2 ||
		claims[0].EventID != first.Lease.eventID.String() || claims[0].ClaimToken != first.Lease.claimToken.String() ||
		claims[1].EventID != second.Lease.eventID.String() || claims[1].ClaimToken != second.Lease.claimToken.String() {
		t.Fatalf("delivery claims = %#v, want both owned leases", claims)
	}

	for _, deliveries := range [][]DiscussEventDelivery{
		{{EventID: first.EventID}},
		{{EventID: second.EventID, Lease: first.Lease}},
	} {
		if _, err := discussEventDeliveryClaims(deliveries); err == nil {
			t.Fatal("discussEventDeliveryClaims() error = nil")
		}
	}
}

func TestRunSessionThreadsEveryMergedDeliveryClaim(t *testing.T) {
	t.Parallel()

	t.Run("model", func(t *testing.T) {
		first := claimDiscussDelivery(t, "33333333-3333-3333-3333-333333333333", 10)
		second := claimDiscussDelivery(t, "44444444-4444-4444-4444-444444444444", 20)
		resolver := &deliveryClaimResolver{
			fakeRunConfigResolver: &fakeRunConfigResolver{},
			commits:               make(chan DiscussCursorCommit, 1),
		}
		driver := NewDiscussDriver(DiscussDriverDeps{
			Agent: &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{
				Type:     agentpkg.EventAgentEnd,
				Messages: []byte(`[{"role":"assistant","content":[{"type":"text","text":"reply"}]}]`),
			}}},
			Resolver: resolver,
		})
		stop := runMergedDeliverySession(t, driver, first, second)
		defer stop()

		select {
		case commit := <-resolver.commits:
			assertDeliveryClaims(t, commit.DeliveryClaims, first, second)
		case <-time.After(time.Second):
			t.Fatal("model persistence did not receive merged delivery claims")
		}
	})

	t.Run("ACP", func(t *testing.T) {
		first := claimDiscussDelivery(t, "55555555-5555-5555-5555-555555555555", 10)
		second := claimDiscussDelivery(t, "66666666-6666-6666-6666-666666666666", 20)
		runtime := &deliveryClaimRuntime{requests: make(chan conversation.ChatRequest, 1)}
		driver := NewDiscussDriver(DiscussDriverDeps{
			Resolver:        &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent}},
			RuntimeStreamer: runtime,
		})
		stop := runMergedDeliverySession(t, driver, first, second)
		defer stop()

		select {
		case req := <-runtime.requests:
			claims := make([]DeliveryClaim, len(req.DiscussDeliveryClaims))
			for i, claim := range req.DiscussDeliveryClaims {
				claims[i] = DeliveryClaim{EventID: claim.EventID, ClaimToken: claim.ClaimToken}
			}
			assertDeliveryClaims(t, claims, first, second)
		case <-time.After(time.Second):
			t.Fatal("ACP runtime did not receive merged delivery claims")
		}
	})
}

func TestRunSessionRejectsIncompleteMergedDeliveryClaims(t *testing.T) {
	t.Parallel()

	first := claimDiscussDelivery(t, "77777777-7777-7777-7777-777777777777", 10)
	missing := DiscussEventDelivery{EventID: "88888888-8888-8888-8888-888888888888", EventCursor: 20}
	runtime := &fakeDiscussRuntimeStreamer{}
	agent := &fakeDiscussStreamer{}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Agent:           agent,
		Resolver:        &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent}},
		RuntimeStreamer: runtime,
	})
	stop := runMergedDeliverySession(t, driver, first, missing)

	select {
	case <-first.Lease.Done():
	case <-time.After(time.Second):
		stop()
		t.Fatal("invalid merged delivery was not released")
	}
	stop()
	if runtime.calls != 0 || agent.lastConfig != nil {
		t.Fatalf("runtime/model calls = %d/%#v, want zero before claim validation", runtime.calls, agent.lastConfig)
	}
}

func claimDiscussDelivery(t *testing.T, eventID string, cursor int64) DiscussEventDelivery {
	t.Helper()
	lease, err := newTestLeaseStore(&leaseQueries{now: time.Now()}, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		eventID,
	)
	if err != nil || lease == nil {
		t.Fatalf("claim delivery %s = %#v, %v", eventID, lease, err)
	}
	t.Cleanup(func() { _ = lease.Release(context.Background()) })
	return DiscussEventDelivery{EventID: eventID, EventCursor: cursor, Lease: lease}
}

func runMergedDeliverySession(
	t *testing.T,
	driver *DiscussDriver,
	first DiscussEventDelivery,
	second DiscussEventDelivery,
) func() {
	t.Helper()
	config := DiscussSessionConfig{
		BotID:                  "bot",
		SessionID:              "session",
		ConversationType:       channel.ConversationTypeGroup,
		PersistedUserMessageID: "request",
	}
	firstConfig := config
	firstConfig.EventDelivery = &first
	secondConfig := config
	secondConfig.EventDelivery = &second
	firstRC := RenderedContext{{
		MessageID:       "first",
		ReceivedAtMs:    100,
		LastEventCursor: first.EventCursor,
		Content:         []RenderedContentPiece{{Type: "text", Text: "first"}},
	}}
	secondRC := append(append(RenderedContext(nil), firstRC...), RenderedSegment{
		MessageID:       "second",
		ReceivedAtMs:    200,
		LastEventCursor: second.EventCursor,
		MentionsMe:      true,
		Content:         []RenderedContentPiece{{Type: "text", Text: "second"}},
	})
	sess := &discussSession{
		config: secondConfig,
		rcCh:   make(chan discussNotification, 2),
		stopCh: make(chan struct{}),
		cancel: func() {},
	}
	sess.rcCh <- discussNotification{rc: firstRC, config: firstConfig, deliveries: []DiscussEventDelivery{first}}
	sess.rcCh <- discussNotification{rc: secondRC, config: secondConfig, deliveries: []DiscussEventDelivery{second}}
	done := make(chan struct{})
	go func() {
		driver.runSession(context.Background(), sess)
		close(done)
	}()
	return func() {
		select {
		case <-sess.stopCh:
		default:
			close(sess.stopCh)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("discuss session did not stop")
		}
	}
}

func assertDeliveryClaims(
	t *testing.T,
	claims []DeliveryClaim,
	first DiscussEventDelivery,
	second DiscussEventDelivery,
) {
	t.Helper()
	if len(claims) != 2 ||
		claims[0].EventID != first.EventID || claims[0].ClaimToken != first.Lease.claimToken.String() ||
		claims[1].EventID != second.EventID || claims[1].ClaimToken != second.Lease.claimToken.String() {
		t.Fatalf("delivery claims = %#v, want both merged claims", claims)
	}
}
