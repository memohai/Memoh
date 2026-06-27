package flow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestCompactOrdinaryStrategySystemFlow(t *testing.T) {
	t.Parallel()

	const botID = "00000000-0000-0000-0000-000000000101"
	const sessionID = "00000000-0000-0000-0000-000000000102"
	const modelID = "00000000-0000-0000-0000-000000000103"
	const priorCompactID = "00000000-0000-0000-0000-000000000301"

	row := func(id string, role string, content json.RawMessage, atMs int64, mutate func(*messagepkg.Message)) messagepkg.Message {
		msg := messagepkg.Message{
			ID:        id,
			BotID:     botID,
			SessionID: sessionID,
			Role:      role,
			Content:   content,
			Usage:     json.RawMessage(`{"outputTokens":100}`),
			CreatedAt: time.UnixMilli(atMs).UTC(),
		}
		if mutate != nil {
			mutate(&msg)
		}
		return msg
	}

	rows := []messagepkg.Message{
		row("00000000-0000-0000-0000-000000000201", "user", conversation.NewTextContent("old compacted user"), 100, func(msg *messagepkg.Message) {
			msg.ExternalMessageID = "external-old"
			msg.Platform = "telegram"
		}),
		row("00000000-0000-0000-0000-000000000202", "user", conversation.NewTextContent("old edit body"), 120, func(msg *messagepkg.Message) {
			msg.ExternalMessageID = "external-edited"
			msg.Platform = "telegram"
		}),
		row("00000000-0000-0000-0000-000000000203", "user", conversation.NewTextContent("old delete body"), 130, func(msg *messagepkg.Message) {
			msg.ExternalMessageID = "external-deleted"
			msg.Platform = "telegram"
		}),
		row("00000000-0000-0000-0000-000000000204", "assistant", mustRawJSON(t, conversation.ModelMessage{
			Role: "assistant",
			Content: json.RawMessage(`[
				{"type":"text","text":"running old command"},
				{"type":"tool-call","toolCallId":"call-build","toolName":"exec_command","input":{"cmd":"make test"}}
			]`),
		}), 150, nil),
		row("00000000-0000-0000-0000-000000000205", "tool", mustRawJSON(t, conversation.ModelMessage{
			Role: "tool",
			Content: json.RawMessage(`[
				{"type":"tool-result","toolCallId":"call-build","toolName":"exec_command","result":{"stdout":"build ok","stderr":"","exit_code":0}}
			]`),
		}), 160, nil),
		row("00000000-0000-0000-0000-000000000206", "assistant", mustRawJSON(t, conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("old compacted assistant"),
		}), 180, nil),
		row("00000000-0000-0000-0000-000000000207", "user", conversation.NewTextContent("fresh user"), 300, func(msg *messagepkg.Message) {
			msg.ExternalMessageID = "external-new"
			msg.Platform = "telegram"
		}),
		row("00000000-0000-0000-0000-000000000208", "assistant", mustRawJSON(t, conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("fresh assistant"),
		}), 400, nil),
	}

	store := &compactSystemStore{
		messages:   slices.Clone(rows),
		completeAt: time.UnixMilli(200).UTC(),
		logs: []dbsqlc.BotHistoryMessageCompact{
			{
				ID:        flowTestUUID(priorCompactID),
				BotID:     flowTestUUID(botID),
				SessionID: flowTestUUID(sessionID),
				Status:    "ok",
				Summary:   "previous compact summary",
				StartedAt: pgtype.Timestamptz{Time: time.UnixMilli(50).UTC(), Valid: true},
			},
		},
	}
	model := &compactSystemModel{summary: "new compact summary"}
	svc := compaction.NewService(slog.New(slog.DiscardHandler), store)

	err := svc.RunCompactionSync(context.Background(), compaction.TriggerConfig{
		BotID:            botID,
		SessionID:        sessionID,
		ModelID:          modelID,
		ClientType:       "openai-completions",
		APIKey:           "test",
		BaseURL:          "http://stub.invalid",
		HTTPClient:       &http.Client{Transport: model},
		Ratio:            70,
		TotalInputTokens: 1000,
		MaxCompactTokens: 30000,
	})
	if err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	if model.calls != 1 {
		t.Fatalf("summarizer calls = %d, want 1", model.calls)
	}
	if store.completed.Status != "ok" || store.completed.Summary != "new compact summary" || store.completed.MessageCount != 6 {
		t.Fatalf("completed compact log mismatch: %#v", store.completed)
	}
	compactID := compactSystemUUIDString(store.completed.ID)
	if compactID == "" {
		t.Fatalf("completed compact id is empty: %#v", store.completed.ID)
	}
	for _, id := range []string{
		"00000000-0000-0000-0000-000000000201",
		"00000000-0000-0000-0000-000000000202",
		"00000000-0000-0000-0000-000000000203",
		"00000000-0000-0000-0000-000000000204",
		"00000000-0000-0000-0000-000000000205",
		"00000000-0000-0000-0000-000000000206",
	} {
		if got := store.messageByID(id).CompactID; got != compactID {
			t.Fatalf("message %s compact_id = %q, want %q", id, got, compactID)
		}
	}
	for _, id := range []string{
		"00000000-0000-0000-0000-000000000207",
		"00000000-0000-0000-0000-000000000208",
	} {
		if got := store.messageByID(id).CompactID; got != "" {
			t.Fatalf("recent message %s compact_id = %q, want empty", id, got)
		}
	}
	for _, want := range []string{"prior_context", "previous compact summary", "old compacted user", "old edit body", "old delete body", "[tool_call: exec_command]", "build ok", "old compacted assistant"} {
		if !strings.Contains(model.prompt, want) {
			t.Fatalf("compaction prompt missing %q:\n%s", want, model.prompt)
		}
	}

	p := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pushSystemCompactMessageEvents(p, sessionID)
	resolver := &Resolver{
		logger:         slog.New(slog.DiscardHandler),
		pipeline:       p,
		messageService: &compactSystemMessageService{store: store},
		queries:        store,
	}

	messages := resolver.buildPipelineContext(context.Background(), conversation.ChatRequest{SessionID: sessionID}, 0).Messages
	got := make([]string, 0, len(messages))
	for _, msg := range messages {
		got = append(got, msg.TextContent())
	}
	joined := strings.Join(got, "\n")

	if len(got) < 3 {
		t.Fatalf("message count = %d, want summary, replayed user context, and fresh assistant: %#v", len(got), got)
	}
	if got[0] != "[Conversation summary]\nnew compact summary" {
		t.Fatalf("first message = %q, want compact summary; all=%#v", got[0], got)
	}
	for _, compactedText := range []string{"old compacted user", "old compacted assistant", "running old command", "build ok", "old edit body", "old delete body"} {
		if strings.Contains(joined, compactedText) {
			t.Fatalf("compacted text %q leaked into final context: %#v", compactedText, got)
		}
	}
	for _, want := range []string{"edited after compact", `<message id="external-deleted"`, "/>", "fresh user", "fresh assistant"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("final context missing %q: %#v", want, got)
		}
	}
}

