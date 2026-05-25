package agentteam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// memStore is a minimal in-memory Store implementation used to exercise the
// dispatcher's anti-loop and handoff-return behaviour. Only the methods used
// by the dispatcher are implemented; all other methods return ErrNotFound.
type memStore struct {
	mu       sync.Mutex
	idSeed   atomic.Int64
	handoffs map[string]Handoff
	roster   []Member
}

func newMemStore() *memStore {
	return &memStore{handoffs: make(map[string]Handoff)}
}

func (s *memStore) setRoster(members []Member) {
	s.mu.Lock()
	s.roster = members
	s.mu.Unlock()
}

func (s *memStore) nextID() string {
	return fmt.Sprintf("ho-%d", s.idSeed.Add(1))
}

func (s *memStore) CreateHandoff(_ context.Context, in CreateHandoffInput) (Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := Handoff{
		ID:               s.nextID(),
		TeamID:           in.TeamID,
		IssueID:          in.IssueID,
		FromActorType:    in.FromActorType,
		FromBotID:        in.FromBotID,
		FromUserID:       in.FromUserID,
		ToBotID:          in.ToBotID,
		TriggerCommentID: in.TriggerCommentID,
		SourceSessionID:  in.SourceSessionID,
		TargetSessionID:  in.TargetSessionID,
		Status:           in.Status,
		Metadata:         in.Metadata,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if h.Status == "" {
		h.Status = HandoffPending
	}
	s.handoffs[h.ID] = h
	return h, nil
}

func (s *memStore) ListPendingHandoffsToBotForIssue(_ context.Context, botID, issueID string) ([]Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Handoff
	for _, h := range s.handoffs {
		if h.ToBotID != botID || h.IssueID != issueID {
			continue
		}
		if h.Status == HandoffPending || h.Status == HandoffDispatched || h.Status == HandoffRunning {
			out = append(out, h)
		}
	}
	return out, nil
}

func (s *memStore) CompleteHandoff(_ context.Context, id, resultCommentID string) (Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.handoffs[id]
	if !ok {
		return Handoff{}, ErrNotFound
	}
	h.Status = HandoffCompleted
	h.ResultCommentID = resultCommentID
	h.HasCompletedAt = true
	h.CompletedAt = time.Now()
	s.handoffs[id] = h
	return h, nil
}

func (s *memStore) SetHandoffReturn(_ context.Context, id, returnHandoffID string) (Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.handoffs[id]
	if !ok {
		return Handoff{}, ErrNotFound
	}
	h.ReturnHandoffID = returnHandoffID
	h.Status = HandoffReturned
	s.handoffs[id] = h
	return h, nil
}

func (s *memStore) FailHandoff(_ context.Context, id, reason string) (Handoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.handoffs[id]
	if !ok {
		return Handoff{}, ErrNotFound
	}
	h.Status = HandoffFailed
	h.FailureReason = reason
	s.handoffs[id] = h
	return h, nil
}

// All other Store methods are stubs that return ErrNotFound so the dispatcher
// can exercise the implemented hot path.
func (*memStore) CreateTeam(context.Context, CreateTeamInput) (Team, error) {
	return Team{}, ErrNotFound
}
func (*memStore) GetTeam(context.Context, string) (Team, error) { return Team{}, ErrNotFound }
func (*memStore) GetTeamForOwner(context.Context, string, string) (Team, error) {
	return Team{}, ErrNotFound
}
func (*memStore) ListTeamsByOwner(context.Context, string) ([]Team, error)    { return nil, nil }
func (*memStore) ListAllTeamsByOwner(context.Context, string) ([]Team, error) { return nil, nil }
func (*memStore) ListAllTeams(context.Context) ([]Team, error)                { return nil, nil }
func (*memStore) ListTeamsForBot(context.Context, string) ([]Team, error)     { return nil, nil }
func (*memStore) UpdateTeam(context.Context, string, UpdateTeamInput) (Team, error) {
	return Team{}, ErrNotFound
}
func (*memStore) ArchiveTeam(context.Context, string) (Team, error) { return Team{}, ErrNotFound }
func (*memStore) DeleteTeam(context.Context, string) error          { return nil }
func (*memStore) AddMember(context.Context, CreateMemberInput) (Member, error) {
	return Member{}, ErrNotFound
}
func (*memStore) GetMember(context.Context, string) (Member, error) { return Member{}, ErrNotFound }
func (*memStore) GetMemberByBot(context.Context, string, string) (Member, error) {
	return Member{}, ErrNotFound
}

func (*memStore) GetMemberByUser(context.Context, string, string) (Member, error) {
	return Member{}, ErrNotFound
}

func (s *memStore) ListMembers(_ context.Context, _ string) ([]Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Member, len(s.roster))
	copy(out, s.roster)
	return out, nil
}

