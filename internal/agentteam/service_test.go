package agentteam

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

type stubResolveStore struct {
	memStore
	issues map[string]Issue
}

func (s *stubResolveStore) GetIssue(_ context.Context, id string) (Issue, error) {
	for _, i := range s.issues {
		if i.ID == id {
			return i, nil
		}
	}
	return Issue{}, ErrNotFound
}

func (s *stubResolveStore) GetIssueByNumber(_ context.Context, teamID string, number int32) (Issue, error) {
	for _, i := range s.issues {
		if i.TeamID == teamID && i.Number == number {
			return i, nil
		}
	}
	return Issue{}, ErrNotFound
}

func TestResolveIssueRefAcceptsNumberHashAndUUID(t *testing.T) {
	t.Parallel()
	want := Issue{ID: "uuid-3", TeamID: "team-a", Number: 3}
	store := &stubResolveStore{issues: map[string]Issue{"k": want}}
	svc := NewService(slog.Default(), store)
	_ = strconv.Itoa(int(want.Number))

	t.Run("bare number", func(t *testing.T) {
		t.Parallel()
		issue, err := svc.ResolveIssueRef(context.Background(), "team-a", "3")
		if err != nil || issue.ID != "uuid-3" {
			t.Fatalf("bare number: got id=%q err=%v", issue.ID, err)
		}
	})
	t.Run("hash prefix", func(t *testing.T) {
		t.Parallel()
		issue, err := svc.ResolveIssueRef(context.Background(), "team-a", "#3")
		if err != nil || issue.ID != "uuid-3" {
			t.Fatalf("hash form: got id=%q err=%v", issue.ID, err)
		}
	})
	t.Run("uuid", func(t *testing.T) {
		t.Parallel()
		issue, err := svc.ResolveIssueRef(context.Background(), "team-a", "uuid-3")
		if err != nil || issue.ID != "uuid-3" {
			t.Fatalf("uuid form: got id=%q err=%v", issue.ID, err)
		}
	})
	t.Run("number without team rejected", func(t *testing.T) {
		t.Parallel()
		_, err := svc.ResolveIssueRef(context.Background(), "", "5")
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})
	t.Run("empty rejected", func(t *testing.T) {
		t.Parallel()
		_, err := svc.ResolveIssueRef(context.Background(), "team-a", "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	// Keep strings/json package imports used.
	_ = strings.TrimSpace("")
	_ = json.RawMessage(nil)
}

func TestValidateSharedDirName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"slash", "foo/bar", true},
		{"absolute", "/foo", true},
		{"backslash", "foo\\bar", true},
		{"dotdot", "..", true},
		{"hidden", "..hidden", true},
		{"data reserved", "data", true},
		{"team reserved", "team", true},
		{"tmp reserved", "TMP", true},
		{"valid simple", "alpha", false},
		{"valid hyphen", "project-x", false},
		{"valid dot", "v1.2", false},
		{"valid underscore", "_internal", false},
		{"too long", strings.Repeat("a", 65), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSharedDirName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error for %q, got %v", tc.input, err)
			}
		})
	}
}

func TestParseMentions(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect []Mention
	}{
		{
			name:  "plain text",
			input: "hello world",
		},
		{
			name:   "single bare mention",
			input:  "@Alice please review",
			expect: []Mention{{Name: "Alice"}},
		},
		{
			name:   "quoted name with space",
			input:  `Hey @"Frontend Bot" please look`,
			expect: []Mention{{Name: "Frontend Bot"}},
		},
		{
			name:   "multiple mentions deduped case insensitive",
			input:  "cc @Bob and @bob again, plus @Alice",
			expect: []Mention{{Name: "Bob"}, {Name: "Alice"}},
		},
		{
			name:  "email is not a mention",
			input: "ping me at me@example.com",
		},
		{
			name:   "mention after punctuation",
			input:  "Result: @Alice — please verify",
			expect: []Mention{{Name: "Alice"}},
		},
		{
			// Punctuation immediately before `@` is fine — only word
			// chars (letters / digits / underscores) form a boundary
			// that suppresses the mention. `(@Alice)` is a mention.
			name:   "parenthesized mention",
			input:  "ping (@Alice) please",
			expect: []Mention{{Name: "Alice"}},
		},
		{
			// Markdown-link decoration like `[@Alice](url)` is
			// lenient-parsed: the `@Alice` label resolves as a normal
			// mention. The URI part is decorative and ignored — bots
			// should NOT bother generating it (see the team prompt).
			name:   "markdown link label resolves leniently",
			input:  "see [@Alice](mention://bot/11111111-1111-1111-1111-111111111111)",
			expect: []Mention{{Name: "Alice"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMentions(tc.input)
			if len(got) != len(tc.expect) {
				t.Fatalf("len: got %d, want %d (%v)", len(got), len(tc.expect), got)
			}
			for i, m := range got {
				if m != tc.expect[i] {
					t.Fatalf("[%d]: got %#v, want %#v", i, m, tc.expect[i])
				}
			}
		})
	}
}