func pushSystemCompactMessageEvents(p *pipelinepkg.Pipeline, sessionID string) {
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old compacted user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-edited",
		ReceivedAtMs: 120,
		TimestampSec: 120,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old edit body"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-deleted",
		ReceivedAtMs: 130,
		TimestampSec: 130,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "old delete body"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	p.PushEvent(sessionID, pipelinepkg.EditEvent{
		SessionID:    sessionID,
		MessageID:    "external-edited",
		ReceivedAtMs: 250,
		TimestampSec: 250,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "edited after compact"}},
	})
	p.PushEvent(sessionID, pipelinepkg.DeleteEvent{
		SessionID:    sessionID,
		MessageIDs:   []string{"external-deleted"},
		ReceivedAtMs: 260,
		TimestampSec: 260,
	})
	p.PushEvent(sessionID, pipelinepkg.MessageEvent{
		SessionID:    sessionID,
		MessageID:    "external-new",
		ReceivedAtMs: 300,
		TimestampSec: 300,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "fresh user"}},
		Conversation: pipelinepkg.ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
}

type compactSystemModel struct {
	summary string
	calls   int
	prompt  string
}

func (m *compactSystemModel) RoundTrip(req *http.Request) (*http.Response, error) {
	m.calls++
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		m.prompt = compactSystemPrompt(body)
	}
	resp := `{"id":"stub","object":"chat.completion","created":0,"model":"stub",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":` + compactSystemJSONStr(m.summary) + `},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(resp)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func compactSystemPrompt(body []byte) string {
	var req struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	var out strings.Builder
	for _, msg := range req.Messages {
		var text string
		if json.Unmarshal(msg.Content, &text) == nil {
			out.WriteString(text)
			out.WriteByte('\n')
			continue
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(msg.Content, &parts) == nil {
			for _, part := range parts {
				out.WriteString(part.Text)
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}

func compactSystemJSONStr(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

type compactSystemStore struct {
	dbstore.Queries
	messages   []messagepkg.Message
	logs       []dbsqlc.BotHistoryMessageCompact
	completed  dbsqlc.CompleteCompactionLogParams
	completeAt time.Time
}

func (s *compactSystemStore) CreateCompactionLog(_ context.Context, arg dbsqlc.CreateCompactionLogParams) (dbsqlc.BotHistoryMessageCompact, error) {
	log := dbsqlc.BotHistoryMessageCompact{
		ID:        flowTestUUID("00000000-0000-0000-0000-000000000302"),
		BotID:     arg.BotID,
		SessionID: arg.SessionID,
		Status:    "running",
		StartedAt: pgtype.Timestamptz{Time: time.UnixMilli(190).UTC(), Valid: true},
	}
	s.logs = append(s.logs, log)
	return log, nil
}

func (s *compactSystemStore) ListUncompactedMessagesBySession(_ context.Context, sessionID pgtype.UUID) ([]dbsqlc.ListUncompactedMessagesBySessionRow, error) {
	var rows []dbsqlc.ListUncompactedMessagesBySessionRow
	for _, msg := range s.messages {
		if msg.SessionID != compactSystemUUIDString(sessionID) || strings.TrimSpace(msg.CompactID) != "" {
			continue
		}
		rows = append(rows, compactSystemUncompactedRow(msg))
	}
	return rows, nil
}

func (s *compactSystemStore) ListCompactionLogsBySession(_ context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotHistoryMessageCompact, error) {
	var logs []dbsqlc.BotHistoryMessageCompact
	for _, log := range s.logs {
		if log.SessionID == sessionID {
			logs = append(logs, log)
		}
	}
	return logs, nil
}

func (s *compactSystemStore) MarkMessagesCompacted(_ context.Context, arg dbsqlc.MarkMessagesCompactedParams) error {
	compactID := compactSystemUUIDString(arg.CompactID)
	marked := make(map[string]struct{}, len(arg.Column2))
	for _, id := range arg.Column2 {
		marked[compactSystemUUIDString(id)] = struct{}{}
	}
	for i := range s.messages {
		if _, ok := marked[s.messages[i].ID]; ok {
			s.messages[i].CompactID = compactID
		}
	}
	return nil
}

func (s *compactSystemStore) CompleteCompactionLog(_ context.Context, arg dbsqlc.CompleteCompactionLogParams) (dbsqlc.BotHistoryMessageCompact, error) {
	s.completed = arg
	for i := range s.logs {
		if s.logs[i].ID != arg.ID {
			continue
		}
		s.logs[i].Status = arg.Status
		s.logs[i].Summary = arg.Summary
		s.logs[i].MessageCount = arg.MessageCount
		s.logs[i].ErrorMessage = arg.ErrorMessage
		s.logs[i].Usage = arg.Usage
		s.logs[i].ModelID = arg.ModelID
		s.logs[i].CompletedAt = pgtype.Timestamptz{Time: s.completeAt, Valid: true}
		return s.logs[i], nil
	}
	return dbsqlc.BotHistoryMessageCompact{}, errors.New("compact log not found")
}

func (s *compactSystemStore) GetCompactionLogByID(_ context.Context, id pgtype.UUID) (dbsqlc.BotHistoryMessageCompact, error) {
	for _, log := range s.logs {
		if log.ID == id {
			return log, nil
		}
	}
	return dbsqlc.BotHistoryMessageCompact{}, errors.New("compact log not found")
}

func (s *compactSystemStore) messageByID(id string) messagepkg.Message {
	for _, msg := range s.messages {
		if msg.ID == id {
			return msg
		}
	}
	return messagepkg.Message{}
}

func compactSystemUncompactedRow(msg messagepkg.Message) dbsqlc.ListUncompactedMessagesBySessionRow {
	metadata, _ := json.Marshal(msg.Metadata)
	return dbsqlc.ListUncompactedMessagesBySessionRow{
		ID:                      flowTestUUID(msg.ID),
		BotID:                   flowTestUUID(msg.BotID),
		SessionID:               flowTestUUID(msg.SessionID),
		SenderChannelIdentityID: compactSystemOptionalUUID(msg.SenderChannelIdentityID),
		SenderUserID:            compactSystemOptionalUUID(msg.SenderUserID),
		ExternalMessageID:       compactSystemText(msg.ExternalMessageID),
		SourceReplyToMessageID:  compactSystemText(msg.SourceReplyToMessageID),
		Role:                    msg.Role,
		Content:                 msg.Content,
		Metadata:                metadata,
		Usage:                   msg.Usage,
		EventID:                 compactSystemOptionalUUID(msg.EventID),
		DisplayText:             compactSystemText(msg.DisplayContent),
		CompactID:               compactSystemOptionalUUID(msg.CompactID),
		CreatedAt:               pgtype.Timestamptz{Time: msg.CreatedAt, Valid: true},
		SenderDisplayName:       compactSystemText(msg.SenderDisplayName),
		SenderAvatarUrl:         compactSystemText(msg.SenderAvatarURL),
		Platform:                compactSystemText(msg.Platform),
		ConversationType:        pgtype.Text{String: "group", Valid: true},
		ConversationName:        "Ops Room",
		ReplyTarget:             pgtype.Text{String: "thread-1", Valid: true},
	}
}

func compactSystemOptionalUUID(value string) pgtype.UUID {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.UUID{}
	}
	return flowTestUUID(value)
}

func compactSystemText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func compactSystemUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	u, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return ""
	}
	return u.String()
}

type compactSystemMessageService struct {
	recordingMessageService
	store *compactSystemStore
}

func (s *compactSystemMessageService) ListActiveSinceBySession(_ context.Context, sessionID string, since time.Time) ([]messagepkg.Message, error) {
	var rows []messagepkg.Message
	for _, msg := range s.store.messages {
		if msg.SessionID != sessionID {
			continue
		}
		if msg.CreatedAt.IsZero() || !msg.CreatedAt.Before(since) {
			rows = append(rows, msg)
		}
	}
	return rows, nil
}
