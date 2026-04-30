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
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/orchestration"
)

type fakeOrchestrationService struct {
	startRun              func(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error)
	cancelRun             func(context.Context, orchestration.ControlIdentity, string, orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error)
	getRunSnapshot        func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error)
	getRunSnapshotAtSeq   func(context.Context, orchestration.ControlIdentity, string, uint64) (*orchestration.RunSnapshot, error)
	listBotRuns           func(context.Context, orchestration.ControlIdentity, string, orchestration.ListBotRunsRequest) (*orchestration.RunListPage, error)
	getRunInspector       func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunInspector, error)
	listRunTasks          func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error)
	createHumanCheckpoint func(context.Context, orchestration.ControlIdentity, orchestration.CreateHumanCheckpointRequest) (*orchestration.CreateHumanCheckpointResult, error)
	listRunCheckpoints    func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error)
	listRunArtifacts      func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunArtifactsRequest) (*orchestration.ArtifactPage, error)
	listRunEvents         func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error)
	resolveCheckpoint     func(context.Context, orchestration.ControlIdentity, string, orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error)
}

func (f fakeOrchestrationService) StartRun(ctx context.Context, caller orchestration.ControlIdentity, req orchestration.StartRunRequest) (orchestration.RunHandle, error) {
	return f.startRun(ctx, caller, req)
}

func (f fakeOrchestrationService) CancelRun(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error) {
	return f.cancelRun(ctx, caller, runID, req)
}

func (f fakeOrchestrationService) GetRunSnapshot(ctx context.Context, caller orchestration.ControlIdentity, runID string) (*orchestration.RunSnapshot, error) {
	return f.getRunSnapshot(ctx, caller, runID)
}

func (f fakeOrchestrationService) GetRunSnapshotAtSeq(ctx context.Context, caller orchestration.ControlIdentity, runID string, asOfSeq uint64) (*orchestration.RunSnapshot, error) {
	return f.getRunSnapshotAtSeq(ctx, caller, runID, asOfSeq)
}

func (f fakeOrchestrationService) ListBotRuns(ctx context.Context, caller orchestration.ControlIdentity, botID string, req orchestration.ListBotRunsRequest) (*orchestration.RunListPage, error) {
	return f.listBotRuns(ctx, caller, botID, req)
}

func (f fakeOrchestrationService) GetRunInspector(ctx context.Context, caller orchestration.ControlIdentity, runID string) (*orchestration.RunInspector, error) {
	return f.getRunInspector(ctx, caller, runID)
}

func (f fakeOrchestrationService) ListRunTasks(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error) {
	return f.listRunTasks(ctx, caller, runID, req)
}

func (f fakeOrchestrationService) CreateHumanCheckpoint(ctx context.Context, caller orchestration.ControlIdentity, req orchestration.CreateHumanCheckpointRequest) (*orchestration.CreateHumanCheckpointResult, error) {
	return f.createHumanCheckpoint(ctx, caller, req)
}

func (f fakeOrchestrationService) ListRunCheckpoints(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error) {
	return f.listRunCheckpoints(ctx, caller, runID, req)
}

func (f fakeOrchestrationService) ListRunArtifacts(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunArtifactsRequest) (*orchestration.ArtifactPage, error) {
	return f.listRunArtifacts(ctx, caller, runID, req)
}

func (f fakeOrchestrationService) ListRunEvents(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error) {
	return f.listRunEvents(ctx, caller, runID, req)
}

func (f fakeOrchestrationService) ResolveCheckpoint(ctx context.Context, caller orchestration.ControlIdentity, checkpointID string, req orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error) {
	return f.resolveCheckpoint(ctx, caller, checkpointID, req)
}

