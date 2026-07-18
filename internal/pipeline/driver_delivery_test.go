package pipeline

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type blockingReleaseQueries struct {
	*leaseQueries
	releaseStarted chan struct{}
	releaseBlock   <-chan struct{}
	releaseOnce    sync.Once
}

func (q *blockingReleaseQueries) ReleaseSessionEventDelivery(
	ctx context.Context,
	arg sqlc.ReleaseSessionEventDeliveryParams,
) (int64, error) {
	q.releaseOnce.Do(func() { close(q.releaseStarted) })
	select {
	case <-q.releaseBlock:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return q.leaseQueries.ReleaseSessionEventDelivery(ctx, arg)
}

func TestMergeDiscussEventDeliveriesReplacesExpiredLease(t *testing.T) {
	queries := &leaseQueries{now: time.Now()}
	store := newTestLeaseStore(queries, time.Minute, time.Hour)
	oldLease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || oldLease == nil {
		t.Fatalf("claim old lease = %#v, %v", oldLease, err)
	}
	oldLease.markLost()
	queries.mu.Lock()
	queries.now = queries.now.Add(time.Minute + time.Millisecond)
	queries.mu.Unlock()
	newLease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || newLease == nil {
		t.Fatalf("claim replacement lease = %#v, %v", newLease, err)
	}
	defer func() { _ = newLease.Release(context.Background()) }()

	merged, discarded := mergeDiscussEventDeliveries([]DiscussEventDelivery{{EventID: "event-1", Lease: oldLease}}, []DiscussEventDelivery{{EventID: "event-1", Lease: newLease}})
	for _, delivery := range discarded {
		_ = delivery.Lease.Release(context.Background())
	}

	if len(merged) != 1 || merged[0].Lease != newLease || !merged[0].Lease.Active() {
		t.Fatalf("merged deliveries = %#v, want active replacement lease", merged)
	}
	if len(discarded) != 1 {
		t.Fatalf("discarded leases = %d, want 1", len(discarded))
	}
	queries.mu.Lock()
	currentToken := queries.token
	queries.mu.Unlock()
	if currentToken != newLease.claimToken {
		t.Fatal("discarding expired lease released the replacement claim")
	}
}

func TestBindDiscussEventDeliveriesCancelsWhenAnyLeaseIsLost(t *testing.T) {
	t.Parallel()

	first, err := newTestLeaseStore(&leaseQueries{now: time.Now()}, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || first == nil {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	second, err := newTestLeaseStore(&leaseQueries{now: time.Now()}, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"44444444-4444-4444-4444-444444444444",
	)
	if err != nil || second == nil {
		t.Fatalf("second claim = %#v, %v", second, err)
	}
	defer func() {
		_ = first.Release(context.Background())
		_ = second.Release(context.Background())
	}()

	ctx, cancel, active := bindDiscussEventDeliveries(context.Background(), []DiscussEventDelivery{
		{EventID: "event-10", Lease: first},
		{EventID: "event-20", Lease: second},
	})
	defer cancel()
	if !active {
		t.Fatal("bound deliveries are inactive")
	}
	second.markLost()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("bound context remained active after one delivery lease was lost")
	}
}

func TestDiscardedDiscussDeliveryReleaseDoesNotHoldDriverLock(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trigger func(*DiscussDriver, *discussSession, DiscussEventDelivery, DiscussEventDelivery) <-chan struct{}
	}{
		{
			name: "notification overflow",
			trigger: func(driver *DiscussDriver, sess *discussSession, oldDelivery, newDelivery DiscussEventDelivery) <-chan struct{} {
				sess.rcCh = make(chan discussNotification, 1)
				sess.rcCh <- discussNotification{rc: RenderedContext{{LastEventCursor: 1}}, deliveries: []DiscussEventDelivery{oldDelivery}}
				done := make(chan struct{})
				go func() {
					driver.NotifyRC(context.Background(), "session-a", RenderedContext{{LastEventCursor: 2}}, DiscussSessionConfig{
						SessionID:     "session-a",
						EventDelivery: &newDelivery,
					})
					close(done)
				}()
				return done
			},
		},
		{
			name: "idle drain",
			trigger: func(driver *DiscussDriver, sess *discussSession, oldDelivery, newDelivery DiscussEventDelivery) <-chan struct{} {
				sess.rcCh = make(chan discussNotification, 2)
				sess.rcCh <- discussNotification{rc: RenderedContext{{LastEventCursor: 1}}, deliveries: []DiscussEventDelivery{oldDelivery}}
				sess.rcCh <- discussNotification{rc: RenderedContext{{LastEventCursor: 2}}, deliveries: []DiscussEventDelivery{newDelivery}}
				done := make(chan struct{})
				go func() {
					_, _ = driver.takeQueuedNotificationOrRetire(context.Background(), "session-a", sess)
					close(done)
				}()
				return done
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			releaseBlock := make(chan struct{})
			queries := &blockingReleaseQueries{
				leaseQueries:   &leaseQueries{now: now},
				releaseStarted: make(chan struct{}),
				releaseBlock:   releaseBlock,
			}
			store := newTestLeaseStore(queries, time.Minute, time.Hour)
			oldLease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
			if err != nil || oldLease == nil {
				t.Fatalf("claim old lease = %#v, %v", oldLease, err)
			}
			oldLease.markLost()
			queries.mu.Lock()
			queries.now = now.Add(time.Minute + time.Millisecond)
			queries.mu.Unlock()
			newLease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
			if err != nil || newLease == nil {
				t.Fatalf("claim replacement lease = %#v, %v", newLease, err)
			}

			driver := NewDiscussDriver(DiscussDriverDeps{})
			sessA := &discussSession{rcCh: make(chan discussNotification, 2), stopCh: make(chan struct{}), cancel: func() {}}
			sessB := &discussSession{rcCh: make(chan discussNotification, 1), stopCh: make(chan struct{}), cancel: func() {}}
			driver.sessions["session-a"] = sessA
			driver.sessions["session-b"] = sessB
			oldDelivery := DiscussEventDelivery{EventID: "event-1", Lease: oldLease}
			newDelivery := DiscussEventDelivery{EventID: "event-1", Lease: newLease}
			triggerDone := tc.trigger(driver, sessA, oldDelivery, newDelivery)

			select {
			case <-queries.releaseStarted:
			case <-time.After(time.Second):
				close(releaseBlock)
				t.Fatal("discarded lease release did not start")
			}
			stopDone := make(chan struct{})
			go func() {
				driver.StopSession("session-b")
				close(stopDone)
			}()
			select {
			case <-stopDone:
			case <-time.After(100 * time.Millisecond):
				close(releaseBlock)
				<-triggerDone
				t.Fatal("blocked discarded lease release held the global driver lock")
			}

			close(releaseBlock)
			select {
			case <-triggerDone:
			case <-time.After(time.Second):
				t.Fatal("notification merge did not finish after release unblocked")
			}
			_ = newLease.Release(context.Background())
		})
	}
}

func TestLatestDiscussNotificationKeepsLastArrivingContextAndPairedConfig(t *testing.T) {
	t.Parallel()

	newer := discussNotification{
		rc:     RenderedContext{{LastEventCursor: 20}},
		config: DiscussSessionConfig{ReplyTarget: "newer"},
	}
	older := discussNotification{
		rc:     RenderedContext{{LastEventCursor: 10}},
		config: DiscussSessionConfig{ReplyTarget: "older"},
	}
	ch := make(chan discussNotification, 2)
	ch <- newer
	ch <- older

	latest := <-ch
	latest, _ = drainLatestDiscussNotification(ch, latest)

	if got := latestRCEventCursor(latest.rc); got != 10 {
		t.Fatalf("last arriving cursor = %d, want 10", got)
	}
	if latest.config.ReplyTarget != "older" {
		t.Fatalf("notification config = %#v, want config paired with last arriving RC", latest.config)
	}
}

func TestHandleReplyWithAgentUsesExactDeliveryBelowProcessedCursor(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{windowStartAtMs: 200}
	agent := &fakeDiscussStreamer{events: []agentpkg.StreamEvent{{
		Type:     agentpkg.EventAgentEnd,
		Messages: []byte(`[{"role":"assistant","content":[{"type":"text","text":"reply"}]}]`),
	}}}
	delivery := DiscussEventDelivery{EventID: "event-10", EventCursor: 10}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:         "bot",
			SessionID:     "session",
			EventDelivery: &delivery,
		},
		lastProcessedCursor: 20,
	}
	rc := RenderedContext{{
		MessageID:       "delayed",
		ReceivedAtMs:    100,
		LastEventCursor: 10,
		Content:         []RenderedContentPiece{{Type: "text", Text: "delayed first delivery"}},
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig == nil {
		t.Fatal("exact first delivery below the processed cursor did not call the agent")
	}
	if sess.lastProcessedCursor != 20 {
		t.Fatalf("processed cursor regressed to %d, want high-water 20", sess.lastProcessedCursor)
	}
}

func TestHandleReplyWithAgentMarksOnlyExactTriggerInSDKContext(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{}
	agent := &fakeDiscussStreamer{}
	delivery := DiscussEventDelivery{EventID: "event-10", EventCursor: 10}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "bot", SessionID: "session", EventDelivery: &delivery},
		lastProcessedCursor: 20,
	}
	rc := RenderedContext{
		{
			MessageID:       "delayed",
			ReceivedAtMs:    100,
			LastEventCursor: 10,
			Content:         []RenderedContentPiece{{Type: "text", Text: "delayed exact trigger"}},
		},
		{
			MessageID:       "higher",
			ReceivedAtMs:    300,
			LastEventCursor: 30,
			Content:         []RenderedContentPiece{{Type: "text", Text: "unselected higher-cursor message"}},
		},
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if agent.lastConfig == nil {
		t.Fatal("exact delayed trigger did not call the agent")
	}
	var marked []string
	for _, message := range agent.lastConfig.Messages {
		text := sdkMessageText([]sdk.Message{message})
		if strings.Contains(text, "[user; current-trigger]") {
			marked = append(marked, text)
		}
	}
	if len(marked) != 1 || !strings.Contains(marked[0], "delayed exact trigger") ||
		strings.Contains(marked[0], "unselected higher-cursor message") {
		t.Fatalf("marked SDK messages = %#v, want only delayed exact trigger", marked)
	}
}

