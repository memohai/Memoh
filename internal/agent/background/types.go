package background

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskStatus represents the lifecycle state of a background task.
type TaskStatus string

const (
	TaskQueued    TaskStatus = "queued"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskKilled    TaskStatus = "killed"
)

// Task represents a single background task (a container command execution
// or a spawn subagent batch, per Kind).
type Task struct {
	ID             string
	Kind           TaskKind
	BotID          string
	SessionID      string
	Command        string
	Description    string
	AgentID        string
	AgentSessionID string
	AgentMessage   string
	AgentReport    string
	AgentError     string
	WorkDir        string
	Status         TaskStatus
	ExitCode       int32
	OutputFile     string // path inside container where output is being written
	StartedAt      time.Time
	CompletedAt    time.Time

	mu              sync.Mutex
	cancel          context.CancelFunc
	notified        bool            // true once a terminal notification has been enqueued; prevents duplicates
	stalledNotified bool            // true once a stalled notification has been enqueued
	output          strings.Builder // buffered output tail
	branches        []SpawnBranch   // spawn-kind branch outcomes, set at completion
}

// TaskSnapshot is a lock-safe, immutable view of a task for handler/UI code.
type TaskSnapshot struct {
	TaskID         string
	Kind           TaskKind
	BotID          string
	SessionID      string
	Command        string
	Description    string
	AgentID        string
	AgentSessionID string
	AgentMessage   string
	AgentReport    string
	AgentError     string
	WorkDir        string
	Status         TaskStatus
	ExitCode       int32
	OutputFile     string
	OutputTail     string
	Branches       []SpawnBranch
	StartedAt      time.Time
	CompletedAt    time.Time
	Duration       time.Duration
	Stalled        bool
}

// Snapshot returns a consistent view of the task without exposing its mutex.
func (t *Task) Snapshot() TaskSnapshot {
	if t == nil {
		return TaskSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	duration := time.Since(t.StartedAt)
	if !t.CompletedAt.IsZero() {
		duration = t.CompletedAt.Sub(t.StartedAt)
	}
	return TaskSnapshot{
		TaskID:         t.ID,
		Kind:           t.Kind,
		BotID:          t.BotID,
		SessionID:      t.SessionID,
		Command:        t.Command,
		Description:    t.Description,
		AgentID:        t.AgentID,
		AgentSessionID: t.AgentSessionID,
		AgentMessage:   t.AgentMessage,
		AgentReport:    t.AgentReport,
		AgentError:     t.AgentError,
		WorkDir:        t.WorkDir,
		Status:         t.Status,
		ExitCode:       t.ExitCode,
		OutputFile:     t.OutputFile,
		OutputTail:     t.outputTailLocked(),
		Branches:       append([]SpawnBranch(nil), t.branches...),
		StartedAt:      t.StartedAt,
		CompletedAt:    t.CompletedAt,
		Duration:       duration,
		Stalled:        t.stalledNotified && t.Status == TaskRunning,
	}
}

// MarkNotified atomically sets the notified flag. Returns true if this call
// was the one that flipped it (i.e., the caller should enqueue the notification).
func (t *Task) MarkNotified() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.notified {
		return false
	}
	t.notified = true
	return true
}

// MarkStalledNotified atomically sets the stalled notification flag.
func (t *Task) MarkStalledNotified() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stalledNotified {
		return false
	}
	t.stalledNotified = true
	return true
}

// Cancel requests cancellation of the task's context.
func (t *Task) Cancel() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
	}
}

// AppendOutput appends text to the buffered output tail.
// Only the last maxTailBytes are kept.
func (t *Task) AppendOutput(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output.WriteString(s)
	// Keep tail bounded
	if t.output.Len() > maxTailBytes*2 {
		tail := t.output.String()
		t.output.Reset()
		if len(tail) > maxTailBytes {
			t.output.WriteString(tail[len(tail)-maxTailBytes:])
		} else {
			t.output.WriteString(tail)
		}
	}
}

// OutputTail returns the last portion of collected output.
func (t *Task) OutputTail() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.outputTailLocked()
}

func (t *Task) outputTailLocked() string {
	s := t.output.String()
	if len(s) > maxTailBytes {
		return s[len(s)-maxTailBytes:]
	}
	return s
}

const maxTailBytes = 4096

