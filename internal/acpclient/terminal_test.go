package acpclient

import (
	"context"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestTerminalOutputUsesPromptToolOutputLimit(t *testing.T) {
	t.Parallel()

	large := "HEAD\n" + strings.Repeat("terminal output ", 300) + "\nTAIL"
	manager := &terminalManager{
		limit: ToolOutputLimit{MaxBytes: 512, MaxLines: 80},
		terminals: map[string]*terminal{
			"term-1": {
				output: large,
				done:   make(chan struct{}),
			},
		},
	}

	resp, err := manager.TerminalOutput(context.Background(), acp.TerminalOutputRequest{TerminalId: "term-1"})
	if err != nil {
		t.Fatalf("TerminalOutput returned error: %v", err)
	}
	if !resp.Truncated {
		t.Fatal("TerminalOutput Truncated = false, want true")
	}
	if len(resp.Output) >= len(large) {
		t.Fatalf("terminal output was not limited")
	}
	for _, want := range []string{"[memoh pruned]", "HEAD", "TAIL"} {
		if !strings.Contains(resp.Output, want) {
			t.Fatalf("terminal output missing %q:\n%s", want, resp.Output)
		}
	}
}

func TestTerminalEndEventUsesPromptToolOutputLimit(t *testing.T) {
	t.Parallel()

	large := "HEAD\n" + strings.Repeat("terminal output ", 300) + "\nTAIL"
	limit := ToolOutputLimit{MaxBytes: 512, MaxLines: 80}
	collector := newEventCollector(limit)
	emitter := &toolEventEmitter{}
	emitter.setPromptState(collector, nil, limit)
	manager := &terminalManager{
		limit:  limit,
		events: emitter,
	}
	term := &terminal{
		id:     "terminal-1",
		input:  map[string]any{"command": "test"},
		output: large,
		done:   make(chan struct{}),
	}

	manager.emitTerminalEnd(term)
	result := collector.result()
	if len(result.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(result.Events))
	}
	output, ok := result.Events[0].Result.(map[string]any)
	if !ok {
		t.Fatalf("event result = %#v, want map", result.Events[0].Result)
	}
	stdout, ok := output["stdout"].(string)
	if !ok {
		t.Fatalf("stdout = %#v, want string", output["stdout"])
	}
	if len(stdout) >= len(large) || !strings.Contains(stdout, "[memoh pruned]") {
		t.Fatalf("terminal end stdout was not limited:\n%s", stdout)
	}
	if truncated, _ := output["truncated"].(bool); !truncated {
		t.Fatalf("terminal end truncated = %#v, want true", output["truncated"])
	}
}
