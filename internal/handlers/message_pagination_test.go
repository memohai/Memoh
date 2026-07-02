package handlers

import (
	"context"
	"log/slog"
	"sort"
	"testing"
	"time"

	messagepkg "github.com/memohai/memoh/internal/message"
)

// stubMessageService mirrors the production ordering contract of the message
// service: ListLatestBySession returns DESC (newest-first, as the DB rows come
// back), while ListBeforeBySession returns ASC (oldest-first — its converter
// reverses the DESC rows). Tests exercise the handler against this real wire
// shape rather than a hand-constructed slice. It embeds the interface so the
// other (unused) methods satisfy it without boilerplate; calling them panics.
type stubMessageService struct {
	messagepkg.Service
	bySession map[string][]messagepkg.Message
}

func (s *stubMessageService) latest(sid string, limit int) []messagepkg.Message {
	all := s.bySession[sid]
	desc := make([]messagepkg.Message, len(all))
	copy(desc, all)
	sort.Slice(desc, func(i, j int) bool { return desc[i].CreatedAt.After(desc[j].CreatedAt) })
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
	// Mirror production: the query selects the `limit` rows closest to the
	// cursor (ORDER BY created_at DESC LIMIT n), then the converter reverses
	// them to oldest-first. So truncate on the DESC side, return ASC.
	sort.Slice(older, func(i, j int) bool { return older[i].CreatedAt.After(older[j].CreatedAt) })
	if len(older) > limit {
		older = older[:limit]
	}
	reverseMessages(older)
	return older
}

func (s *stubMessageService) ListLatestBySession(_ context.Context, sid string, limit int32) ([]messagepkg.Message, error) {
	return s.latest(sid, int(limit)), nil
}

func (s *stubMessageService) ListBeforeBySession(_ context.Context, sid string, before time.Time, _ string, limit int32) ([]messagepkg.Message, error) {
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

// TestExtendToUITurnHead_PreservesMonotonicOrder is the regression test for the
// before-page double-reverse bug: ListBeforeBySession already returns
// oldest-first (ASC), so extendToUITurnHead must prepend each fetched older
// batch as-is to keep the combined slice monotonic — the ordering
// ConvertMessagesToUITurns depends on. The bug reversed the already-ASC batch a
// second time, producing a scrambled, non-monotonic slice that split one turn
// into several and duplicated turns.
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
	got := h.extendToUITurnHead(context.Background(), sessionID, "", latest)

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
// must NOT keep pulling past the boundary (the double-reverse bug mis-ordered
// the batch so messages[0] was no longer the true oldest row).
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
	got := h.extendToUITurnHead(context.Background(), sessionID, "", latest)
	// Must pull back exactly the one user boundary and stop — 6 total, not more.
	if len(got) != 6 {
		t.Fatalf("expected exactly the user boundary + 5 assistant = 6, got %d (over-pulled?)", len(got))
	}
	if got[0].Role != "user" {
		t.Fatalf("expected head to be the user boundary, got role %q", got[0].Role)
	}
}
