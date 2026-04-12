// Package background implements a background task manager for long-running
// commands executed inside bot containers. It follows a task-notification
// architecture:
//
//  1. Commands can be started in the background (fire-and-forget).
//  2. Output is collected asynchronously and written to a file in the container.
//  3. When a task completes, a structured Notification is enqueued.
//  4. Notifications are scoped to (botID, sessionID); the agent loop drains
//     them at step boundaries and injects them as context messages so the
//     model learns about completed work.
//
// The manager is a server-level singleton, safe for concurrent use.
package background

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	// DefaultExecTimeout is the default timeout for foreground exec calls.
	DefaultExecTimeout int32 = 30
	// MaxExecTimeout is the maximum allowed timeout (10 minutes).
	MaxExecTimeout int32 = 600
	// BackgroundExecTimeout is the timeout for background tasks (30 minutes).
	BackgroundExecTimeout int32 = 1800
	// DefaultCleanupInterval is how often the manager prunes old completed tasks.
	DefaultCleanupInterval = time.Hour
	// DefaultTaskRetention is how long completed tasks are retained in memory.
	DefaultTaskRetention = 24 * time.Hour
	// OutputLogDir is the directory inside the container where background
	// task output logs are written.
	OutputLogDir = "/tmp/memoh-bg"

	// stallCheckInterval is how often the stall watchdog checks output growth.
	stallCheckInterval = 5 * time.Second
	// stallThreshold is the duration of zero output growth before we consider
	// the command stalled and possibly waiting for interactive input.
	stallThreshold = 45 * time.Second
)

// ExecFunc executes a command in a container and returns the result.
// This is the signature that bridge.Client.Exec satisfies.
type ExecFunc func(ctx context.Context, command, workDir string, timeout int32) (*bridge.ExecResult, error)

// WriteFileFunc writes content to a file in the container.
type WriteFileFunc func(ctx context.Context, path string, data []byte) error

// ReadFileFunc reads content from a file in the container.
type ReadFileFunc func(ctx context.Context, path string) ([]byte, error)

// Manager tracks background tasks and delivers completion notifications.
type Manager struct {
	mu            sync.Mutex
	tasks         map[string]*Task // taskID -> Task
	notifications []Notification   // pending notifications, protected by mu
	logger        *slog.Logger
	wakeFunc      func(botID, sessionID string) // optional callback to wake agent on new notification
}

// New creates a new background task Manager.
func New(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		tasks:  make(map[string]*Task),
		logger: logger.With(slog.String("service", "background")),
	}
}

// SetWakeFunc registers a callback that is invoked (in a goroutine) whenever a
// new notification is enqueued. Use this to wake up a sleeping agent so it
// can drain the notification immediately instead of waiting for user input.
func (m *Manager) SetWakeFunc(fn func(botID, sessionID string)) {
	m.mu.Lock()
	m.wakeFunc = fn
	m.mu.Unlock()
}

// enqueueNotification appends n to the pending list and, if a wake function is
// registered, calls it asynchronously so the agent can process the notification.
func (m *Manager) enqueueNotification(n Notification) {
	m.mu.Lock()
	m.notifications = append(m.notifications, n)
	wakeFn := m.wakeFunc
	m.mu.Unlock()
	m.logger.Info("notification enqueued",
		slog.String("task_id", n.TaskID),
		slog.String("bot_id", n.BotID),
		slog.Bool("has_wake_func", wakeFn != nil),
	)
	if wakeFn != nil {
		go wakeFn(n.BotID, n.SessionID)
	}
}

// Spawn starts a command in the background. It returns the task ID immediately.
// The command runs asynchronously; when it completes, a Notification is sent
// to the Notifications channel.
//
// execFn should call bridge.Client.Exec (or equivalent).
// writeFn should call bridge.Client.WriteFile to persist output logs.
func (m *Manager) Spawn(
	parentCtx context.Context,
	botID, sessionID, command, workDir, description string,
	execFn ExecFunc,
	writeFn WriteFileFunc,
	readFn ReadFileFunc,
) (taskID, outputFile string) {
	m.mu.Lock()
	taskID = m.newTaskIDLocked(botID)
	outputFile = fmt.Sprintf("%s/%s.log", OutputLogDir, taskID)

	task := &Task{
		ID:          taskID,
		BotID:       botID,
		SessionID:   sessionID,
		Command:     command,
		Description: description,
		WorkDir:     workDir,
		Status:      TaskRunning,
		OutputFile:  outputFile,
		StartedAt:   time.Now(),
	}
	m.tasks[taskID] = task
	m.mu.Unlock()

	m.logger.Info("background task spawned",
		slog.String("task_id", taskID),
		slog.String("bot_id", botID),
		slog.String("command", truncate(command, 120)),
	)

	go m.run(parentCtx, task, execFn, writeFn, readFn)
	return taskID, outputFile
}