func TestHandleReplyWithACPUsesExactMentionBelowProcessedCursor(t *testing.T) {
	t.Parallel()

	resolver := &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{RuntimeType: sessionpkg.RuntimeACPAgent}}
	runtime := &fakeDiscussRuntimeStreamer{}
	delivery := DiscussEventDelivery{EventID: "event-10", EventCursor: 10}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, RuntimeStreamer: runtime})
	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:            "bot",
			SessionID:        "session",
			ConversationType: channel.ConversationTypeGroup,
			EventDelivery:    &delivery,
		},
		lastProcessedCursor: 20,
	}
	rc := RenderedContext{{
		MessageID:       "delayed-mention",
		ReceivedAtMs:    100,
		LastEventCursor: 10,
		MentionsMe:      true,
		Content:         []RenderedContentPiece{{Type: "text", Text: "@bot delayed first delivery"}},
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, &fakeDiscussStreamer{})

	if runtime.calls != 1 {
		t.Fatalf("ACP runtime calls = %d, want 1 for exact delayed mention", runtime.calls)
	}
}

func TestHandleReplyWithAgentInlinesExactImageBelowProcessedCursor(t *testing.T) {
	t.Parallel()

	inlineCalls := 0
	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{RunConfig: agentpkg.RunConfig{SupportsImageInput: true}},
		inlineFn: func(_ context.Context, _ string, refs []ImageAttachmentRef) []sdk.ImagePart {
			inlineCalls++
			if len(refs) != 1 || refs[0].ContentHash != "image-10" {
				t.Fatalf("image refs = %#v, want exact image-10", refs)
			}
			return []sdk.ImagePart{{Image: "data:image/png;base64,EXACT", MediaType: "image/png"}}
		},
	}
	agent := &fakeDiscussStreamer{}
	delivery := DiscussEventDelivery{EventID: "event-10", EventCursor: 10}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver})
	sess := &discussSession{
		config:              DiscussSessionConfig{BotID: "bot", SessionID: "session", EventDelivery: &delivery},
		lastProcessedCursor: 20,
	}
	rc := RenderedContext{{
		MessageID:       "delayed-image",
		ReceivedAtMs:    100,
		LastEventCursor: 10,
		Content:         []RenderedContentPiece{{Type: "text", Text: "delayed image"}},
		ImageRefs:       []ImageAttachmentRef{{ContentHash: "image-10", Mime: "image/png"}},
	}}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, agent)

	if inlineCalls != 1 {
		t.Fatalf("inline image calls = %d, want 1 for exact delayed image", inlineCalls)
	}
	found := false
	for _, message := range agent.lastConfig.Messages {
		for _, part := range message.Content {
			if image, ok := part.(sdk.ImagePart); ok && image.Image == "data:image/png;base64,EXACT" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("exact delayed image was not attached to its rendered user message")
	}
}

