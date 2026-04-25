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

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/orchestration"
)

type fakeOrchestrationService struct {
	startRun              func(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error)
	getRunSnapshot        func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error)
	getRunSnapshotAtSeq   func(context.Context, orchestration.ControlIdentity, string, uint64) (*orchestration.RunSnapshot, error)
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

func (f fakeOrchestrationService) GetRunSnapshot(ctx context.Context, caller orchestration.ControlIdentity, runID string) (*orchestration.RunSnapshot, error) {
	return f.getRunSnapshot(ctx, caller, runID)
}

func (f fakeOrchestrationService) GetRunSnapshotAtSeq(ctx context.Context, caller orchestration.ControlIdentity, runID string, asOfSeq uint64) (*orchestration.RunSnapshot, error) {
	return f.getRunSnapshotAtSeq(ctx, caller, runID, asOfSeq)
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
		getRunSnapshot: func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error) {
			return &orchestration.RunSnapshot{}, nil
		},
		getRunSnapshotAtSeq: func(context.Context, orchestration.ControlIdentity, string, uint64) (*orchestration.RunSnapshot, error) {
			return &orchestration.RunSnapshot{}, nil
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