// SpawnAdopt registers a background task for a command that is already running
// externally (e.g. via ExecStream). Instead of re-executing the command, it
// waits for the result on the provided channel. This enables "flip to background"
// where a foreground stream is handed off without killing the process.
func (m *Manager) SpawnAdopt(
	parentCtx context.Context,
	botID, sessionID, command, workDir, description string,
	resultCh <-chan AdoptResult,
	writeFn WriteFileFunc,
) (taskID, outputFile string) {
	m.mu.Lock()
	taskID = m.newTaskIDLocked(botID)
	outputFile = fmt.Sprintf("%s/%s.log", OutputLogDir, taskID)

	task := &Task{
		ID:          taskID,
		BotID:       botID,
		SessionID:   sessionID,
		Command:     command,
		Description: description,
		WorkDir:     workDir,
		Status:      TaskRunning,
		OutputFile:  outputFile,
		StartedAt:   time.Now(),
	}
	m.tasks[taskID] = task
	m.mu.Unlock()

	m.logger.Info("background task adopted",
		slog.String("task_id", taskID),
		slog.String("bot_id", botID),
		slog.String("command", truncate(command, 120)),
	)

	go m.runAdopt(parentCtx, task, resultCh, writeFn)
	return taskID, outputFile
}

func (m *Manager) newTaskIDLocked(botID string) string {
	prefix := botID[:min(8, len(botID))]
	for {
		id := fmt.Sprintf("bg_%s_%s", prefix, shortRandHex(4))
		if _, exists := m.tasks[id]; !exists {
			return id
		}
	}
}

func shortRandHex(n int) string {
	if n <= 0 {
		n = 4
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Errorf("background: read random bytes: %w", err))
	}
	return hex.EncodeToString(buf)
}

// runAdopt waits for the adopted stream result and handles completion.
func (m *Manager) runAdopt(parentCtx context.Context, task *Task, resultCh <-chan AdoptResult, writeFn WriteFileFunc) {
	ctx, cancel := detachedContextWithTimeout(parentCtx, time.Duration(BackgroundExecTimeout)*time.Second)
	task.mu.Lock()
	task.cancel = cancel
	task.mu.Unlock()
	defer cancel()

	// Ensure output directory exists.
	_ = ensureOutputDir(ctx, writeFn)

	// Start stall watchdog.
	go m.stallWatchdog(ctx, task)

	// Wait for the result from the already-running stream.
	var result AdoptResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		result = AdoptResult{Err: ctx.Err()}
	}

	// Write output to log file in container.
	if writeFn != nil && result.Err == nil {
		combined := result.Stdout
		if result.Stderr != "" {
			combined += "\n--- stderr ---\n" + result.Stderr
		}
		_ = writeFn(context.WithoutCancel(ctx), task.OutputFile, []byte(combined))
	}

	m.completeTask(task, result.Stdout, result.Stderr, result.Err, result.ExitCode)
}

func (m *Manager) run(parentCtx context.Context, task *Task, execFn ExecFunc, writeFn WriteFileFunc, readFn ReadFileFunc) {
	ctx, cancel := detachedContextWithTimeout(parentCtx, time.Duration(BackgroundExecTimeout)*time.Second)
	task.mu.Lock()
	task.cancel = cancel
	task.mu.Unlock()
	defer cancel()

	// Ensure output directory exists.
	_ = ensureOutputDir(ctx, writeFn)

	// Start stall watchdog to detect commands waiting for interactive input.
	go m.stallWatchdog(ctx, task)

	// Wrap command to tee output to the log file inside the container and
	// capture the command exit code into a sentinel file via fd 3 redirect.
	// Even if the gRPC stream dies after process completion, we can recover
	// the actual exit code by reading the sentinel file.
	wrappedCmd := fmt.Sprintf(
		"{ { ( %s ) ; echo $? >&3 ; } 2>&1 | tee %s ; } 3>%s.exit",
		task.Command, task.OutputFile, task.OutputFile,
	)

	result, err := execFn(ctx, wrappedCmd, task.WorkDir, BackgroundExecTimeout)
	if err != nil {
		m.logger.Warn("background task: execFn returned error",
			slog.String("task_id", task.ID),
			slog.Any("exec_error", err),
		)
	}

	// Always prefer the sentinel file for the real exit code.
	// The wrappedCmd uses a pipeline: the shell exits with tee's code (0),
	// not the actual command's code. The sentinel captures the real value.
	// On gRPC error the sentinel also lets us recover without -1.
	if readFn != nil {
		ec, recoverErr := readSentinelExitCode(ctx, task.OutputFile+".exit", readFn)
		if recoverErr == nil {
			if err != nil {
				m.logger.Info("background task: recovered exit code from sentinel file after stream error",
					slog.String("task_id", task.ID),
					slog.Int("recovered_exit_code", int(ec)),
					slog.Any("stream_error", err),
				)
			}
			result = &bridge.ExecResult{ExitCode: ec}
			err = nil
		} else if err != nil {
			m.logger.Warn("background task: sentinel recovery failed",
				slog.String("task_id", task.ID),
				slog.Any("recover_error", recoverErr),
			)
		}
		// If err==nil but sentinel unreadable: fall through to use gRPC exit code
	}

	var stdout, stderr string
	var exitCode int32
	if result != nil {
		stdout = result.Stdout
		stderr = result.Stderr
		exitCode = result.ExitCode
	}
	m.completeTask(task, stdout, stderr, err, exitCode)
}