func TestComposeContextKeepsExactTriggerSeparateFromUnselectedContext(t *testing.T) {
	t.Parallel()

	rc := markCurrentTriggerSegments(RenderedContext{
		{
			MessageID:       "delayed",
			ReceivedAtMs:    100,
			LastEventCursor: 10,
			Content:         []RenderedContentPiece{{Type: "text", Text: "delayed exact trigger"}},
		},
		{
			MessageID:       "higher",
			ReceivedAtMs:    300,
			LastEventCursor: 30,
			Content:         []RenderedContentPiece{{Type: "text", Text: "unselected higher-cursor message"}},
		},
	},
		newDiscussCurrentTriggerSelection([]DiscussEventDelivery{{EventID: "event-10", EventCursor: 10}}),
	)
	composed := ComposeContext(rc, nil)
	if composed == nil || len(composed.Messages) != 2 {
		t.Fatalf("composed messages = %#v, want separate selected low10 and unselected high30", composed)
	}
	messages := composed.Messages
	prompt := discussACPFullContextPrompt(messages, 20, "")

	if !strings.Contains(prompt, "[user; current-trigger]\ndelayed exact trigger") {
		t.Fatalf("ACP prompt did not mark the exact delayed trigger: %q", prompt)
	}
	if strings.Contains(prompt, "[user; current-trigger]\nunselected higher-cursor message") {
		t.Fatalf("ACP prompt marked an unselected higher-cursor message as current: %q", prompt)
	}
	if !messages[0].CurrentTriggerEvaluated || !messages[1].CurrentTriggerEvaluated ||
		!messages[0].CurrentTrigger || messages[1].CurrentTrigger {
		t.Fatalf("exact trigger selection = %#v, want evaluated low10 selected and high30 unselected", messages)
	}
}

