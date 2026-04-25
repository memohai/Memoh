package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/orchestration"
)

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