func (m *Manager) completeTask(task *Task, stdout, stderr string, execErr error, exitCode int32) {
	if execErr != nil {
		task.AppendOutput(fmt.Sprintf("[error] %v\n", execErr))
	} else {
		task.AppendOutput(stdout)
		if stderr != "" {
			task.AppendOutput(stderr)
		}
	}

	task.mu.Lock()
	if task.Status == TaskKilled {
		task.mu.Unlock()
		return
	}
	task.CompletedAt = time.Now()
	if execErr != nil {
		task.Status = TaskFailed
		task.ExitCode = -1
	} else {
		task.ExitCode = exitCode
		if exitCode == 0 {
			task.Status = TaskCompleted
		} else {
			task.Status = TaskFailed
		}
	}
	status := task.Status
	finalExitCode := task.ExitCode
	duration := task.CompletedAt.Sub(task.StartedAt)
	task.mu.Unlock()

	m.logger.Info("background task finished",
		slog.String("task_id", task.ID),
		slog.String("status", string(status)),
		slog.Int("exit_code", int(finalExitCode)),
		slog.Duration("duration", duration),
	)

	// Guard against double notification when Kill or an auto-background race
	// already enqueued one for this task.
	if !task.MarkNotified() {
		return
	}

	m.enqueueNotification(Notification{
		TaskID:      task.ID,
		BotID:       task.BotID,
		SessionID:   task.SessionID,
		Status:      status,
		Command:     task.Command,
		Description: task.Description,
		ExitCode:    finalExitCode,
		OutputFile:  task.OutputFile,
		OutputTail:  task.OutputTail(),
		Duration:    duration,
	})
}

func readSentinelExitCode(ctx context.Context, path string, readFn ReadFileFunc) (int32, error) {
	data, err := readFn(ctx, path)
	if err != nil {
		return 0, err
	}
	ec, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse exit code %q: %w", string(data), err)
	}
	return int32(ec), nil //nolint:gosec // G115: exit codes are 0-255
}

func ensureOutputDir(ctx context.Context, writeFn WriteFileFunc) error {
	if writeFn == nil {
		return nil
	}
	// Create a marker file to ensure the directory exists.
	return writeFn(ctx, OutputLogDir+"/.keep", []byte(""))
}

// Kill cancels a running background task.
func (m *Manager) Kill(taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	task.mu.Lock()
	if task.Status != TaskRunning {
		task.mu.Unlock()
		return fmt.Errorf("task %s is not running (status: %s)", taskID, task.Status)
	}
	task.Status = TaskKilled
	task.CompletedAt = time.Now()
	task.mu.Unlock()

	task.Cancel()
	m.logger.Info("background task killed", slog.String("task_id", taskID))
	return nil
}

// Get returns a task by ID, or nil if not found.
func (m *Manager) Get(taskID string) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[taskID]
}

// GetForSession returns a task by ID only if it belongs to the provided
// bot+session.
func (m *Manager) GetForSession(botID, sessionID, taskID string) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	task := m.tasks[taskID]
	if task == nil || task.BotID != botID || task.SessionID != sessionID {
		return nil
	}
	return task
}

// ListForSession returns all tasks for a given bot+session, most recent first.
func (m *Manager) ListForSession(botID, sessionID string) []*Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*Task
	for _, t := range m.tasks {
		if t.BotID == botID && t.SessionID == sessionID {
			result = append(result, t)
		}
	}
	return result
}

// KillForSession cancels a running background task only when it belongs to the
// provided bot+session.
func (m *Manager) KillForSession(botID, sessionID, taskID string) error {
	task := m.GetForSession(botID, sessionID, taskID)
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	return m.Kill(taskID)
}

