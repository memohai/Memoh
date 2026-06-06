package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestCreateBotStreamsLifecycleWhenSSERequested(t *testing.T) {
	ownerID := "00000000-0000-0000-0000-000000000101"
	botID := "00000000-0000-0000-0000-000000000201"
	botUUID := testUUID(botID)

	handler := &UsersHandler{
		service: newTestCreateBotAccountService(ownerID),
		botService: bots.NewService(nil, postgresstore.NewQueries(sqlc.New(&createBotStreamDB{
			ownerID: ownerID,
			botID:   botID,
		}))),
		acpWorkspace: &createBotStreamWorkspace{},
	}

	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(`{
		"name": "stream-bot",
		"display_name": "Stream Bot",
		"acl_preset": "allow_all",
		"wait_for_ready": true
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	ctx := testAuthContext(echo.New(), req, rec, ownerID)

	if err := handler.CreateBot(ctx); err != nil {
		t.Fatalf("CreateBot() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get(echo.HeaderContentType); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	events := decodeSSEEvents(t, rec.Body.String())
	if len(events) < 2 {
		t.Fatalf("events len = %d, want at least bot_created and ready: %#v", len(events), events)
	}
	if events[0]["type"] != "bot_created" {
		t.Fatalf("first event type = %#v, want bot_created; events=%#v", events[0]["type"], events)
	}
	if got := eventBotID(events[0]); got != botID {
		t.Fatalf("created bot id = %q, want %q", got, botID)
	}
	last := events[len(events)-1]
	if last["type"] != "ready" {
		t.Fatalf("last event type = %#v, want ready; events=%#v", last["type"], events)
	}
	if got := eventBotID(last); got != botID {
		t.Fatalf("ready bot id = %q, want %q", got, botID)
	}
	if handler.botService == nil {
		t.Fatal("bot service should be configured")
	}
	if botUUID.Valid != true {
		t.Fatal("bot UUID helper sanity check failed")
	}
}

func TestCreateBotStreamRequiresWorkspaceLifecycle(t *testing.T) {
	ownerID := "00000000-0000-0000-0000-000000000103"

	handler := &UsersHandler{
		service: newTestCreateBotAccountService(ownerID),
		botService: bots.NewService(nil, postgresstore.NewQueries(sqlc.New(&createBotStreamDB{
			ownerID: ownerID,
			botID:   "00000000-0000-0000-0000-000000000203",
		}))),
	}

	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(`{
		"name": "misconfigured-bot",
		"display_name": "Misconfigured Bot",
		"acl_preset": "allow_all",
		"wait_for_ready": true
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	ctx := testAuthContext(echo.New(), req, rec, ownerID)

	err := handler.CreateBot(ctx)
	if err == nil {
		t.Fatal("CreateBot() error = nil, want workspace lifecycle configuration error")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("CreateBot() error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusInternalServerError {
		t.Fatalf("CreateBot() status = %d, want %d", httpErr.Code, http.StatusInternalServerError)
	}
}

func TestCreateBotStreamsContainerProgressEvents(t *testing.T) {
	ownerID := "00000000-0000-0000-0000-000000000102"
	botID := "00000000-0000-0000-0000-000000000202"

	handler := &UsersHandler{
		service: newTestCreateBotAccountService(ownerID),
		botService: bots.NewService(nil, postgresstore.NewQueries(sqlc.New(&createBotStreamDB{
			ownerID: ownerID,
			botID:   botID,
		}))),
		acpWorkspace: &createBotStreamWorkspace{events: []workspace.ContainerSetupEvent{
			{Type: "pulling", Image: "debian:bookworm-slim"},
			{Type: "pull_progress", Layers: []ctr.LayerStatus{{Ref: "layer-1", Offset: 10, Total: 100}}},
			{Type: "creating"},
		}},
	}

	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(`{
		"name": "progress-bot",
		"display_name": "Progress Bot",
		"acl_preset": "allow_all",
		"wait_for_ready": true
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	ctx := testAuthContext(echo.New(), req, rec, ownerID)

	if err := handler.CreateBot(ctx); err != nil {
		t.Fatalf("CreateBot() error = %v", err)
	}

	events := decodeSSEEvents(t, rec.Body.String())
	for _, eventType := range []string{"bot_created", "pulling", "pull_progress", "creating", "ready"} {
		if !hasEventType(events, eventType) {
			t.Fatalf("missing %q event: %#v", eventType, events)
		}
	}
}

func TestCreateBotStreamReportsSetupErrorAfterCreatedBot(t *testing.T) {
	ownerID := "00000000-0000-0000-0000-000000000104"
	botID := "00000000-0000-0000-0000-000000000204"

	handler := &UsersHandler{
		logger:  slog.Default(),
		service: newTestCreateBotAccountService(ownerID),
		botService: bots.NewService(nil, postgresstore.NewQueries(sqlc.New(&createBotStreamDB{
			ownerID: ownerID,
			botID:   botID,
		}))),
		acpWorkspace: &createBotStreamWorkspace{err: errors.New("image pull failed")},
	}

	req := httptest.NewRequest(http.MethodPost, "/bots", strings.NewReader(`{
		"name": "setup-failed-bot",
		"display_name": "Setup Failed Bot",
		"acl_preset": "allow_all",
		"wait_for_ready": true
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "text/event-stream")
	rec := httptest.NewRecorder()
	ctx := testAuthContext(echo.New(), req, rec, ownerID)

	if err := handler.CreateBot(ctx); err != nil {
		t.Fatalf("CreateBot() error = %v", err)
	}

	events := decodeSSEEvents(t, rec.Body.String())
	if len(events) < 2 {
		t.Fatalf("events len = %d, want bot_created and error: %#v", len(events), events)
	}
	if events[0]["type"] != "bot_created" {
		t.Fatalf("first event type = %#v, want bot_created; events=%#v", events[0]["type"], events)
	}
	if got := eventBotID(events[0]); got != botID {
		t.Fatalf("created bot id = %q, want %q", got, botID)
	}
	last := events[len(events)-1]
	if last["type"] != "error" {
		t.Fatalf("last event type = %#v, want error; events=%#v", last["type"], events)
	}
	message, _ := last["message"].(string)
	if !strings.Contains(message, "container setup failed: image pull failed") {
		t.Fatalf("error message = %q, want setup failure details", message)
	}
}

func decodeSSEEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	events := make([]map[string]any, 0)
	for _, block := range strings.Split(raw, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatalf("decode event %q: %v", line, err)
			}
			events = append(events, event)
		}
	}
	return events
}

func eventBotID(event map[string]any) string {
	bot, ok := event["bot"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := bot["id"].(string)
	return id
}

func hasEventType(events []map[string]any, eventType string) bool {
	for _, event := range events {
		if event["type"] == eventType {
			return true
		}
	}
	return false
}

func newTestCreateBotAccountService(userID string) *accounts.Service {
	return accounts.NewService(nil, createBotAccountStore{userID: userID})
}

type createBotStreamWorkspace struct {
	events []workspace.ContainerSetupEvent
	err    error
}

func (w *createBotStreamWorkspace) MCPClient(context.Context, string) (*bridge.Client, error) {
	return nil, nil
}

func (w *createBotStreamWorkspace) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer}, nil
}

