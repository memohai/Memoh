package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"testing"
	"time"

	messagepkg "github.com/memohai/memoh/internal/message"
)

// stubMessageService fakes the DB ordering (DESC newest-first) that
// ListBeforeBySession / ListLatestBySession return in production, so the
// handler's reverse logic is exercised against the real wire shape rather than
// a hand-constructed ASC slice. It embeds the interface so the other (unused)
// methods satisfy the interface without boilerplate; calling them panics.
type stubMessageService struct {
	messagepkg.Service
	bySession map[string][]messagepkg.Message
}

func (s *stubMessageService) latest(sid string, limit int) []messagepkg.Message {
	all := s.bySession[sid]
	desc := make([]messagepkg.Message, len(all))
	copy(desc, all)
	sort.Slice(desc, func(i, j int) bool {
		if desc[i].CreatedAt.Equal(desc[j].CreatedAt) {
			return desc[i].ID > desc[j].ID
		}
		return desc[i].CreatedAt.After(desc[j].CreatedAt)
	})
	if len(desc) > limit {
		desc = desc[:limit]
	}
	return desc
}

func (s *stubMessageService) before(sid string, t time.Time, limit int) []messagepkg.Message {
	var older []messagepkg.Message
	for _, m := range s.bySession[sid] {
		if m.CreatedAt.Before(t) {
			older = append(older, m)
		}
	}
	sort.Slice(older, func(i, j int) bool { return older[i].CreatedAt.After(older[j].CreatedAt) })
	if len(older) > limit {
		older = older[:limit]
	}
	return older
}

func (s *stubMessageService) ListLatestBySession(_ context.Context, sid string, limit int32) ([]messagepkg.Message, error) {
	return s.latest(sid, int(limit)), nil
}

func (s *stubMessageService) ListBeforeBySession(_ context.Context, sid string, before time.Time, limit int32) ([]messagepkg.Message, error) {
	return s.before(sid, before, int(limit)), nil
}

func msg(role string, t time.Time) messagepkg.Message {
	return messagepkg.Message{Role: role, CreatedAt: t, Content: []byte(`{}`)}
}

// userMsg builds a visible user message — IsUITurnBoundary requires non-empty
// text (DisplayContent is the first source it checks), otherwise the row is
// treated as an invisible user ping and NOT a boundary, which would defeat the
// test.
func userMsg(t time.Time, text string) messagepkg.Message {
	return messagepkg.Message{Role: "user", CreatedAt: t, Content: []byte(`{}`), DisplayContent: text}
}

func indexedMsg(id int, role string, t time.Time) messagepkg.Message {
	return messagepkg.Message{
		ID:        fmt.Sprintf("%04d", id),
		Role:      role,
		CreatedAt: t,
		Content:   []byte(`{}`),
	}
}

// TestExtendToUITurnHead_PreservesMonotonicOrder is the regression test for the
// DESC-pagination bug: extendToUITurnHead must reverse each fetched older batch
// (ListBeforeBySession returns newest-first) before prepending, so the combined
// slice stays oldest-first — the ordering ConvertMessagesToUITurns depends on.
// Before the fix the DESC older fragment was prepended as-is, producing a
// V-shaped non-monotonic slice that split one turn into several and reordered
// messages.
func TestExtendToUITurnHead_PreservesMonotonicOrder(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	// A session whose latest page (limit 30) lands mid-turn: a user boundary,
	// then 40 assistant/tool rows forming one turn. Ask for the latest 30.
	const sessionID = "s1"
	var all []messagepkg.Message
	all = append(all, userMsg(base, "hello")) // turn boundary (oldest)
	for i := 1; i <= 40; i++ {
		all = append(all, msg("assistant", base.Add(time.Duration(i)*time.Second)))
		all = append(all, msg("tool", base.Add(time.Duration(i)*time.Second+time.Millisecond)))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 30)
	reverseMessages(latest) // mirrors the handler's latest-page branch
	before := len(latest)
	got := h.extendToUITurnHead(context.Background(), sessionID, latest)

	if len(got) <= before {
		t.Fatalf("extendToUITurnHead did not pull back the turn head: got %d, had %d", len(got), before)
	}
	if got[0].Role != "user" {
		t.Fatalf("expected the pulled-back head to be the user boundary, got role %q", got[0].Role)
	}
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.Before(got[i-1].CreatedAt) {
			t.Fatalf("non-monotonic order at index %d: older batch prepended without reversing (DESC bug)", i)
		}
	}
}