// DrainNotifications returns all pending notifications for a given
// bot+session without blocking. Used by the resolver to inject
// notifications at the start of a new agent run.
func (m *Manager) DrainNotifications(botID, sessionID string) []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()

	var matched []Notification
	remaining := m.notifications[:0] // reuse backing array
	for _, n := range m.notifications {
		if n.BotID == botID && n.SessionID == sessionID {
			matched = append(matched, n)
		} else {
			remaining = append(remaining, n)
		}
	}
	m.notifications = remaining
	return matched
}

// HasNotifications reports whether there are pending notifications for the
// given bot+session without consuming them.
func (m *Manager) HasNotifications(botID, sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.notifications {
		if n.BotID == botID && n.SessionID == sessionID {
			return true
		}
	}
	return false
}

// RunningTasksSummary returns a text summary of currently running tasks
// for a given bot+session. This is injected into the system prompt so the
// agent knows about ongoing background work.
func (m *Manager) RunningTasksSummary(botID, sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var lines []string
	for _, t := range m.tasks {
		if t.BotID == botID && t.SessionID == sessionID && t.Status == TaskRunning {
			desc := t.Description
			if desc == "" {
				desc = truncate(t.Command, 80)
			}
			lines = append(lines, fmt.Sprintf("- [%s] %s (started %s ago, output: %s)",
				t.ID, desc,
				time.Since(t.StartedAt).Round(time.Second),
				t.OutputFile,
			))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Currently running background tasks:\n" + joinLines(lines)
}

// Cleanup removes completed tasks older than the given duration.
func (m *Manager) Cleanup(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, t := range m.tasks {
		if t.Status != TaskRunning && t.CompletedAt.Before(cutoff) {
			delete(m.tasks, id)
		}
	}
}

// StartCleanupLoop periodically removes old completed tasks until done is closed.
func (m *Manager) StartCleanupLoop(done <-chan struct{}, interval, maxAge time.Duration) {
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	if maxAge <= 0 {
		maxAge = DefaultTaskRetention
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.Cleanup(maxAge)
		case <-done:
			return
		}
	}
}

// RequeueNotifications puts notifications back into the pending queue.
// Used when proactive delivery for a session fails and the batch should be retried.
func (m *Manager) RequeueNotifications(ns []Notification) {
	if len(ns) == 0 {
		return
	}
	m.mu.Lock()
	m.notifications = append(m.notifications, ns...)
	m.mu.Unlock()
}

// promptPatterns matches common interactive prompt endings that indicate
// a command is waiting for user input.
var promptPatterns = regexp.MustCompile(
	`(?i)(\$ ?$|> ?$|# ?$|password\s*:|passphrase\s*:|y/n\]|yes/no\)|enter .*:|Press .* to continue|Are you sure|Continue\?|Proceed\?)`,
)

// stallWatchdog monitors a background task's output for stalls that might
// indicate the command is waiting for interactive input. If detected, it
// enqueues a notification advising the agent to kill and retry.
func (m *Manager) stallWatchdog(ctx context.Context, task *Task) {
	ticker := time.NewTicker(stallCheckInterval)
	defer ticker.Stop()

	var lastLen int
	var stalledSince time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		task.mu.Lock()
		if task.Status != TaskRunning {
			task.mu.Unlock()
			return
		}
		currentLen := task.output.Len()
		// Read tail inline (we already hold the lock).
		tail := task.output.String()
		if len(tail) > maxTailBytes {
			tail = tail[len(tail)-maxTailBytes:]
		}
		task.mu.Unlock()

		if currentLen != lastLen {
			// Output is still growing — reset stall timer.
			lastLen = currentLen
			stalledSince = time.Time{}
			continue
		}

		// Output hasn't grown.
		if stalledSince.IsZero() {
			stalledSince = time.Now()
			continue
		}

		if time.Since(stalledSince) < stallThreshold {
			continue
		}

		// Stalled long enough. Check if the tail looks like an interactive prompt.
		if !promptPatterns.MatchString(tail) {
			continue
		}

		m.logger.Warn("background task appears stalled on interactive prompt",
			slog.String("task_id", task.ID),
		)

		// Enqueue a stall notification (only once).
		if !task.MarkNotified() {
			return
		}

		n := Notification{
			TaskID:      task.ID,
			BotID:       task.BotID,
			SessionID:   task.SessionID,
			Status:      TaskRunning, // still running, but stalled
			Command:     task.Command,
			Description: task.Description,
			ExitCode:    0,
			OutputFile:  task.OutputFile,
			OutputTail:  tail,
			Duration:    time.Since(task.StartedAt),
			Stalled:     true,
		}

		m.enqueueNotification(n)
		return // only notify once per task
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func detachedContextWithTimeout(parentCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parentCtx), timeout)
}