func TestMergeAndReadCommentMetadata(t *testing.T) {
	t.Parallel()

	// Empty input + empty source → "{}"
	out, err := MergeCommentMetadata(nil, "")
	if err != nil {
		t.Fatalf("merge nil/empty: %v", err)
	}
	if string(out) != "{}" {
		t.Fatalf("expected {} for empty merge, got %s", out)
	}
	if got := ReadSourceSessionFromMetadata(out); got != "" {
		t.Fatalf("expected empty source from {} metadata, got %q", got)
	}

	// Adds the key when source is set.
	merged, err := MergeCommentMetadata(nil, "sess-1")
	if err != nil {
		t.Fatalf("merge with source: %v", err)
	}
	if got := ReadSourceSessionFromMetadata(merged); got != "sess-1" {
		t.Fatalf("expected sess-1, got %q", got)
	}

	// Preserves unrelated caller-supplied keys.
	merged, err = MergeCommentMetadata([]byte(`{"reason":"escalate"}`), "sess-2")
	if err != nil {
		t.Fatalf("merge preserve: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(merged, &obj); err != nil {
		t.Fatalf("re-parse merged: %v", err)
	}
	if obj[CommentMetadataKeySourceSession] != "sess-2" {
		t.Fatalf("missing source after merge: %v", obj)
	}
	if obj["reason"] != "escalate" {
		t.Fatalf("dropped caller metadata: %v", obj)
	}

	// Garbage caller metadata is reset rather than failing the write.
	merged, err = MergeCommentMetadata([]byte("not json"), "sess-3")
	if err != nil {
		t.Fatalf("merge garbage: %v", err)
	}
	if got := ReadSourceSessionFromMetadata(merged); got != "sess-3" {
		t.Fatalf("expected sess-3 from garbage→reset path, got %q", got)
	}

	// Source key is trimmed.
	if got := ReadSourceSessionFromMetadata([]byte(`{"source_session_id":"   "}`)); got != "" {
		t.Fatalf("expected empty after trim, got %q", got)
	}
}

func TestMatchMember(t *testing.T) {
	roster := []Member{
		{ID: "m1", MemberType: MemberBot, BotID: "bot-1", DisplayName: "Alice"},
		{ID: "m2", MemberType: MemberBot, BotID: "bot-2", DisplayName: "Bob"},
		{ID: "m3", MemberType: MemberBot, BotID: "bot-3", DisplayName: "Alice"}, // collision
		{ID: "m4", MemberType: MemberUser, UserID: "user-1", DisplayName: "Charlie"},
	}
	cases := []struct {
		name      string
		input     string
		expectID  string
		expectHit bool
	}{
		{"exact match user", "Charlie", "m4", true},
		{"case insensitive", "BOB", "m2", true},
		{"trim", "  bob  ", "m2", true},
		{"ambiguous", "Alice", "", false},
		{"unknown", "DoesNotExist", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := MatchMember(tc.input, roster)
			if ok != tc.expectHit {
				t.Fatalf("hit: got %v want %v", ok, tc.expectHit)
			}
			if ok && m.ID != tc.expectID {
				t.Fatalf("id: got %s want %s", m.ID, tc.expectID)
			}
		})
	}
}