func TestLegacyCurrentTriggerFallbackRemainsUnevaluatedWithoutDeliveries(t *testing.T) {
	t.Parallel()

	messages := []ContextMessage{{
		Role:                      "user",
		Content:                   "legacy cursor trigger",
		LatestExternalEventCursor: 30,
	}}

	if messages[0].CurrentTriggerEvaluated {
		t.Fatal("nil-delivery path disabled the legacy cursor fallback")
	}
	prompt := discussACPFullContextPrompt(messages, 20, "")
	if !strings.Contains(prompt, "[user; current-trigger]\nlegacy cursor trigger") {
		t.Fatalf("legacy cursor trigger was not marked current: %q", prompt)
	}
}

func TestLatestDiscussNotificationRetainsEveryEventDelivery(t *testing.T) {
	t.Parallel()

	newer := discussNotification{
		rc:         RenderedContext{{LastEventCursor: 20}},
		config:     DiscussSessionConfig{ReplyTarget: "newer"},
		deliveries: []DiscussEventDelivery{{EventID: "event-20", EventCursor: 20}},
	}
	older := discussNotification{
		rc:         RenderedContext{{LastEventCursor: 10}},
		config:     DiscussSessionConfig{ReplyTarget: "older"},
		deliveries: []DiscussEventDelivery{{EventID: "event-10", EventCursor: 10}},
	}

	got, _ := newerDiscussNotification(older, newer)
	if len(got.deliveries) != 2 {
		t.Fatalf("merged deliveries = %#v, want both provider events", got.deliveries)
	}
	seen := make(map[string]bool, len(got.deliveries))
	for _, delivery := range got.deliveries {
		seen[delivery.EventID] = true
	}
	if !seen["event-10"] || !seen["event-20"] {
		t.Fatalf("merged deliveries = %#v, want event-10 and event-20", got.deliveries)
	}
}

func TestCompleteDiscussEventDeliveriesCompletesEveryLease(t *testing.T) {
	t.Parallel()

	now := time.Now()
	firstQueries := &leaseQueries{now: now, historyReady: true}
	secondQueries := &leaseQueries{now: now, historyReady: true}
	first, err := newTestLeaseStore(firstQueries, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || first == nil {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	second, err := newTestLeaseStore(secondQueries, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"44444444-4444-4444-4444-444444444444",
	)
	if err != nil || second == nil {
		t.Fatalf("second claim = %#v, %v", second, err)
	}

	driver := NewDiscussDriver(DiscussDriverDeps{})
	completeDiscussEventDeliveries(context.Background(), []DiscussEventDelivery{
		{EventID: "event-10", EventCursor: 10, Lease: first},
		{EventID: "event-20", EventCursor: 20, Lease: second},
	}, driver.logger)

	firstQueries.mu.Lock()
	firstCompleted := firstQueries.completed
	firstQueries.mu.Unlock()
	secondQueries.mu.Lock()
	secondCompleted := secondQueries.completed
	secondQueries.mu.Unlock()
	if !firstCompleted || !secondCompleted {
		t.Fatalf("completed leases = first:%t second:%t, want both", firstCompleted, secondCompleted)
	}
}

func TestEnqueueDiscussNotificationKeepsArrivingConfigOnEqualCursorOverflow(t *testing.T) {
	t.Parallel()

	ch := make(chan discussNotification, 1)
	ch <- discussNotification{
		rc:     RenderedContext{{LastEventCursor: 20}},
		config: DiscussSessionConfig{ReplyTarget: "queued"},
	}

	enqueueDiscussNotification(ch, discussNotification{
		rc:     RenderedContext{{LastEventCursor: 20}},
		config: DiscussSessionConfig{ReplyTarget: "arriving"},
	})

	got := <-ch
	if got.config.ReplyTarget != "arriving" {
		t.Fatalf("notification config = %#v, want arriving config on equal cursor", got.config)
	}
}

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
