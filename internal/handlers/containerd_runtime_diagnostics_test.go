package handlers

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type recordingRuntimeDiagnosticEvents struct {
	events []recordedRuntimeDiagnosticEvent
}

type recordedRuntimeDiagnosticEvent struct {
	botID    string
	scope    string
	phase    string
	severity string
	code     string
	message  string
	metadata map[string]any
}

func (r *recordingRuntimeDiagnosticEvents) RecordRuntimeDiagnosticEvent(ctx context.Context, botID, scope, _ string, _ string, _ string, phase, severity, code, message string, metadata map[string]any) {
	_ = ctx
	r.events = append(r.events, recordedRuntimeDiagnosticEvent{
		botID:    botID,
		scope:    scope,
		phase:    phase,
		severity: severity,
		code:     code,
		message:  message,
		metadata: metadata,
	})
}

func TestContainerdHandlerRecordsRuntimeDiagnosticFailures(t *testing.T) {
	recorder := &recordingRuntimeDiagnosticEvents{}
	handler := &ContainerdHandler{logger: slog.Default()}
	handler.SetRuntimeDiagnosticRecorder(recorder)

	handler.recordRuntimeDiagnosticFailure(context.Background(), "bot-1", "container", "start", "container_start_failed", errors.New("containerd failed"), map[string]any{
		"runtime_backend": "containerd",
	})
	handler.recordRuntimeDiagnosticFailure(context.Background(), "bot-1", "display", "prepare", "display_prepare_failed", errors.New("xvnc failed"), map[string]any{
		"step": "starting",
	})

	if len(recorder.events) != 2 {
		t.Fatalf("recorded events = %#v, want 2 events", recorder.events)
	}
	if recorder.events[0].botID != "bot-1" || recorder.events[0].scope != "container" ||
		recorder.events[0].phase != "start" || recorder.events[0].severity != "error" ||
		recorder.events[0].code != "container_start_failed" || recorder.events[0].message != "containerd failed" {
		t.Fatalf("container event = %#v", recorder.events[0])
	}
	if recorder.events[1].scope != "display" || recorder.events[1].phase != "prepare" ||
		recorder.events[1].code != "display_prepare_failed" || recorder.events[1].metadata["step"] != "starting" {
		t.Fatalf("display event = %#v", recorder.events[1])
	}
}