func newNoopOrchestrationService() fakeOrchestrationService {
	return fakeOrchestrationService{
		startRun: func(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error) {
			return orchestration.RunHandle{}, nil
		},
		cancelRun: func(context.Context, orchestration.ControlIdentity, string, orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error) {
			return &orchestration.CancelRunResult{}, nil
		},
		getRunSnapshot: func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error) {
			return &orchestration.RunSnapshot{}, nil
		},
		getRunSnapshotAtSeq: func(context.Context, orchestration.ControlIdentity, string, uint64) (*orchestration.RunSnapshot, error) {
			return &orchestration.RunSnapshot{}, nil
		},
		listBotRuns: func(context.Context, orchestration.ControlIdentity, string, orchestration.ListBotRunsRequest) (*orchestration.RunListPage, error) {
			return &orchestration.RunListPage{}, nil
		},
		getRunInspector: func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunInspector, error) {
			return &orchestration.RunInspector{}, nil
		},
		listRunTasks: func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error) {
			return &orchestration.TaskPage{}, nil
		},
		createHumanCheckpoint: func(context.Context, orchestration.ControlIdentity, orchestration.CreateHumanCheckpointRequest) (*orchestration.CreateHumanCheckpointResult, error) {
			return &orchestration.CreateHumanCheckpointResult{}, nil
		},
		listRunCheckpoints: func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error) {
			return &orchestration.HumanCheckpointPage{}, nil
		},
		listRunArtifacts: func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunArtifactsRequest) (*orchestration.ArtifactPage, error) {
			return &orchestration.ArtifactPage{}, nil
		},
		listRunEvents: func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error) {
			return &orchestration.RunEventPage{}, nil
		},
		resolveCheckpoint: func(context.Context, orchestration.ControlIdentity, string, orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error) {
			return &orchestration.ResolveCheckpointResult{}, nil
		},
	}
}

func newAuthedEcho(method, path string, body string) (*echo.Echo, *httptest.ResponseRecorder, *http.Request) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user", &jwt.Token{
				Valid: true,
				Claims: jwt.MapClaims{
					"user_id":   "user-123",
					"tenant_id": "tenant-123",
				},
			})
			return next(c)
		}
	})
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	return e, rec, req
}

func newJWTProtectedEcho(method, path string, body string, secret string) (*echo.Echo, *httptest.ResponseRecorder, *http.Request) {
	e := echo.New()
	e.Use(auth.JWTMiddleware(secret, func(echo.Context) bool { return false }))
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	return e, rec, req
}

func signJWTForTest(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	now := time.Now().UTC()
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = now.Unix()
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = now.Add(time.Hour).Unix()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestOrchestrationControlIdentityRejectsScopedChatToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orchestration/runs/run-1/snapshot", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"typ":       "chat_route",
			"user_id":   "user-123",
			"tenant_id": "tenant-123",
		},
	})

	_, err := controlIdentity(c)
	if err == nil {
		t.Fatal("controlIdentity(chat_route) error = nil, want unauthorized")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("controlIdentity(chat_route) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("controlIdentity(chat_route) status = %d, want %d", httpErr.Code, http.StatusUnauthorized)
	}
	if httpErr.Message != "user token required" {
		t.Fatalf("controlIdentity(chat_route) message = %v, want %q", httpErr.Message, "user token required")
	}
}

func TestOrchestrationControlIdentityAcceptsUserToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orchestration/runs/run-1/snapshot", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"user_id":   "user-123",
			"tenant_id": "tenant-123",
		},
	})

	identity, err := controlIdentity(c)
	if err != nil {
		t.Fatalf("controlIdentity(user token) error = %v", err)
	}
	if identity != (orchestration.ControlIdentity{TenantID: "tenant-123", Subject: "user-123"}) {
		t.Fatalf("controlIdentity(user token) = %+v", identity)
	}
}

func TestOrchestrationControlIdentityRejectsServiceToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orchestration/runs/run-1/snapshot", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"typ":       "service",
			"user_id":   "user-123",
			"tenant_id": "tenant-123",
		},
	})

	_, err := controlIdentity(c)
	if err == nil {
		t.Fatal("controlIdentity(service token) error = nil, want unauthorized")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("controlIdentity(service token) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("controlIdentity(service token) status = %d, want %d", httpErr.Code, http.StatusUnauthorized)
	}
	if httpErr.Message != "user token required" {
		t.Fatalf("controlIdentity(service token) message = %v, want %q", httpErr.Message, "user token required")
	}
}

