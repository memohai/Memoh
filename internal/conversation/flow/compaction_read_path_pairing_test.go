package flow

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/historyfrag"
)

type pairingQueries struct {
	dbstore.Queries
	uncompacted []sqlc.ListUncompactedMessagesBySessionRow
	logID       pgtype.UUID
	markedIDs   []pgtype.UUID
}

func (f *pairingQueries) ListUncompactedMessagesBySession(context.Context, pgtype.UUID) ([]sqlc.ListUncompactedMessagesBySessionRow, error) {
	return f.uncompacted, nil
}

func (*pairingQueries) ListCompactionLogsBySession(context.Context, pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	return nil, nil
}

func (f *pairingQueries) CreateCompactionLog(context.Context, sqlc.CreateCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	return sqlc.BotHistoryMessageCompact{ID: f.logID}, nil
}

func (f *pairingQueries) MarkMessagesCompacted(_ context.Context, arg sqlc.MarkMessagesCompactedParams) error {
	f.markedIDs = append([]pgtype.UUID(nil), arg.Column2...)
	return nil
}

func (*pairingQueries) CompleteCompactionLog(_ context.Context, arg sqlc.CompleteCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	return sqlc.BotHistoryMessageCompact{ID: arg.ID, Status: arg.Status, Summary: arg.Summary}, nil
}

type pairingSummarizer struct{ summary string }

func (s pairingSummarizer) RoundTrip(*http.Request) (*http.Response, error) {
	body := `{"id":"stub","object":"chat.completion","created":0,"model":"stub",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":"` + s.summary + `"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func pairingRow(t *testing.T, role, content string) sqlc.ListUncompactedMessagesBySessionRow {
	t.Helper()
	return sqlc.ListUncompactedMessagesBySessionRow{
		ID:      pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Role:    role,
		Content: []byte(content),
		Usage:   []byte(`{"outputTokens":100}`),
	}
}

// TestSelectorToReadPathPreservesOrderEndToEnd drives the real compaction
// selector over a history with a must-keep ask_user island and feeds its
// actual marked rows into replaceCompactedHistoryRecords. It pins the pair of
// invariants that live in two packages: the selector marks one contiguous
// pre-island run under one compact_id, and the read path substitutes it in
// place — content behind the island (like "mid q") must never fold in front
// of it.
func TestSelectorToReadPathPreservesOrderEndToEnd(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		pairingRow(t, "user", `"old q"`),
		pairingRow(t, "assistant", `"old a"`),
		pairingRow(t, "assistant", `[{"type":"tool-call","toolCallId":"ask-1","toolName":"ask_user","input":{"questions":[]}}]`),
		pairingRow(t, "tool", `[{"type":"tool-result","toolCallId":"ask-1","toolName":"ask_user","output":"answered"}]`),
		pairingRow(t, "user", `"mid q"`),
		pairingRow(t, "user", `"current q"`),
	}
	q := &pairingQueries{
		uncompacted: rows,
		logID:       pgtype.UUID{Bytes: uuid.New(), Valid: true},
	}
	svc := compaction.NewService(slog.New(slog.DiscardHandler), q)

	res, err := svc.RunCompactionSync(context.Background(), compaction.TriggerConfig{
		BotID:        uuid.NewString(),
		SessionID:    uuid.NewString(),
		ModelID:      "stub-model",
		ClientType:   "openai-completions",
		APIKey:       "test",
		BaseURL:      "http://stub.invalid",
		HTTPClient:   &http.Client{Transport: pairingSummarizer{summary: "condensed old exchange"}},
		TargetTokens: 200,
	})
	if err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if res.Status != compaction.StatusOK {
		t.Fatalf("status = %q, want %q", res.Status, compaction.StatusOK)
	}

	marked := make(map[pgtype.UUID]bool, len(q.markedIDs))
	for _, id := range q.markedIDs {
		marked[id] = true
	}
	if len(marked) != 2 || !marked[rows[0].ID] || !marked[rows[1].ID] {
		t.Fatalf("marked = %v, want exactly the contiguous pre-island run [old q, old a]", q.markedIDs)
	}

	compactID := uuid.UUID(q.logID.Bytes).String()
	texts := []string{`old q`, `old a`, `ask you something`, `answered`, `mid q`, `current q`}
	roles := []string{"user", "assistant", "assistant", "tool", "user", "user"}
	records := make([]historyfrag.HistoryRecord, 0, len(rows))
	for i, row := range rows {
		id := uuid.UUID(row.ID.Bytes).String()
		record := historyRecord(id, conversation.ModelMessage{
			Role:    roles[i],
			Content: conversation.NewTextContent(texts[i]),
		}, nil)
		if marked[row.ID] {
			record.CompactID = compactID
		}
		records = append(records, record)
	}

	got := replaceCompactedHistoryRecords(records, map[string]string{compactID: res.Summary}, contextfrag.Scope{})
	want := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("<summary>\n" + res.Summary + "\n</summary>")},
		{Role: "assistant", Content: conversation.NewTextContent("ask you something")},
		{Role: "tool", Content: conversation.NewTextContent("answered")},
		{Role: "user", Content: conversation.NewTextContent("mid q")},
		{Role: "user", Content: conversation.NewTextContent("current q")},
	}
	if gotMessages := historyfrag.ToModelMessages(got); !reflect.DeepEqual(gotMessages, want) {
		t.Fatalf("selector output broke read-path ordering:\ngot  %#v\nwant %#v", gotMessages, want)
	}
}