// TestExtendToUITurnHead_StopsAtBoundary asserts the loop does not over-pull
// once a real turn boundary is at messages[0]. The session has 1 user + 5
// assistant; the latest page (limit 5) returns the 5 newest (all assistant),
// so extendToUITurnHead pulls exactly the one older user row and stops — it
// must NOT keep pulling past the boundary (the DESC-direction bug pulled until
// maxRows because it misread messages[0]).
func TestExtendToUITurnHead_StopsAtBoundary(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	const sessionID = "s2"
	var all []messagepkg.Message
	all = append(all, userMsg(base, "hello"))
	for i := 1; i <= 5; i++ {
		all = append(all, msg("assistant", base.Add(time.Duration(i)*time.Second)))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 5) // 5 newest = all assistant, no boundary
	reverseMessages(latest)
	got := h.extendToUITurnHead(context.Background(), sessionID, latest)
	// Must pull back exactly the one user boundary and stop — 6 total, not more.
	if len(got) != 6 {
		t.Fatalf("expected exactly the user boundary + 5 assistant = 6, got %d (over-pulled?)", len(got))
	}
	if got[0].Role != "user" {
		t.Fatalf("expected head to be the user boundary, got role %q", got[0].Role)
	}
}

func TestEnsureUITurnHead_ExpandsSameSecondPage(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 13, 17, 26, 0, time.UTC)
	const sessionID = "same-second"
	all := []messagepkg.Message{{
		ID:             "0001",
		Role:           "user",
		CreatedAt:      base,
		Content:        []byte(`{}`),
		DisplayContent: "run many commands",
	}}
	all = append(all, indexedMsg(2, "assistant", base))
	for i := 3; i <= 32; i++ {
		role := "assistant"
		if i%2 == 1 {
			role = "tool"
		}
		all = append(all, indexedMsg(i, role, base))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 30)
	reverseMessages(latest)
	if len(latest) != 30 || latest[0].Role == "user" {
		t.Fatalf("test setup failed: latest 30 should start mid-turn, got len=%d role=%q", len(latest), latest[0].Role)
	}
	if extended := h.extendToUITurnHead(context.Background(), sessionID, latest); len(extended) != len(latest) {
		t.Fatalf("test setup failed: created_at-only backfill unexpectedly changed same-second page")
	}

	got := h.ensureUITurnHead(context.Background(), sessionID, latest, false, time.Time{}, 30)
	if len(got) != len(all) {
		t.Fatalf("expected same-second fallback to expand to full test turn, got %d want %d", len(got), len(all))
	}
	if got[0].Role != "user" {
		t.Fatalf("expected expanded page to start at user boundary, got role %q", got[0].Role)
	}
}

func TestEnsureUITurnHead_EscalatesSameSecondPagePastFirstFallback(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 12, 40, 57, 0, time.UTC)
	const sessionID = "same-second-large"
	all := []messagepkg.Message{{
		ID:             "0001",
		Role:           "user",
		CreatedAt:      base,
		Content:        []byte(`{}`),
		DisplayContent: "run a very long command-heavy task",
	}}
	for i := 2; i <= 132; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "tool"
		}
		all = append(all, indexedMsg(i, role, base))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 30)
	reverseMessages(latest)
	firstFallback := svc.latest(sessionID, 100)
	reverseMessages(firstFallback)
	if len(firstFallback) != 100 || firstFallback[0].Role == "user" {
		t.Fatalf("test setup failed: first fallback should still start mid-turn, got len=%d role=%q", len(firstFallback), firstFallback[0].Role)
	}

	got := h.ensureUITurnHead(context.Background(), sessionID, latest, false, time.Time{}, 30)
	if len(got) != len(all) {
		t.Fatalf("expected second fallback to expand to full test turn, got %d want %d", len(got), len(all))
	}
	if got[0].Role != "user" {
		t.Fatalf("expected expanded page to start at user boundary, got role %q", got[0].Role)
	}
}