func (w *createBotStreamWorkspace) SetupBotContainerWithProgress(_ context.Context, _ string, progress workspace.ContainerSetupProgress) error {
	for _, event := range w.events {
		progress(event)
	}
	return w.err
}

type createBotAccountStore struct {
	dbstore.AccountStore
	userID string
}

func (s createBotAccountStore) GetByUserID(_ context.Context, userID string) (dbstore.AccountRecord, error) {
	if userID != s.userID {
		return dbstore.AccountRecord{}, pgx.ErrNoRows
	}
	return dbstore.AccountRecord{ID: userID, Role: "member", IsActive: true}, nil
}

type createBotStreamDB struct {
	ownerID string
	botID   string
}

func (d *createBotStreamDB) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (*createBotStreamDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (d *createBotStreamDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	switch {
	case strings.Contains(query, "FROM users") && strings.Contains(query, "WHERE id = $1"):
		return &createBotStreamRow{scanFunc: func(_ ...any) error { return nil }}
	case strings.Contains(query, "INSERT INTO bots"):
		return d.botRow(bots.BotStatusCreating)
	case strings.Contains(query, "FROM bots") && strings.Contains(query, "WHERE id = $1"):
		return d.botRow(bots.BotStatusReady)
	case strings.Contains(query, "FROM bots") && strings.Contains(query, "WHERE name = $1"):
		return &createBotStreamRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
	default:
		_ = args
		return &createBotStreamRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
	}
}

func (d *createBotStreamDB) botRow(status string) pgx.Row {
	botID := testUUID(d.botID)
	ownerID := testUUID(d.ownerID)
	return &createBotStreamRow{scanFunc: func(dest ...any) error {
		if len(dest) < 20 {
			return pgx.ErrNoRows
		}
		*dest[0].(*pgtype.UUID) = botID
		*dest[1].(*pgtype.UUID) = ownerID
		*dest[2].(*string) = "stream-bot"
		*dest[3].(*pgtype.Text) = pgtype.Text{String: "Stream Bot", Valid: true}
		*dest[4].(*pgtype.Text) = pgtype.Text{}
		*dest[5].(*pgtype.Text) = pgtype.Text{}
		*dest[6].(*bool) = true
		*dest[7].(*string) = status
		*dest[8].(*string) = "en"
		*dest[9].(*bool) = false
		*dest[10].(*string) = "medium"
		*dest[11].(*pgtype.UUID) = pgtype.UUID{}
		*dest[12].(*pgtype.UUID) = pgtype.UUID{}
		*dest[13].(*pgtype.UUID) = pgtype.UUID{}
		*dest[14].(*bool) = false
		*dest[15].(*int32) = 30
		*dest[16].(*string) = ""
		if len(dest) == 20 {
			*dest[17].(*[]byte) = []byte(`{}`)
			*dest[18].(*pgtype.Timestamptz) = pgtype.Timestamptz{Valid: false}
			*dest[19].(*pgtype.Timestamptz) = pgtype.Timestamptz{Valid: false}
			return nil
		}
		*dest[17].(*bool) = false
		*dest[18].(*int32) = 200
		*dest[19].(*int32) = 50
		*dest[20].(*pgtype.UUID) = pgtype.UUID{}
		*dest[21].(*[]byte) = []byte(`{}`)
		*dest[22].(*pgtype.Timestamptz) = pgtype.Timestamptz{Valid: false}
		*dest[23].(*pgtype.Timestamptz) = pgtype.Timestamptz{Valid: false}
		return nil
	}}
}

type createBotStreamRow struct {
	scanFunc func(dest ...any) error
}

func (r *createBotStreamRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}
