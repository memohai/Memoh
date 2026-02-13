package flow

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
)

func TestPostTriggerSchedule_Endpoint(t *testing.T) {
	var capturedPath string
	var capturedBody []byte
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)
		resp := gatewayResponse{
			Messages: []conversation.ModelMessage{{Role: "assistant", Content: conversation.NewTextContent("ok")}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	resolver := &Resolver{
		gatewayBaseURL: srv.URL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		logger:         slog.Default(),
	}

	maxCalls := 5
	req := triggerScheduleRequest{
		gatewayRequest: gatewayRequest{
			Model: gatewayModelConfig{
				ModelID:    "gpt-4",
				ClientType: "openai",
				APIKey:     "sk-test",
				BaseURL:    "https://api.openai.com",
			},
			ActiveContextTime: 1440,
			Channels:          []string{},
			Messages:          []conversation.ModelMessage{},
			Skills:            []string{},
			Identity: gatewayIdentity{
				BotID:             "bot-123",
				ContainerID:       "mcp-bot-123",
				ChannelIdentityID: "owner-user-1",
				DisplayName:       "Scheduler",
			},
			Attachments: []any{},
		},
		Schedule: gatewaySchedule{
			ID:          "sched-1",
			Name:        "daily report",
			Description: "generate daily report",
			Pattern:     "0 9 * * *",
			MaxCalls:    &maxCalls,
			Command:     "generate the daily report",
		},
	}

	resp, err := resolver.postTriggerSchedule(context.Background(), req, "Bearer test-token")
	if err != nil {
		t.Fatalf("postTriggerSchedule returned error: %v", err)
	}

	if capturedPath != "/chat/trigger-schedule" {
		t.Errorf("expected path /chat/trigger-schedule, got %s", capturedPath)
	}
	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Authorization header 'Bearer test-token', got %s", capturedAuth)
	}
	if len(resp.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(resp.Messages))
	}

	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}
	schedule, ok := body["schedule"].(map[string]any)
	if !ok {
		t.Fatal("expected 'schedule' field in request body")
	}
	if schedule["id"] != "sched-1" {
		t.Errorf("expected schedule.id=sched-1, got %v", schedule["id"])
	}
	if schedule["command"] != "generate the daily report" {
		t.Errorf("expected schedule.command, got %v", schedule["command"])
	}
	if _, hasQuery := body["query"]; hasQuery {
		t.Error("trigger-schedule request should not contain 'query' field")
	}
}

func TestPostTriggerSchedule_NoAuth(t *testing.T) {
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		resp := gatewayResponse{Messages: []conversation.ModelMessage{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	resolver := &Resolver{
		gatewayBaseURL: srv.URL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		logger:         slog.Default(),
	}

	req := triggerScheduleRequest{
		gatewayRequest: gatewayRequest{
			Channels:    []string{},
			Messages:    []conversation.ModelMessage{},
			Skills:      []string{},
			Attachments: []any{},
		},
		Schedule: gatewaySchedule{ID: "s1", Command: "test"},
	}

	_, err := resolver.postTriggerSchedule(context.Background(), req, "")
	if err != nil {
		t.Fatalf("postTriggerSchedule returned error: %v", err)
	}
	if capturedAuth != "" {
		t.Errorf("expected no Authorization header, got %s", capturedAuth)
	}
}

func TestPostTriggerSchedule_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	resolver := &Resolver{
		gatewayBaseURL: srv.URL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		logger:         slog.Default(),
	}

	req := triggerScheduleRequest{
		gatewayRequest: gatewayRequest{
			Channels:    []string{},
			Messages:    []conversation.ModelMessage{},
			Skills:      []string{},
			Attachments: []any{},
		},
		Schedule: gatewaySchedule{ID: "s1", Command: "test"},
	}

	_, err := resolver.postTriggerSchedule(context.Background(), req, "Bearer tok")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