func (*memStore) UpdateMember(context.Context, string, UpdateMemberInput) (Member, error) {
	return Member{}, ErrNotFound
}
func (*memStore) DeleteMember(context.Context, string) error { return nil }
func (*memStore) CreateIssue(context.Context, CreateIssueInput) (Issue, error) {
	return Issue{}, ErrNotFound
}
func (*memStore) GetIssue(context.Context, string) (Issue, error) { return Issue{}, ErrNotFound }

func (*memStore) GetIssueByNumber(context.Context, string, int32) (Issue, error) {
	return Issue{}, ErrNotFound
}

func (*memStore) GetIssueInTeam(context.Context, string, string) (Issue, error) {
	return Issue{}, ErrNotFound
}
func (*memStore) ListIssuesByTeam(context.Context, string) ([]Issue, error)     { return nil, nil }
func (*memStore) ListOpenIssuesByTeam(context.Context, string) ([]Issue, error) { return nil, nil }
func (*memStore) ListIssuesForOwner(context.Context, string) ([]Issue, error)   { return nil, nil }
func (*memStore) UpdateIssue(context.Context, string, UpdateIssueInput) (Issue, error) {
	return Issue{}, ErrNotFound
}

func (*memStore) SetIssueAssignee(context.Context, string, AssignIssueInput) (Issue, error) {
	return Issue{}, ErrNotFound
}
func (*memStore) DeleteIssue(context.Context, string) error { return nil }
func (*memStore) CreateComment(context.Context, CreateCommentInput) (Comment, error) {
	return Comment{}, ErrNotFound
}

func (*memStore) GetComment(context.Context, string) (Comment, error)     { return Comment{}, ErrNotFound }
func (*memStore) ListComments(context.Context, string) ([]Comment, error) { return nil, nil }
func (*memStore) TouchIssueAfterComment(context.Context, string) error    { return nil }
func (*memStore) DeleteComment(context.Context, string) error             { return nil }
func (*memStore) GetHandoff(context.Context, string) (Handoff, error) {
	return Handoff{}, ErrNotFound
}

func (*memStore) ListPendingHandoffsToBot(context.Context, string) ([]Handoff, error) {
	return nil, nil
}

func (*memStore) ListPendingReturnsForBotInIssue(context.Context, string, string) ([]Handoff, error) {
	return nil, nil
}

func (*memStore) ListHandoffsByIssue(context.Context, string) ([]Handoff, error) {
	return nil, nil
}

func (*memStore) MarkHandoffDispatched(context.Context, string, string) (Handoff, error) {
	return Handoff{}, ErrNotFound
}

func (*memStore) MarkHandoffRunning(context.Context, string, string) (Handoff, error) {
	return Handoff{}, ErrNotFound
}

func (*memStore) CancelHandoff(context.Context, string, string) (Handoff, error) {
	return Handoff{}, ErrNotFound
}

func (*memStore) AcquireFileLock(context.Context, AcquireLockInput) (FileLock, error) {
	return FileLock{}, ErrNotFound
}

func (*memStore) GetFileLockByID(context.Context, string) (FileLock, error) {
	return FileLock{}, ErrNotFound
}

func (*memStore) GetFileLock(context.Context, string, string, LockScope) (FileLock, error) {
	return FileLock{}, ErrNotFound
}