func TestOrchestrationControlIdentityRejectsQueryTokenTransport(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orchestration/runs/run-1/snapshot?token=leaky", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"user_id":   "user-123",
			"tenant_id": "tenant-123",
		},
	})

	_, err := controlIdentity(c)
	if err == nil {
		t.Fatal("controlIdentity(query token) error = nil, want unauthorized")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("controlIdentity(query token) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("controlIdentity(query token) status = %d, want %d", httpErr.Code, http.StatusUnauthorized)
	}
	if httpErr.Message != "authorization header required" {
		t.Fatalf("controlIdentity(query token) message = %v, want %q", httpErr.Message, "authorization header required")
	}
}

func TestOrchestrationControlIdentityRejectsMissingTenantID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orchestration/runs/run-1/snapshot", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"user_id": "user-123",
		},
	})

	_, err := controlIdentity(c)
	if err == nil {
		t.Fatal("controlIdentity(missing tenant) error = nil, want unauthorized")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("controlIdentity(missing tenant) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("controlIdentity(missing tenant) status = %d, want %d", httpErr.Code, http.StatusUnauthorized)
	}
}

func TestOrchestrationControlIdentityRejectsMissingUserID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/orchestration/runs/run-1/snapshot", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"tenant_id": "tenant-123",
		},
	})

	_, err := controlIdentity(c)
	if err == nil {
		t.Fatal("controlIdentity(missing user) error = nil, want unauthorized")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("controlIdentity(missing user) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("controlIdentity(missing user) status = %d, want %d", httpErr.Code, http.StatusUnauthorized)
	}
}

func TestOrchestrationRegisterIncludesCheckpointCreateRoute(t *testing.T) {
	e := echo.New()
	handler := &OrchestrationHandler{}

	handler.Register(e)

	want := "/orchestration/runs/:run_id/tasks/:task_id/checkpoints"
	for _, route := range e.Routes() {
		if route.Method == http.MethodPost && route.Path == want {
			return
		}
	}
	t.Fatalf("Register() missing route %s %s", http.MethodPost, want)
}

func TestOrchestrationRegisterIncludesRunCancelRoute(t *testing.T) {
	e := echo.New()
	handler := &OrchestrationHandler{}

	handler.Register(e)

	want := "/orchestration/runs/:run_id/cancel"
	for _, route := range e.Routes() {
		if route.Method == http.MethodPost && route.Path == want {
			return
		}
	}
	t.Fatalf("Register() missing route %s %s", http.MethodPost, want)
}

func TestOrchestrationCreateHumanCheckpointRejectsBodyPathMismatch(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/orchestration/runs/run-1/tasks/task-1/checkpoints", strings.NewReader(`{"run_id":"other-run","task_id":"task-1","question":"approve?","idempotency_key":"checkpoint-1","options":[{"id":"ok","kind":"choice","label":"OK"}]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/orchestration/runs/:run_id/tasks/:task_id/checkpoints")
	c.SetParamNames("run_id", "task_id")
	c.SetParamValues("run-1", "task-1")
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"user_id":   "user-123",
			"tenant_id": "tenant-123",
		},
	})

	handler := &OrchestrationHandler{}
	err := handler.CreateHumanCheckpoint(c)
	if err == nil {
		t.Fatal("CreateHumanCheckpoint(body/path mismatch) error = nil, want bad request")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("CreateHumanCheckpoint(body/path mismatch) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusBadRequest {
		t.Fatalf("CreateHumanCheckpoint(body/path mismatch) status = %d, want %d", httpErr.Code, http.StatusBadRequest)
	}
}

