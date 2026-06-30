package conversation

import (
	"testing"
)

func TestApplyToolResultRecognizesBackgroundStartByShape(t *testing.T) {
	tests := []struct {
		name           string
		toolName       string
		output         map[string]any
		wantTaskID     string
		wantCommand    string
		wantOutputFile string
	}{
		{
			name:     "spawn",
			toolName: "spawn",
			output: map[string]any{
				"status":      "background_started",
				"task_id":     "bg_bot1_aaaa",
				"kind":        "spawn",
				"task_count":  2,
				"description": "spawn 2 task(s): alpha | beta",
				"message":     "2 subagent task(s) started in background",
			},
			wantTaskID:  "bg_bot1_aaaa",
			wantCommand: "spawn 2 task(s): alpha | beta",
		},
		{
			name:     "exec",
			toolName: "exec",
			output: map[string]any{
				"status":      "auto_backgrounded",
				"task_id":     "bg_bot1_bbbb",
				"output_file": "/tmp/memoh-bg/bg_bot1_bbbb.log",
			},
			wantTaskID:     "bg_bot1_bbbb",
			wantOutputFile: "/tmp/memoh-bg/bg_bot1_bbbb.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &UIMessage{Type: UIMessageTool, Name: tt.toolName}
			applyToolResultToUIMessage(msg, tt.output)

			if msg.Background == nil {
				t.Fatal("expected background task recognized from tool result")
			}
			if msg.Background.TaskID != tt.wantTaskID || msg.Background.Status != "running" {
				t.Errorf("unexpected background state: %+v", msg.Background)
			}
			if msg.Background.Command != tt.wantCommand {
				t.Errorf("command = %q, want %q", msg.Background.Command, tt.wantCommand)
			}
			if msg.Background.OutputFile != tt.wantOutputFile {
				t.Errorf("output file = %q, want %q", msg.Background.OutputFile, tt.wantOutputFile)
			}
			if msg.Running == nil || !*msg.Running {
				t.Error("expected tool to be marked running")
			}
		})
	}
}

func TestApplyToolResultIgnoresTerminalTaskStatusPayloads(t *testing.T) {
	// Background status inspection results carry task_id with a terminal status;
	// they must not turn the tool card into a running background task.
	msg := &UIMessage{Type: UIMessageTool, Name: "get_background_status"}
	applyToolResultToUIMessage(msg, map[string]any{
		"task_id": "bg_bot1_cccc",
		"kind":    "exec",
		"status":  "completed",
	})

	if msg.Background != nil {
		t.Fatalf("expected no background task for terminal status payload, got %+v", msg.Background)
	}
	if msg.Running == nil || *msg.Running {
		t.Error("expected tool to be marked not running")
	}
}