// AdoptResult carries the outcome of a command whose execution was started
// externally (e.g. via ExecStream) and then handed off to the Manager.
//
// ExitReceived distinguishes "the bridge actually sent us an EXIT frame"
// (so ExitCode is the real value the process returned, even if the gRPC
// stream errored out afterwards) from "we never saw an EXIT frame, ExitCode
// is just its zero value". Without this flag, downstream code can't tell
// "the command finished with exit 0 right before the stream died" from
// "we have no idea what the exit code was".
type AdoptResult struct {
	Stdout         string
	Stderr         string
	ExitCode       int32
	ExitReceived   bool
	Err            error
	OutputRecorded bool
}

// Notification is the structured event sent to the agent when a background
// task reaches a terminal state or requires attention (e.g. stalled).
type Notification struct {
	TaskID         string
	Kind           TaskKind
	BotID          string
	SessionID      string
	Status         TaskStatus
	Command        string
	Description    string
	AgentID        string
	AgentSessionID string
	AgentMessage   string
	AgentReport    string
	AgentError     string
	ExitCode       int32
	OutputFile     string
	OutputTail     string // last N bytes of output for quick summary
	Branches       []SpawnBranch
	Duration       time.Duration
	Stalled        bool // true when task appears stuck on interactive input
}

// TaskEventType identifies a UI-facing background task event.
type TaskEventType string

const (
	TaskEventQueued    TaskEventType = "queued"
	TaskEventStarted   TaskEventType = "started"
	TaskEventOutput    TaskEventType = "output"
	TaskEventCompleted TaskEventType = "completed"
	TaskEventFailed    TaskEventType = "failed"
	TaskEventStalled   TaskEventType = "stalled"
)

// TaskEvent is emitted for live UI updates. Output events are intentionally
// lightweight and non-persistent; completion notifications remain the source of
// truth for agent wakeups and history.
type TaskEvent struct {
	Event          TaskEventType `json:"event"`
	TaskID         string        `json:"task_id"`
	Kind           TaskKind      `json:"kind,omitempty"`
	BotID          string        `json:"bot_id,omitempty"`
	SessionID      string        `json:"session_id,omitempty"`
	Command        string        `json:"command,omitempty"`
	AgentID        string        `json:"agent_id,omitempty"`
	AgentSessionID string        `json:"agent_session_id,omitempty"`
	Status         TaskStatus    `json:"status,omitempty"`
	Stream         string        `json:"stream,omitempty"`
	Chunk          string        `json:"chunk,omitempty"`
	Tail           string        `json:"tail,omitempty"`
	OutputFile     string        `json:"output_file,omitempty"`
	ExitCode       int32         `json:"exit_code,omitempty"`
	Duration       string        `json:"duration,omitempty"`
	Stalled        bool          `json:"stalled,omitempty"`
}

// MessageText returns the full user-message text that should be injected into
// the agent's message stream — a human lead-in line followed by the
// <task-notification> block.
func (n Notification) MessageText() string {
	lead := "A background task completed:"
	if n.Stalled {
		lead = "A background task appears stuck and may need attention:"
	}
	return lead + "\n" + n.FormatForAgent()
}

// FormatForAgent returns a human-readable task-notification block that can be
// injected into the agent's message stream.
func (n Notification) FormatForAgent() string {
	if n.Kind == KindAgent {
		return n.formatAgentForAgent()
	}
	if n.Kind == KindSpawn {
		return n.formatSpawnForAgent()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<task-notification>\n")
	fmt.Fprintf(&b, "  <task-id>%s</task-id>\n", n.TaskID)
	if n.Stalled {
		fmt.Fprintf(&b, "  <status>stalled</status>\n")
	} else {
		fmt.Fprintf(&b, "  <status>%s</status>\n", n.Status)
	}
	fmt.Fprintf(&b, "  <command>%s</command>\n", n.Command)
	if n.Description != "" {
		fmt.Fprintf(&b, "  <description>%s</description>\n", n.Description)
	}
	if !n.Stalled {
		fmt.Fprintf(&b, "  <exit-code>%d</exit-code>\n", n.ExitCode)
	}
	fmt.Fprintf(&b, "  <duration>%s</duration>\n", n.Duration.Round(time.Millisecond))
	if n.OutputFile != "" {
		fmt.Fprintf(&b, "  <output-file>%s</output-file>\n", n.OutputFile)
	}
	if n.OutputTail != "" {
		fmt.Fprintf(&b, "  <output-tail>\n%s\n  </output-tail>\n", strings.TrimRight(n.OutputTail, "\n"))
	}
	if n.Stalled {
		fmt.Fprintf(&b, "  <suggestion>This command appears to be waiting for interactive input. Kill it with an available background task tool and retry with a non-interactive flag (e.g. -y, --yes, --non-interactive).</suggestion>\n")
	}
	fmt.Fprintf(&b, "</task-notification>")
	return b.String()
}