func (*memStore) ListFileLocks(context.Context, string) ([]FileLock, error) { return nil, nil }

func (*memStore) ListActiveFileLocks(context.Context, string) ([]FileLock, error) { return nil, nil }

func (*memStore) RefreshFileLock(context.Context, string, time.Time) (FileLock, error) {
	return FileLock{}, ErrNotFound
}
func (*memStore) ReleaseFileLock(context.Context, string) error { return nil }
func (*memStore) ReleaseExpiredFileLocks(context.Context) error { return nil }

// Compile-time check: memStore satisfies the Store interface.
var _ Store = (*memStore)(nil)

func newServiceWithStore(t *testing.T, store Store) *Service {
	t.Helper()
	return NewService(slog.Default(), store)
}

// seedTeamRoster registers a small roster on the store so that the
// dispatcher's name → bot resolution has something to match against.
func seedTeamRoster(store *memStore) {
	store.setRoster([]Member{
		{ID: "m1", TeamID: "team-1", MemberType: MemberBot, BotID: "bot-1", DisplayName: "Leader"},
		{ID: "m2", TeamID: "team-1", MemberType: MemberBot, BotID: "bot-2", DisplayName: "Worker"},
		{ID: "m3", TeamID: "team-1", MemberType: MemberUser, UserID: "user-1", DisplayName: "Owner"},
	})
}

func TestDispatcherCreateHandoffOnMention(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	// No trigger configured: handoffs should still be persisted.
	comment := Comment{
		ID:           "cmt-1",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "Hi @Worker please look",
	}
	if err := d.HandleComment(context.Background(), comment); err != nil {
		t.Fatalf("HandleComment: %v", err)
	}

	handoffs, err := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(handoffs) != 1 {
		t.Fatalf("expected 1 handoff, got %d", len(handoffs))
	}
	if handoffs[0].FromActorType != ActorUser {
		t.Fatalf("from actor type = %s, want user", handoffs[0].FromActorType)
	}
}

func TestDispatcherSelfMentionSkipped(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	comment := Comment{
		ID:          "cmt-1",
		IssueID:     "issue-1",
		TeamID:      "team-1",
		AuthorType:  ActorBot,
		AuthorBotID: "bot-1",
		Content:     "Note @Leader — self-trigger should not happen",
	}
	if err := d.HandleComment(context.Background(), comment); err != nil {
		t.Fatalf("HandleComment: %v", err)
	}
	handoffs, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-1", "issue-1")
	if len(handoffs) != 0 {
		t.Fatalf("expected 0 handoffs (self-mention skipped), got %d", len(handoffs))
	}
}