func TestOrchestrationHTTPErrorHidesInternalErrorDetails(t *testing.T) {
	handler := &OrchestrationHandler{logger: slog.New(slog.DiscardHandler)}

	err := handler.httpError(errors.New("db blew up"))
	if err == nil {
		t.Fatal("httpError(internal) error = nil")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("httpError(internal) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusInternalServerError {
		t.Fatalf("httpError(internal) status = %d, want %d", httpErr.Code, http.StatusInternalServerError)
	}
	if httpErr.Message != "internal orchestration error" {
		t.Fatalf("httpError(internal) message = %v, want %q", httpErr.Message, "internal orchestration error")
	}
}

func TestOrchestrationHTTPErrorMapsNotFound(t *testing.T) {
	handler := &OrchestrationHandler{logger: slog.New(slog.DiscardHandler)}

	err := handler.httpError(orchestration.ErrRunNotFound)
	if err == nil {
		t.Fatal("httpError(not found) error = nil")
	}

	httpErr := &echo.HTTPError{}
	if !errors.As(err, &httpErr) {
		t.Fatalf("httpError(not found) error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusNotFound {
		t.Fatalf("httpError(not found) status = %d, want %d", httpErr.Code, http.StatusNotFound)
	}
}

func TestOrchestrationGetRunSnapshotReturnsNotFoundForHiddenRun(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "")
	svc := newNoopOrchestrationService()
	svc.getRunSnapshot = func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error) {
		return nil, orchestration.ErrRunNotFound
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET snapshot hidden run status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestOrchestrationCreateCheckpointReturnsNotFoundForHiddenRun(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/runs/run-1/tasks/task-1/checkpoints", `{"question":"approve?","idempotency_key":"checkpoint-1","options":[{"id":"ok","kind":"choice","label":"OK"}]}`)
	svc := newNoopOrchestrationService()
	svc.createHumanCheckpoint = func(context.Context, orchestration.ControlIdentity, orchestration.CreateHumanCheckpointRequest) (*orchestration.CreateHumanCheckpointResult, error) {
		return nil, orchestration.ErrRunNotFound
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST checkpoint hidden run status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestOrchestrationCreateCheckpointReturnsNotFoundForHiddenTask(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/runs/run-1/tasks/task-1/checkpoints", `{"question":"approve?","idempotency_key":"checkpoint-1","options":[{"id":"ok","kind":"choice","label":"OK"}]}`)
	svc := newNoopOrchestrationService()
	svc.createHumanCheckpoint = func(context.Context, orchestration.ControlIdentity, orchestration.CreateHumanCheckpointRequest) (*orchestration.CreateHumanCheckpointResult, error) {
		return nil, orchestration.ErrTaskNotFound
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST checkpoint hidden task status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestOrchestrationGetRunSnapshotHidesInternalErrorBody(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "")
	svc := newNoopOrchestrationService()
	svc.getRunSnapshot = func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error) {
		return nil, errors.New("db connection exploded")
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET snapshot internal error status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["message"] != "internal orchestration error" {
		t.Fatalf("GET snapshot internal error body = %v, want %q", body["message"], "internal orchestration error")
	}
}

func TestOrchestrationGetRunSnapshotPassesAsOfSeq(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot?as_of_seq=42", "")
	var called bool
	svc := newNoopOrchestrationService()
	svc.getRunSnapshotAtSeq = func(_ context.Context, _ orchestration.ControlIdentity, runID string, asOfSeq uint64) (*orchestration.RunSnapshot, error) {
		called = true
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		if asOfSeq != 42 {
			t.Fatalf("asOfSeq = %d, want %d", asOfSeq, 42)
		}
		return &orchestration.RunSnapshot{SnapshotSeq: asOfSeq}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET snapshot as_of_seq status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("GetRunSnapshotAtSeq was not called")
	}
}

func TestOrchestrationStartRunReturnsCreatedHandle(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/runs", `{"goal":"ship runtime loop","idempotency_key":"run-1","input":{"k":"v"}}`)
	var called bool
	svc := newNoopOrchestrationService()
	svc.startRun = func(_ context.Context, caller orchestration.ControlIdentity, req orchestration.StartRunRequest) (orchestration.RunHandle, error) {
		called = true
		if caller != (orchestration.ControlIdentity{TenantID: "tenant-123", Subject: "user-123"}) {
			t.Fatalf("caller = %+v", caller)
		}
		if req.Goal != "ship runtime loop" {
			t.Fatalf("goal = %q, want %q", req.Goal, "ship runtime loop")
		}
		if req.IdempotencyKey != "run-1" {
			t.Fatalf("idempotency_key = %q, want %q", req.IdempotencyKey, "run-1")
		}
		return orchestration.RunHandle{RunID: "run-1", RootTaskID: "task-root", SnapshotSeq: 7}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /runs status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if !called {
		t.Fatal("StartRun was not called")
	}
	var body orchestration.RunHandle
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.RunID != "run-1" || body.RootTaskID != "task-root" || body.SnapshotSeq != 7 {
		t.Fatalf("response = %+v", body)
	}
}

func TestOrchestrationListBotRunsPassesPathAndLimit(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/bots/bot-1/runs?limit=25", "")
	svc := newNoopOrchestrationService()
	called := false
	svc.listBotRuns = func(_ context.Context, caller orchestration.ControlIdentity, botID string, req orchestration.ListBotRunsRequest) (*orchestration.RunListPage, error) {
		called = true
		if caller != (orchestration.ControlIdentity{TenantID: "tenant-123", Subject: "user-123"}) {
			t.Fatalf("caller = %+v", caller)
		}
		if botID != "bot-1" {
			t.Fatalf("botID = %q, want %q", botID, "bot-1")
		}
		if req.Limit != 25 {
			t.Fatalf("limit = %d, want %d", req.Limit, 25)
		}
		return &orchestration.RunListPage{
			Items: []orchestration.RunListItem{{ID: "run-1", Goal: "inspect run"}},
		}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if !called {
		t.Fatal("ListBotRuns service was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bot runs status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestOrchestrationGetRunInspectorReturnsPayload(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/inspector", "")
	svc := newNoopOrchestrationService()
	svc.getRunInspector = func(_ context.Context, caller orchestration.ControlIdentity, runID string) (*orchestration.RunInspector, error) {
		if caller != (orchestration.ControlIdentity{TenantID: "tenant-123", Subject: "user-123"}) {
			t.Fatalf("caller = %+v", caller)
		}
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		return &orchestration.RunInspector{
			Run: orchestration.Run{ID: "run-1", Goal: "inspect run"},
		}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET inspector status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body orchestration.RunInspector
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Run.ID != "run-1" {
		t.Fatalf("body.run.id = %q, want %q", body.Run.ID, "run-1")
	}
}

func TestOrchestrationCancelRunReturnsResult(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/runs/run-1/cancel", `{"idempotency_key":"cancel-1"}`)
	var called bool
	svc := newNoopOrchestrationService()
	svc.cancelRun = func(_ context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error) {
		called = true
		if caller != (orchestration.ControlIdentity{TenantID: "tenant-123", Subject: "user-123"}) {
			t.Fatalf("caller = %+v", caller)
		}
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		if req.IdempotencyKey != "cancel-1" {
			t.Fatalf("idempotency_key = %q, want %q", req.IdempotencyKey, "cancel-1")
		}
		return &orchestration.CancelRunResult{
			RunID:           runID,
			LifecycleStatus: orchestration.LifecycleStatusCancelling,
			SnapshotSeq:     11,
		}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST cancel status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("CancelRun was not called")
	}
}

func TestOrchestrationStartRunMapsIdempotencyConflicts(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{name: "conflict", err: orchestration.ErrIdempotencyConflict},
		{name: "incomplete", err: orchestration.ErrIdempotencyIncomplete},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/runs", `{"goal":"ship runtime loop","idempotency_key":"run-1"}`)
			svc := newNoopOrchestrationService()
			svc.startRun = func(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error) {
				return orchestration.RunHandle{}, tc.err
			}
			handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("POST /runs status = %d, want %d", rec.Code, http.StatusConflict)
			}
		})
	}
}

func TestOrchestrationListRunTasksPassesFilters(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/tasks?status=ready,%20waiting_human,,&after=%20cursor-1%20&limit=25&as_of_seq=42", "")
	var called bool
	svc := newNoopOrchestrationService()
	svc.listRunTasks = func(_ context.Context, _ orchestration.ControlIdentity, runID string, req orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error) {
		called = true
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		if got, want := req.Status, []string{"ready", "waiting_human"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("status = %#v, want %#v", got, want)
		}
		if req.After != "cursor-1" {
			t.Fatalf("after = %q, want %q", req.After, "cursor-1")
		}
		if req.Limit != 25 {
			t.Fatalf("limit = %d, want %d", req.Limit, 25)
		}
		if req.AsOfSeq != 42 {
			t.Fatalf("as_of_seq = %d, want %d", req.AsOfSeq, 42)
		}
		return &orchestration.TaskPage{SnapshotSeq: 42}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tasks status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("ListRunTasks was not called")
	}
}

func TestOrchestrationListEndpointsRejectInvalidLimit(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{name: "tasks", path: "/orchestration/runs/run-1/tasks?limit=abc"},
		{name: "checkpoints", path: "/orchestration/runs/run-1/checkpoints?limit=0"},
		{name: "artifacts", path: "/orchestration/runs/run-1/artifacts?limit=-1"},
		{name: "events", path: "/orchestration/runs/run-1/events?limit=abc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, rec, req := newAuthedEcho(http.MethodGet, tc.path, "")
			handler := &OrchestrationHandler{service: newNoopOrchestrationService(), logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("GET invalid limit status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestOrchestrationListRunTasksRejectsInvalidAsOfSeq(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/tasks?as_of_seq=nope", "")
	handler := &OrchestrationHandler{service: newNoopOrchestrationService(), logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /tasks invalid as_of_seq status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOrchestrationListRunCheckpointsPassesFilters(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/checkpoints?status=open,%20resolved&after=%20cursor-2%20&limit=15&as_of_seq=24", "")
	var called bool
	svc := newNoopOrchestrationService()
	svc.listRunCheckpoints = func(_ context.Context, _ orchestration.ControlIdentity, runID string, req orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error) {
		called = true
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		if got, want := req.Status, []string{"open", "resolved"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("status = %#v, want %#v", got, want)
		}
		if req.After != "cursor-2" || req.Limit != 15 || req.AsOfSeq != 24 {
			t.Fatalf("req = %+v", req)
		}
		return &orchestration.HumanCheckpointPage{SnapshotSeq: 24}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /checkpoints status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("ListRunCheckpoints was not called")
	}
}

func TestOrchestrationListRunArtifactsPassesFilters(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/artifacts?task_id=%20task-1%20&kind=file,%20report&after=%20cursor-3%20&limit=8&as_of_seq=31", "")
	var called bool
	svc := newNoopOrchestrationService()
	svc.listRunArtifacts = func(_ context.Context, _ orchestration.ControlIdentity, runID string, req orchestration.ListRunArtifactsRequest) (*orchestration.ArtifactPage, error) {
		called = true
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		if req.TaskID != "task-1" {
			t.Fatalf("task_id = %q, want %q", req.TaskID, "task-1")
		}
		if got, want := req.Kind, []string{"file", "report"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("kind = %#v, want %#v", got, want)
		}
		if req.After != "cursor-3" || req.Limit != 8 || req.AsOfSeq != 31 {
			t.Fatalf("req = %+v", req)
		}
		return &orchestration.ArtifactPage{SnapshotSeq: 31}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /artifacts status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("ListRunArtifacts was not called")
	}
}

func TestOrchestrationListRunEventsPassesFilters(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/events?after_seq=10&until_seq=20&limit=12", "")
	var called bool
	svc := newNoopOrchestrationService()
	svc.listRunEvents = func(_ context.Context, _ orchestration.ControlIdentity, runID string, req orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error) {
		called = true
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		if req.AfterSeq != 10 || req.UntilSeq != 20 || req.Limit != 12 {
			t.Fatalf("req = %+v", req)
		}
		return &orchestration.RunEventPage{UntilSeq: 20}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /events status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("ListRunEvents was not called")
	}
}

func TestOrchestrationListRunCheckpointsMapsInvalidCursor(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/checkpoints?after=bad-cursor", "")
	svc := newNoopOrchestrationService()
	svc.listRunCheckpoints = func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error) {
		return nil, orchestration.ErrInvalidCursor
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /checkpoints invalid cursor status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOrchestrationListRunArtifactsMapsInvalidCursor(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/artifacts?after=bad-cursor", "")
	svc := newNoopOrchestrationService()
	svc.listRunArtifacts = func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunArtifactsRequest) (*orchestration.ArtifactPage, error) {
		return nil, orchestration.ErrInvalidCursor
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /artifacts invalid cursor status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOrchestrationListRunEventsRejectsInvalidSeqBounds(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{name: "invalid after_seq", path: "/orchestration/runs/run-1/events?after_seq=nope"},
		{name: "invalid until_seq", path: "/orchestration/runs/run-1/events?until_seq=nope"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, rec, req := newAuthedEcho(http.MethodGet, tc.path, "")
			handler := &OrchestrationHandler{service: newNoopOrchestrationService(), logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("GET /events invalid seq status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestOrchestrationListRunEventsMapsInvalidArgument(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodGet, "/orchestration/runs/run-1/events?after_seq=20&until_seq=10", "")
	svc := newNoopOrchestrationService()
	svc.listRunEvents = func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error) {
		return nil, orchestration.ErrInvalidArgument
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /events invalid bounds status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOrchestrationResolveCheckpointPassesResolution(t *testing.T) {
	e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/checkpoints/checkpoint-1/resolve", `{"mode":"select_option","option_id":"approve","idempotency_key":"resolve-1","metadata":{"approved":true}}`)
	var called bool
	svc := newNoopOrchestrationService()
	svc.resolveCheckpoint = func(_ context.Context, _ orchestration.ControlIdentity, checkpointID string, req orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error) {
		called = true
		if checkpointID != "checkpoint-1" {
			t.Fatalf("checkpointID = %q, want %q", checkpointID, "checkpoint-1")
		}
		if req.Mode != "select_option" || req.OptionID != "approve" || req.IdempotencyKey != "resolve-1" {
			t.Fatalf("req = %+v", req)
		}
		if approved, ok := req.Metadata["approved"].(bool); !ok || !approved {
			t.Fatalf("metadata = %#v, want approved=true", req.Metadata)
		}
		return &orchestration.ResolveCheckpointResult{CheckpointID: checkpointID, SnapshotSeq: 55}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /resolve status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("ResolveCheckpoint was not called")
	}
}

func TestOrchestrationResolveCheckpointMapsErrorContract(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want int
	}{
		{name: "hidden", err: orchestration.ErrCheckpointNotFound, want: http.StatusNotFound},
		{name: "not-open", err: orchestration.ErrCheckpointNotOpen, want: http.StatusConflict},
		{name: "invalid-resolution", err: orchestration.ErrInvalidCheckpointResolution, want: http.StatusBadRequest},
		{name: "internal", err: errors.New("write failed"), want: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, rec, req := newAuthedEcho(http.MethodPost, "/orchestration/checkpoints/checkpoint-1/resolve", `{"mode":"select_option","option_id":"approve","idempotency_key":"resolve-1"}`)
			svc := newNoopOrchestrationService()
			svc.resolveCheckpoint = func(context.Context, orchestration.ControlIdentity, string, orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error) {
				return nil, tc.err
			}
			handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("POST /resolve status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusInternalServerError {
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body["message"] != "internal orchestration error" {
					t.Fatalf("internal body message = %v", body["message"])
				}
			}
		})
	}
}

func TestOrchestrationJWTMiddlewareRejectsQueryTokenOnSnapshot(t *testing.T) {
	const secret = "test-secret"
	token := signJWTForTest(t, secret, jwt.MapClaims{
		"user_id":   "user-123",
		"tenant_id": "tenant-123",
	})
	e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot?token="+token, "", secret)
	handler := &OrchestrationHandler{service: newNoopOrchestrationService(), logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET snapshot query token status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOrchestrationJWTMiddlewareRejectsMissingOrMalformedBearerOnSnapshot(t *testing.T) {
	testCases := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "missing authorization", header: "", wantStatus: http.StatusUnauthorized},
		{name: "malformed bearer", header: "Bearer", wantStatus: http.StatusUnauthorized},
		{name: "non-jwt bearer", header: "Bearer not-a-jwt", wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const secret = "test-secret"
			e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "", secret)
			if tc.header != "" {
				req.Header.Set(echo.HeaderAuthorization, tc.header)
			}
			called := false
			svc := newNoopOrchestrationService()
			svc.getRunSnapshot = func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error) {
				called = true
				return &orchestration.RunSnapshot{SnapshotSeq: 1}, nil
			}
			handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("GET snapshot status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if called {
				t.Fatal("service should not be called for rejected bearer input")
			}
		})
	}
}

func TestOrchestrationJWTMiddlewareRejectsScopedTokensOnSnapshot(t *testing.T) {
	testCases := []struct {
		name   string
		claims jwt.MapClaims
	}{
		{
			name: "chat-route",
			claims: jwt.MapClaims{
				"typ":       "chat_route",
				"user_id":   "user-123",
				"tenant_id": "tenant-123",
			},
		},
		{
			name: "service",
			claims: jwt.MapClaims{
				"typ":       "service",
				"user_id":   "user-123",
				"tenant_id": "tenant-123",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const secret = "test-secret"
			token := signJWTForTest(t, secret, tc.claims)
			e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "", secret)
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
			handler := &OrchestrationHandler{service: newNoopOrchestrationService(), logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("GET snapshot status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestOrchestrationJWTMiddlewareRejectsMissingTenantClaimOnSnapshot(t *testing.T) {
	const secret = "test-secret"
	token := signJWTForTest(t, secret, jwt.MapClaims{
		"user_id": "user-123",
	})
	e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "", secret)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	handler := &OrchestrationHandler{service: newNoopOrchestrationService(), logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET snapshot missing tenant status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOrchestrationJWTMiddlewareRejectsExpiredOrWronglySignedTokenOnSnapshot(t *testing.T) {
	testCases := []struct {
		name   string
		token  string
		status int
	}{
		{
			name: "expired",
			token: signJWTForTest(t, "test-secret", jwt.MapClaims{
				"user_id":   "user-123",
				"tenant_id": "tenant-123",
				"exp":       time.Now().UTC().Add(-time.Minute).Unix(),
			}),
			status: http.StatusUnauthorized,
		},
		{
			name: "wrong signature",
			token: signJWTForTest(t, "other-secret", jwt.MapClaims{
				"user_id":   "user-123",
				"tenant_id": "tenant-123",
			}),
			status: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const secret = "test-secret"
			e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "", secret)
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+tc.token)
			called := false
			svc := newNoopOrchestrationService()
			svc.getRunSnapshot = func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error) {
				called = true
				return &orchestration.RunSnapshot{SnapshotSeq: 1}, nil
			}
			handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("GET snapshot status = %d, want %d", rec.Code, tc.status)
			}
			if called {
				t.Fatal("service should not be called for rejected token")
			}
		})
	}
}

func TestOrchestrationJWTMiddlewarePassesTenantAndSubjectToService(t *testing.T) {
	const secret = "test-secret"
	token := signJWTForTest(t, secret, jwt.MapClaims{
		"user_id":   "user-123",
		"tenant_id": "tenant-123",
	})
	e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "", secret)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	var called bool
	svc := newNoopOrchestrationService()
	svc.getRunSnapshot = func(_ context.Context, caller orchestration.ControlIdentity, runID string) (*orchestration.RunSnapshot, error) {
		called = true
		if caller != (orchestration.ControlIdentity{TenantID: "tenant-123", Subject: "user-123"}) {
			t.Fatalf("caller = %+v", caller)
		}
		if runID != "run-1" {
			t.Fatalf("runID = %q, want %q", runID, "run-1")
		}
		return &orchestration.RunSnapshot{SnapshotSeq: 9}, nil
	}
	handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
	handler.Register(e)

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET snapshot status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("GetRunSnapshot was not called")
	}
}

func TestOrchestrationJWTMiddlewareAllowsHiddenRunAsNotFound(t *testing.T) {
	const secret = "test-secret"
	testCases := []struct {
		name   string
		claims jwt.MapClaims
	}{
		{
			name: "cross-tenant same-subject",
			claims: jwt.MapClaims{
				"user_id":   "user-123",
				"tenant_id": "tenant-other",
			},
		},
		{
			name: "same-tenant non-owner",
			claims: jwt.MapClaims{
				"user_id":   "user-other",
				"tenant_id": "tenant-123",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token := signJWTForTest(t, secret, tc.claims)
			e, rec, req := newJWTProtectedEcho(http.MethodGet, "/orchestration/runs/run-1/snapshot", "", secret)
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
			svc := newNoopOrchestrationService()
			svc.getRunSnapshot = func(_ context.Context, caller orchestration.ControlIdentity, _ string) (*orchestration.RunSnapshot, error) {
				if caller.TenantID == "tenant-123" && caller.Subject == "user-123" {
					return &orchestration.RunSnapshot{SnapshotSeq: 1}, nil
				}
				return nil, orchestration.ErrRunNotFound
			}
			handler := &OrchestrationHandler{service: svc, logger: slog.New(slog.DiscardHandler)}
			handler.Register(e)

			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("GET snapshot status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}
