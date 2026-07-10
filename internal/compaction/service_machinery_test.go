package compaction

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

// --- stub summarizer model (intercepts the SDK HTTP call) ---------------------

type stubModel struct {
	summary string
	calls   int
	prompt  string // decoded text of the captured request messages
}

func (s *stubModel) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		s.prompt = decodePromptMessages(body)
	}
	resp := `{"id":"stub","object":"chat.completion","created":0,"model":"stub",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":` + jsonStr(s.summary) + `},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(resp)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func decodePromptMessages(body []byte) string {
	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	var sb strings.Builder
	for _, m := range req.Messages {
		var s string
		if json.Unmarshal(m.Content, &s) == nil {
			sb.WriteString(s)
			sb.WriteByte('\n')
			continue
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(m.Content, &parts) == nil {
			for _, p := range parts {
				sb.WriteString(p.Text)
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

// --- fake Queries (only the 5 methods compaction touches) ---------------------

type fakeQueries struct {
	dbstore.Queries // embedded interface; unimplemented methods would panic if called
	uncompacted     []sqlc.ListUncompactedMessagesBySessionRow
	priorLogs       []sqlc.BotHistoryMessageCompact

	created   bool
	markedIDs []pgtype.UUID
	completed sqlc.CompleteCompactionLogParams
}

func (f *fakeQueries) CreateCompactionLog(_ context.Context, _ sqlc.CreateCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	f.created = true
	return sqlc.BotHistoryMessageCompact{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}, nil
}

func (f *fakeQueries) ListUncompactedMessagesBySession(_ context.Context, _ pgtype.UUID) ([]sqlc.ListUncompactedMessagesBySessionRow, error) {
	return f.uncompacted, nil
}

func (f *fakeQueries) ListCompactionLogsBySession(_ context.Context, _ pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	return f.priorLogs, nil
}

func (f *fakeQueries) MarkMessagesCompacted(_ context.Context, arg sqlc.MarkMessagesCompactedParams) error {
	f.markedIDs = append([]pgtype.UUID(nil), arg.Column2...)
	return nil
}

func (f *fakeQueries) CompleteCompactionLog(_ context.Context, arg sqlc.CompleteCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	f.completed = arg
	return sqlc.BotHistoryMessageCompact{ID: arg.ID, Status: arg.Status, Summary: arg.Summary}, nil
}

// --- harness ------------------------------------------------------------------

func newMachineryService(q dbstore.Queries) *Service {
	return NewService(slog.New(slog.DiscardHandler), q)
}

func machineryConfig(stub *stubModel, targetTokens int) TriggerConfig {
	return TriggerConfig{
		BotID:        uuid.NewString(),
		SessionID:    uuid.NewString(),
		ModelID:      "stub-model",
		ClientType:   "openai-completions",
		APIKey:       "test",
		BaseURL:      "http://stub.invalid",
		HTTPClient:   &http.Client{Transport: stub},
		TargetTokens: targetTokens,
	}
}

func idSet(ids []pgtype.UUID) map[pgtype.UUID]bool {
	m := make(map[pgtype.UUID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// machineryCorpus returns a deterministic session whose oldest portion contains
// two tool exchanges (one base64-image result, one structured stdout result),
// plus recent text turns. Indices are returned for precise assertions.
func machineryCorpus(t *testing.T) []sqlc.ListUncompactedMessagesBySessionRow {
	t.Helper()
	b64 := strings.Repeat("QUJD", 100) // 400 base64 chars
	return []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `[{"type":"text","text":"deploy please"}]`, 100),                                                                                          // 0
		mkRow(t, "assistant", `[{"type":"text","text":"on it"},{"type":"tool-call","toolCallId":"A","toolName":"screenshot","input":{}}]`, 100),                    // 1 call A
		mkRow(t, "tool", `[{"type":"tool-result","toolCallId":"A","toolName":"screenshot","result":{"mime":"image/png","data":"`+b64+`"}}]`, 100),                  // 2 result A (base64)
		mkRow(t, "assistant", `[{"type":"text","text":"captured the screen"}]`, 100),                                                                               // 3
		mkRow(t, "user", `[{"type":"text","text":"now build"}]`, 100),                                                                                              // 4
		mkRow(t, "assistant", `[{"type":"text","text":"running"},{"type":"tool-call","toolCallId":"B","toolName":"exec_command","input":{"cmd":"make"}}]`, 100),    // 5 call B
		mkRow(t, "tool", `[{"type":"tool-result","toolCallId":"B","toolName":"exec_command","result":{"exit_code":0,"stdout":"build ok done","stderr":""}}]`, 100), // 6 result B (structured)
		mkRow(t, "assistant", `[{"type":"text","text":"build finished"}]`, 100),                                                                                    // 7
		mkRow(t, "user", `[{"type":"text","text":"recent question"}]`, 100),                                                                                        // 8
		mkRow(t, "assistant", `[{"type":"text","text":"recent answer"}]`, 100),                                                                                     // 9
	}
}

// --- tests --------------------------------------------------------------------

func TestDoCompactionMarksToolAwareWindowAndRendersCleanPrompt(t *testing.T) {
	rows := machineryCorpus(t)
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "SUMMARY-OK"}
	svc := newMachineryService(q)

	// 10 msgs x 100 tokens. target 450: the naive cut would land at index 6
	// (compact 0..5, keep 6..9) — orphaning tool result B at the keep-side head.
	// The boundary guard must advance to 7, pulling result B in with its call, so
	// the marked set is exactly 0..6. If adjustForToolBoundary regressed, this
	// marks only 0..5 and the assertions below fail.
	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 450)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("summarizer called %d times, want 1", stub.calls)
	}
	if len(q.markedIDs) != 7 {
		t.Fatalf("marked %d messages, want 7 (0..6, tool exchange kept whole)", len(q.markedIDs))
	}
	marked := idSet(q.markedIDs)
	for i := 0; i <= 6; i++ {
		if !marked[rows[i].ID] {
			t.Fatalf("row %d should be marked compacted", i)
		}
	}
	for i := 7; i <= 9; i++ {
		if marked[rows[i].ID] {
			t.Fatalf("row %d (recent) should NOT be marked", i)
		}
	}
	if !marked[rows[6].ID] {
		t.Fatalf("tool result B must be pulled into the compact set with its call")
	}
	if q.completed.Status != "ok" || q.completed.Summary != "SUMMARY-OK" || q.completed.MessageCount != 7 {
		t.Fatalf("complete log = status=%q summary=%q count=%d", q.completed.Status, q.completed.Summary, q.completed.MessageCount)
	}

	// The summarizer prompt must carry clean rendered outcomes, no media, no noise.
	if !strings.Contains(stub.prompt, "build ok done") {
		t.Fatalf("structured tool outcome missing from prompt:\n%s", stub.prompt)
	}
	if !strings.Contains(stub.prompt, "[media]") || strings.Contains(stub.prompt, "QUJDQUJDQUJD") {
		t.Fatalf("base64 image result not scrubbed in prompt")
	}
	if strings.Contains(stub.prompt, `{"type":"text"`) || strings.Contains(stub.prompt, `"toolCallId"`) {
		t.Fatalf("raw JSON-envelope noise leaked into prompt:\n%s", stub.prompt)
	}
}

func TestDoCompactionInjectsPriorContext(t *testing.T) {
	rows := machineryCorpus(t)
	q := &fakeQueries{
		uncompacted: rows,
		priorLogs:   []sqlc.BotHistoryMessageCompact{{Summary: "earlier-segment-summary", Status: "ok"}},
	}
	stub := &stubModel{summary: "S2"}
	svc := newMachineryService(q)

	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 450)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if !strings.Contains(stub.prompt, "prior_context") || !strings.Contains(stub.prompt, "earlier-segment-summary") {
		t.Fatalf("prior summary not injected as prior context:\n%s", stub.prompt)
	}
}

func TestDoCompactionAllEmptyWindowSkipsModelAndMarking(t *testing.T) {
	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "assistant", `[{"type":"reasoning","text":"thinking a"}]`, 100),
		mkRow(t, "assistant", `[{"type":"reasoning","text":"thinking b"}]`, 100),
		mkRow(t, "assistant", `[{"type":"text","text":"recent kept"}]`, 100),
	}
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)

	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 150)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if stub.calls != 0 {
		t.Fatalf("summarizer should not be called for an all-empty window (calls=%d)", stub.calls)
	}
	if len(q.markedIDs) != 0 {
		t.Fatalf("nothing should be marked for an all-empty window (marked=%d)", len(q.markedIDs))
	}
	if q.created {
		t.Fatal("a no-op compaction must not create a log row")
	}
}

func TestDoCompactionIncompleteToolExchangeSkipsModelAndMarking(t *testing.T) {
	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		toolCallRow(t, 100),
		mkRow(t, "tool", `[]`, 100),
		mkRow(t, "assistant", `[{"type":"text","text":"recent kept"}]`, 100),
	}
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)

	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 150)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if stub.calls != 0 {
		t.Fatalf("summarizer should not be called when no row can be marked (calls=%d)", stub.calls)
	}
	if q.created {
		t.Fatal("a no-op compaction must not create a log row")
	}
	if len(q.markedIDs) != 0 {
		t.Fatalf("nothing should be marked for an incomplete tool exchange (marked=%d)", len(q.markedIDs))
	}
}

func TestDoCompactionMarksOnlyContiguousRunAcrossEmptyMiddleRow(t *testing.T) {
	// Window: an empty-rendering reasoning row sits between two rendered rows.
	// Marking both rendered rows under one compact_id would leave the raw
	// reasoning row between them and let the read path fold history out of
	// order. doCompaction must mark only the first contiguous run (row 0) and
	// leave row 2 for a later pass.
	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `[{"type":"text","text":"old question"}]`, 100),       // 0
		mkRow(t, "assistant", `[{"type":"reasoning","text":"thinking"}]`, 100), // 1 renders empty
		mkRow(t, "assistant", `[{"type":"text","text":"old answer"}]`, 100),    // 2
		mkRow(t, "user", `[{"type":"text","text":"recent question"}]`, 100),    // 3 kept
		mkRow(t, "assistant", `[{"type":"text","text":"recent answer"}]`, 100), // 4 kept
	}
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "SUMMARY"}
	svc := newMachineryService(q)

	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 250)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if len(q.markedIDs) != 1 || q.markedIDs[0] != rows[0].ID {
		t.Fatalf("marked = %d ids, want only the contiguous leading run [row 0]", len(q.markedIDs))
	}
	if q.completed.Status != "ok" || q.completed.MessageCount != 1 {
		t.Fatalf("complete log = status=%q count=%d, want ok/1", q.completed.Status, q.completed.MessageCount)
	}
}

func TestDoCompactionEmptyHistoryNoOp(t *testing.T) {
	q := &fakeQueries{}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)

	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 100)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if stub.calls != 0 || len(q.markedIDs) != 0 {
		t.Fatalf("empty history must be a no-op (calls=%d marked=%d)", stub.calls, len(q.markedIDs))
	}
	if q.created {
		t.Fatal("empty history must not create a log row")
	}
}

type failingModel struct{ calls int }

func (f *failingModel) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls++
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"boom"}}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestDoCompactionSummarizerFailureRecordsErrorWithoutMarking(t *testing.T) {
	rows := machineryCorpus(t)
	q := &fakeQueries{uncompacted: rows}
	svc := newMachineryService(q)

	cfg := machineryConfig(&stubModel{}, 450)
	cfg.HTTPClient = &http.Client{Transport: &failingModel{}}

	if err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
		t.Fatal("summarizer failure must surface an error")
	}
	if len(q.markedIDs) != 0 {
		t.Fatalf("nothing may be marked when the summarizer fails (marked=%d)", len(q.markedIDs))
	}
	if !q.created || q.completed.Status != "error" {
		t.Fatalf("a failed attempt must leave an error log row (created=%v status=%q)", q.created, q.completed.Status)
	}
}

func TestDoCompactionEmptySummaryRecordsErrorWithoutMarking(t *testing.T) {
	rows := machineryCorpus(t)
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "   "}
	svc := newMachineryService(q)

	if err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 450)); err == nil {
		t.Fatal("an empty summary must surface an error")
	}
	if len(q.markedIDs) != 0 {
		t.Fatalf("nothing may be marked when the summary is empty (marked=%d)", len(q.markedIDs))
	}
	if !q.created || q.completed.Status != "error" {
		t.Fatalf("an empty summary must leave an error log row (created=%v status=%q)", q.created, q.completed.Status)
	}
}

func TestRunCompactionFailureCooldownSkipsImmediateRetry(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	cfg := machineryConfig(&stubModel{}, 450)
	fail := &failingModel{}
	cfg.HTTPClient = &http.Client{Transport: fail}

	if err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
		t.Fatal("first attempt must run and fail")
	}
	if fail.calls != 1 {
		t.Fatalf("calls = %d, want 1", fail.calls)
	}

	if err := svc.RunCompactionSync(context.Background(), cfg); err != nil {
		t.Fatalf("cooldown skip must not surface an error: %v", err)
	}
	if fail.calls != 1 {
		t.Fatalf("immediate retry within cooldown must be skipped, calls=%d", fail.calls)
	}

	now = now.Add(compactionFailureCooldown + time.Second)
	if err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
		t.Fatal("attempt after cooldown must run and fail again")
	}
	if fail.calls != 2 {
		t.Fatalf("attempt after cooldown should run, calls=%d", fail.calls)
	}
}

func TestRunCompactionManualRequestBypassesFailureCooldown(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	autoCfg := machineryConfig(&stubModel{}, 450)
	autoCfg.HTTPClient = &http.Client{Transport: &failingModel{}}
	if err := svc.RunCompactionSync(context.Background(), autoCfg); err == nil {
		t.Fatal("first automatic attempt must run and fail, arming the cooldown")
	}

	// The user fixes the model and presses compact within the cooldown window.
	// A manual request must actually run (not be skipped and reported as done):
	// it compacts and reports a real result instead of a false success.
	manualStub := &stubModel{summary: "recovered by manual run"}
	manualCfg := autoCfg
	manualCfg.Manual = true
	manualCfg.HTTPClient = &http.Client{Transport: manualStub}
	if err := svc.RunCompactionSync(context.Background(), manualCfg); err != nil {
		t.Fatalf("manual compaction must run despite cooldown: %v", err)
	}
	if manualStub.calls != 1 {
		t.Fatalf("manual request must call the model, not skip on cooldown (calls=%d)", manualStub.calls)
	}
	if !q.created || len(q.markedIDs) == 0 || q.completed.Status != "ok" {
		t.Fatalf("manual run must do real work: created=%v marked=%d status=%q", q.created, len(q.markedIDs), q.completed.Status)
	}

	// An automatic request in the same window still respects the cooldown: the
	// manual success above cleared it, so this one runs — proving cooldown is a
	// shared per-session state that manual participates in, not a bypass leak.
	autoRetry := &failingModel{}
	autoRetryCfg := autoCfg
	autoRetryCfg.HTTPClient = &http.Client{Transport: autoRetry}
	if err := svc.RunCompactionSync(context.Background(), autoRetryCfg); err == nil {
		t.Fatal("automatic retry after a successful manual run should proceed and fail")
	}
	if autoRetry.calls != 1 {
		t.Fatalf("manual success must clear the shared cooldown for automatic runs too (calls=%d)", autoRetry.calls)
	}
}

func TestRunCompactionManualFailureStillSurfacesError(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	autoCfg := machineryConfig(&stubModel{}, 450)
	autoCfg.HTTPClient = &http.Client{Transport: &failingModel{}}
	if err := svc.RunCompactionSync(context.Background(), autoCfg); err == nil {
		t.Fatal("automatic attempt must fail to arm the cooldown")
	}

	// A manual request that also fails must surface the real error, never a
	// silent nil that callers render as "done".
	manualFail := &failingModel{}
	manualCfg := autoCfg
	manualCfg.Manual = true
	manualCfg.HTTPClient = &http.Client{Transport: manualFail}
	if err := svc.RunCompactionSync(context.Background(), manualCfg); err == nil {
		t.Fatal("a failing manual compaction must return an error, not a false success")
	}
	if manualFail.calls != 1 {
		t.Fatalf("manual request must attempt the model despite cooldown (calls=%d)", manualFail.calls)
	}
}

func TestRunCompactionFailureCooldownClearsOnSuccess(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	sessionCfg := machineryConfig(&stubModel{}, 450)

	failCfg := sessionCfg
	failCfg.HTTPClient = &http.Client{Transport: &failingModel{}}
	if err := svc.RunCompactionSync(context.Background(), failCfg); err == nil {
		t.Fatal("expected initial failure")
	}

	now = now.Add(compactionFailureCooldown + time.Second)
	successStub := &stubModel{summary: "recovered"}
	successCfg := sessionCfg
	successCfg.HTTPClient = &http.Client{Transport: successStub}
	if err := svc.RunCompactionSync(context.Background(), successCfg); err != nil {
		t.Fatalf("attempt after cooldown should succeed: %v", err)
	}
	if successStub.calls != 1 {
		t.Fatalf("success attempt should have called the model once, got %d", successStub.calls)
	}

	retryFail := &failingModel{}
	retryCfg := sessionCfg
	retryCfg.HTTPClient = &http.Client{Transport: retryFail}
	if err := svc.RunCompactionSync(context.Background(), retryCfg); err == nil {
		t.Fatal("expected failure from immediate retry model")
	}
	if retryFail.calls != 1 {
		t.Fatalf("success must have cleared the cooldown, allowing an immediate retry; calls=%d", retryFail.calls)
	}
}

func TestRunCompactionSkipsWhenSessionAlreadyInFlight(t *testing.T) {
	rows := machineryCorpus(t)
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)

	cfg := machineryConfig(stub, 450)
	if !svc.beginSessionCompaction(cfg.SessionID) {
		t.Fatal("first acquisition must succeed")
	}
	defer svc.endSessionCompaction(cfg.SessionID)

	if err := svc.RunCompactionSync(context.Background(), cfg); err != nil {
		t.Fatalf("in-flight skip must not error: %v", err)
	}
	if stub.calls != 0 || q.created || len(q.markedIDs) != 0 {
		t.Fatalf("in-flight session must skip entirely (calls=%d created=%v marked=%d)", stub.calls, q.created, len(q.markedIDs))
	}
}