func TestDispatcherUserMentionIsNotificationOnly(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	comment := Comment{
		ID:          "cmt-1",
		IssueID:     "issue-1",
		TeamID:      "team-1",
		AuthorType:  ActorBot,
		AuthorBotID: "bot-1",
		Content:     "@Owner please review",
	}
	if err := d.HandleComment(context.Background(), comment); err != nil {
		t.Fatalf("HandleComment: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.handoffs) != 0 {
		t.Fatalf("expected no handoff for user mention, got %d", len(store.handoffs))
	}
}

func TestDispatcherUnknownNameIgnored(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	comment := Comment{
		ID:           "cmt-1",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "@SomeoneNotInTeam please help",
	}
	if err := d.HandleComment(context.Background(), comment); err != nil {
		t.Fatalf("HandleComment: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.handoffs) != 0 {
		t.Fatalf("expected no handoff for unknown @name, got %d", len(store.handoffs))
	}
}

func TestDispatcherDuplicateMentionDedup(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	comment1 := Comment{
		ID:           "cmt-1",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "@Worker",
	}
	if err := d.HandleComment(context.Background(), comment1); err != nil {
		t.Fatalf("HandleComment 1: %v", err)
	}
	comment2 := Comment{
		ID:           "cmt-2",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "@Worker again",
	}
	if err := d.HandleComment(context.Background(), comment2); err != nil {
		t.Fatalf("HandleComment 2: %v", err)
	}

	handoffs, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if len(handoffs) != 1 {
		t.Fatalf("expected 1 handoff (dedup), got %d", len(handoffs))
	}
}

func TestDispatcherReturnOnBotComment(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	originalCmt := Comment{
		ID:              "cmt-1",
		IssueID:         "issue-1",
		TeamID:          "team-1",
		AuthorType:      ActorBot,
		AuthorBotID:     "bot-1",
		Content:         "@Worker please implement",
		SourceSessionID: "sess-leader-1",
	}
	if err := d.HandleComment(context.Background(), originalCmt); err != nil {
		t.Fatalf("HandleComment 1: %v", err)
	}
	resultCmt := Comment{
		ID:              "cmt-2",
		IssueID:         "issue-1",
		TeamID:          "team-1",
		AuthorType:      ActorBot,
		AuthorBotID:     "bot-2",
		Content:         "Done, results in /team/output.md",
		ParentCommentID: "cmt-1",
		SourceSessionID: "sess-worker-1",
	}
	if err := d.HandleComment(context.Background(), resultCmt); err != nil {
		t.Fatalf("HandleComment 2: %v", err)
	}

	pendingForBot1, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-1", "issue-1")
	if len(pendingForBot1) != 1 {
		t.Fatalf("expected 1 return handoff queued for bot-1, got %d", len(pendingForBot1))
	}
	if pendingForBot1[0].FromActorType != ActorSystem {
		t.Fatalf("return from_actor = %s, want system", pendingForBot1[0].FromActorType)
	}
	// The whole point: the return targets the delegator's originating session.
	if pendingForBot1[0].TargetSessionID != "sess-leader-1" {
		t.Fatalf("return target_session = %q, want %q", pendingForBot1[0].TargetSessionID, "sess-leader-1")
	}

	originalForBot2, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if len(originalForBot2) != 0 {
		t.Fatalf("expected bot-2 pending handoff to be completed, got %d still pending", len(originalForBot2))
	}
}

// TestDispatcherPerSessionRoutingSameIssue is the test that pins the
// new design: two mentions to the same bot on the same issue but from
// DIFFERENT sessions of the delegator must each return to their own
// originating session.
func TestDispatcherPerSessionRoutingSameIssue(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	// Bot-1 in session S1 mentions Worker about task X.
	mentionFromS1 := Comment{
		ID:              "cmt-s1",
		IssueID:         "issue-1",
		TeamID:          "team-1",
		AuthorType:      ActorBot,
		AuthorBotID:     "bot-1",
		Content:         "@Worker handle X",
		SourceSessionID: "sess-S1",
	}
	if err := d.HandleComment(context.Background(), mentionFromS1); err != nil {
		t.Fatalf("S1 mention: %v", err)
	}

	// Bot-1 in session S2 (different chat) mentions Worker about task Y.
	mentionFromS2 := Comment{
		ID:              "cmt-s2",
		IssueID:         "issue-1",
		TeamID:          "team-1",
		AuthorType:      ActorBot,
		AuthorBotID:     "bot-1",
		Content:         "@Worker also handle Y",
		SourceSessionID: "sess-S2",
	}
	if err := d.HandleComment(context.Background(), mentionFromS2); err != nil {
		t.Fatalf("S2 mention: %v", err)
	}

	// Two distinct handoffs are queued for bot-2.
	pendingForWorker, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if len(pendingForWorker) != 2 {
		t.Fatalf("expected 2 pending handoffs to worker, got %d", len(pendingForWorker))
	}

	// Worker replies to the S1 mention via threading.
	replyToS1 := Comment{
		ID:              "cmt-s1-reply",
		IssueID:         "issue-1",
		TeamID:          "team-1",
		AuthorType:      ActorBot,
		AuthorBotID:     "bot-2",
		Content:         "X done",
		ParentCommentID: "cmt-s1",
		SourceSessionID: "sess-worker-issue-1",
	}
	if err := d.HandleComment(context.Background(), replyToS1); err != nil {
		t.Fatalf("S1 reply: %v", err)
	}

	// Only ONE return queued for bot-1, targeting S1.
	pendingForLeader, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-1", "issue-1")
	if len(pendingForLeader) != 1 {
		t.Fatalf("expected 1 return for bot-1 after S1 reply, got %d", len(pendingForLeader))
	}
	if pendingForLeader[0].TargetSessionID != "sess-S1" {
		t.Fatalf("S1 return target = %q, want sess-S1", pendingForLeader[0].TargetSessionID)
	}

	// The S2 mention is still pending — worker hasn't replied to it yet.
	pendingForWorker, _ = store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if len(pendingForWorker) != 1 {
		t.Fatalf("expected 1 worker handoff still pending after S1 reply, got %d", len(pendingForWorker))
	}
}

// TestDispatcherTopLevelReplyClosesAll covers the fallback path: a bot
// posting a top-level result comment (no parent thread) means "I'm done
// with everything you asked me on this issue" and closes all pending
// handoffs, queuing one return per source session.
func TestDispatcherTopLevelReplyClosesAll(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	// Two separate sessions of bot-1 ask bot-2 to do things.
	for _, m := range []Comment{
		{
			ID: "cmt-s1", IssueID: "issue-1", TeamID: "team-1",
			AuthorType: ActorBot, AuthorBotID: "bot-1",
			Content: "@Worker do X", SourceSessionID: "sess-S1",
		},
		{
			ID: "cmt-s2", IssueID: "issue-1", TeamID: "team-1",
			AuthorType: ActorBot, AuthorBotID: "bot-1",
			Content: "@Worker do Y", SourceSessionID: "sess-S2",
		},
	} {
		if err := d.HandleComment(context.Background(), m); err != nil {
			t.Fatalf("seed mention: %v", err)
		}
	}

	// Worker posts a top-level wrap-up comment (no parent_comment_id).
	topLevel := Comment{
		ID:          "cmt-wrap",
		IssueID:     "issue-1",
		TeamID:      "team-1",
		AuthorType:  ActorBot,
		AuthorBotID: "bot-2",
		Content:     "All done with X and Y",
	}
	if err := d.HandleComment(context.Background(), topLevel); err != nil {
		t.Fatalf("top-level reply: %v", err)
	}

	pendingForWorker, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if len(pendingForWorker) != 0 {
		t.Fatalf("expected all worker handoffs closed, got %d still pending", len(pendingForWorker))
	}

	// Per-source-session routing: a top-level wrap-up that closes
	// handoffs from two different originating sessions must queue ONE
	// return per session — S1 ⇢ sess-S1, S2 ⇢ sess-S2 — so each chat
	// gets its own reply.
	pendingForLeader, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-1", "issue-1")
	if len(pendingForLeader) != 2 {
		t.Fatalf("expected 2 returns queued (one per source session), got %d", len(pendingForLeader))
	}
	got := map[string]bool{}
	for _, h := range pendingForLeader {
		got[h.TargetSessionID] = true
	}
	for _, want := range []string{"sess-S1", "sess-S2"} {
		if !got[want] {
			t.Fatalf("missing return for %s, returns = %+v", want, pendingForLeader)
		}
	}
}

// TestDispatcherUnknownParentLeavesHandoffsOpen ensures a bot replying
// in some unrelated thread (parent_comment_id that does not match any
// pending handoff's trigger comment) does NOT accidentally close any
// pending delegation. This is the strict policy guaranteeing that a
// stray threaded comment cannot route returns to the wrong session.
func TestDispatcherUnknownParentLeavesHandoffsOpen(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	if err := d.HandleComment(context.Background(), Comment{
		ID: "cmt-mention", IssueID: "issue-1", TeamID: "team-1",
		AuthorType: ActorBot, AuthorBotID: "bot-1",
		Content: "@Worker do X", SourceSessionID: "sess-S1",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Worker comments threaded under an *unrelated* comment.
	if err := d.HandleComment(context.Background(), Comment{
		ID: "cmt-reply", IssueID: "issue-1", TeamID: "team-1",
		AuthorType: ActorBot, AuthorBotID: "bot-2",
		Content:         "stray note",
		ParentCommentID: "some-other-comment-id",
	}); err != nil {
		t.Fatalf("stray reply: %v", err)
	}

	pendingForWorker, _ := store.ListPendingHandoffsToBotForIssue(context.Background(), "bot-2", "issue-1")
	if len(pendingForWorker) != 1 {
		t.Fatalf("expected worker handoff still open, got %d pending", len(pendingForWorker))
	}
}

// TestDispatcherReturnTargetEmptyWhenUserOrigin: a user-originated
// mention (no source session) produces no return at all, and crucially
// no handoff with target_session_id set to a stale value.
func TestDispatcherReturnTargetEmptyWhenUserOrigin(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	if err := d.HandleComment(context.Background(), Comment{
		ID: "cmt-mention", IssueID: "issue-1", TeamID: "team-1",
		AuthorType: ActorUser, AuthorUserID: "user-1",
		Content: "@Worker do X",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := d.HandleComment(context.Background(), Comment{
		ID: "cmt-reply", IssueID: "issue-1", TeamID: "team-1",
		AuthorType: ActorBot, AuthorBotID: "bot-2",
		Content: "done", ParentCommentID: "cmt-mention",
	}); err != nil {
		t.Fatalf("reply: %v", err)
	}

	for _, h := range store.handoffs {
		if h.FromActorType == ActorSystem {
			t.Fatalf("unexpected return handoff for user-origin chain: %+v", h)
		}
	}
}

func TestDispatcherNoReturnIfUserOrigin(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	originalCmt := Comment{
		ID:           "cmt-1",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "@Worker please implement",
	}
	if err := d.HandleComment(context.Background(), originalCmt); err != nil {
		t.Fatalf("HandleComment 1: %v", err)
	}
	resultCmt := Comment{
		ID:          "cmt-2",
		IssueID:     "issue-1",
		TeamID:      "team-1",
		AuthorType:  ActorBot,
		AuthorBotID: "bot-2",
		Content:     "Done",
	}
	if err := d.HandleComment(context.Background(), resultCmt); err != nil {
		t.Fatalf("HandleComment 2: %v", err)
	}

	// User-originated delegations should NOT produce a return handoff.
	for _, h := range store.handoffs {
		if h.FromActorType == ActorSystem {
			t.Fatalf("unexpected return handoff queued for user-originated delegation: %+v", h)
		}
	}
}

func TestDispatcherTriggerInvokedOnce(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	var calls atomic.Int32
	done := make(chan struct{}, 1)
	d.SetTrigger(func(_ context.Context, _ Handoff, _ Comment) error {
		calls.Add(1)
		done <- struct{}{}
		return nil
	})

	comment := Comment{
		ID:           "cmt-1",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "@Worker please look",
	}
	if err := d.HandleComment(context.Background(), comment); err != nil {
		t.Fatalf("HandleComment: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("trigger did not fire within 2s")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("trigger calls = %d, want 1", got)
	}
}

func TestDispatcherFailHandoffWhenTriggerFails(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	seedTeamRoster(store)
	svc := newServiceWithStore(t, store)
	d := svc.Dispatcher()

	done := make(chan struct{}, 1)
	d.SetTrigger(func(_ context.Context, _ Handoff, _ Comment) error {
		done <- struct{}{}
		return errors.New("boom")
	})

	comment := Comment{
		ID:           "cmt-1",
		IssueID:      "issue-1",
		TeamID:       "team-1",
		AuthorType:   ActorUser,
		AuthorUserID: "user-1",
		Content:      "@Worker please look",
	}
	if err := d.HandleComment(context.Background(), comment); err != nil {
		t.Fatalf("HandleComment: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("trigger did not fire within 2s")
	}
	// Allow the goroutine to record the failure.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		var failed bool
		for _, h := range store.handoffs {
			if h.Status == HandoffFailed {
				failed = true
			}
		}
		store.mu.Unlock()
		if failed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("handoff was not marked failed after trigger error")
}
