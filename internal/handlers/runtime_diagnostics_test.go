package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/runtimediagnostics"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type runtimeDiagnosticsQueries struct {
	dbstore.Queries
	bot    sqlc.GetBotByIDRow
	grants []sqlc.ListBotUserGrantsForUserRow
}

func (q runtimeDiagnosticsQueries) GetBotByID(context.Context, pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q runtimeDiagnosticsQueries) ListBotUserGrantsForUser(context.Context, sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
	return q.grants, nil
}

func TestRuntimeDiagnosticsHandlerRequiresManagePermission(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	viewerID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	queries := runtimeDiagnosticsQueries{
		bot: testBotRow(botID, map[string]any{}),
		grants: []sqlc.ListBotUserGrantsForUserRow{{
			ID:          testUUID("cccccccc-cccc-cccc-cccc-cccccccccccc"),
			BotID:       testUUID(botID),
			SubjectType: bots.GrantSubjectUser,
			UserID:      testUUID(viewerID),
			Permissions: []byte(`["chat"]`),
		}},
	}
	workspaceFake := &runtimeDiagnosticsWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: t.TempDir(),
		},
	}
	handler := NewRuntimeDiagnosticsHandler(
		runtimediagnostics.NewService(nil, workspaceFake, nil, nil, nil),
		bots.NewService(nil, queries),
		newTestAdminAccountService("user"),
	)

	req := httptest.NewRequest(http.MethodGet, "/bots/"+botID+"/runtime-diagnostics", nil)
	rec := httptest.NewRecorder()
	echoCtx := runtimeDiagnosticsEchoContext(req, rec, viewerID, botID)

	err := handler.Get(echoCtx)
	if err == nil {
		t.Fatal("Get() error = nil, want forbidden")
	}
	httpErr := requireHTTPError(t, err)
	if httpErr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", httpErr.Code)
	}
	if workspaceFake.workspaceInfoCalls != 0 {
		t.Fatalf("workspace diagnostics called %d time(s), want 0 before manage authorization", workspaceFake.workspaceInfoCalls)
	}
}

func TestRuntimeDiagnosticsHandlerReturnsReadOnlyDiagnostics(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	ownerID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	queries := runtimeDiagnosticsQueries{bot: testBotRow(botID, map[string]any{})}
	workspaceFake := &runtimeDiagnosticsWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: t.TempDir(),
		},
		mcpErr: errors.New("MCPClient should not be called for local diagnostics"),
	}
	handler := NewRuntimeDiagnosticsHandler(
		runtimediagnostics.NewService(nil, workspaceFake, nil, nil, nil),
		bots.NewService(nil, queries),
		newTestAdminAccountService("user"),
	)

	req := httptest.NewRequest(http.MethodGet, "/bots/"+botID+"/runtime-diagnostics", nil)
	rec := httptest.NewRecorder()
	echoCtx := runtimeDiagnosticsEchoContext(req, rec, ownerID, botID)

	if err := handler.Get(echoCtx); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp runtimediagnostics.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Workspace.Backend != bridge.WorkspaceBackendLocal {
		t.Fatalf("workspace backend = %q, want local", resp.Workspace.Backend)
	}
	if resp.Workspace.Evidence["bridge_probe"] != "skipped_for_local_readonly" {
		t.Fatalf("workspace evidence = %#v, want local bridge probe skipped", resp.Workspace.Evidence)
	}
	if workspaceFake.mcpCalls != 0 {
		t.Fatalf("MCPClient called %d time(s), want 0 for local read-only diagnostics", workspaceFake.mcpCalls)
	}
}

func runtimeDiagnosticsEchoContext(req *http.Request, rec http.ResponseWriter, userID string, botID string) echo.Context {
	e := echo.New()
	ctx := testAuthContext(e, req, rec, userID)
	ctx.SetPath("/bots/:bot_id/runtime-diagnostics")
	ctx.SetParamNames("bot_id")
	ctx.SetParamValues(botID)
	return ctx
}

type runtimeDiagnosticsWorkspaceFake struct {
	info               bridge.WorkspaceInfo
	mcpErr             error
	workspaceInfoCalls int
	mcpCalls           int
	displayEnabled     bool
	displaySocket      string
}

func (w *runtimeDiagnosticsWorkspaceFake) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	w.workspaceInfoCalls++
	return w.info, nil
}

func (w *runtimeDiagnosticsWorkspaceFake) MCPClient(context.Context, string) (*bridge.Client, error) {
	w.mcpCalls++
	return nil, w.mcpErr
}

func (w *runtimeDiagnosticsWorkspaceFake) BotDisplayEnabled(context.Context, string) bool {
	return w.displayEnabled
}

func (w *runtimeDiagnosticsWorkspaceFake) DisplaySocketPath(string) string {
	return w.displaySocket
}

func (*runtimeDiagnosticsWorkspaceFake) GetContainerInfo(context.Context, string) (*workspace.ContainerStatus, error) {
	return nil, workspace.ErrContainerNotFound
}

func (*runtimeDiagnosticsWorkspaceFake) GetContainerMetrics(context.Context, string) (*workspace.ContainerMetricsResult, error) {
	return &workspace.ContainerMetricsResult{Supported: true}, nil
}
