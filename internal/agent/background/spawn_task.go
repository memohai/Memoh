package background

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// TaskKind identifies what kind of work a background task tracks.
type TaskKind string

const (
	// KindExec is a background container command execution.
	KindExec TaskKind = "exec"
	// KindSpawn is a background subagent batch run by the spawn tool.
	KindSpawn TaskKind = "spawn"
	// KindAgent is a single managed subagent task.
	KindAgent TaskKind = "agent"
)

// SpawnTaskTimeout is the safety ceiling for a background spawn task,
// mirroring BackgroundExecTimeout for exec tasks.
const SpawnTaskTimeout = 30 * time.Minute

// MaxRunningSpawnTasks caps concurrently running background spawn tasks per
// bot+session to prevent subagent storms across agent runs.
const MaxRunningSpawnTasks = 3

// spawnReportMaxBytes caps each branch report carried in join notifications,
// mirroring maxTailBytes for exec output tails. Full transcripts stay in the
// child session.
const spawnReportMaxBytes = 2048

// spawnBranchTaskMaxBytes caps the per-branch task echo: identification only,
// the full task text is persisted as the child session's user message.
const spawnBranchTaskMaxBytes = 200

// spawnBranchErrorMaxBytes caps per-branch error strings, whose summary sits
// at the head.
const spawnBranchErrorMaxBytes = 512

// SpawnBranch is the join-record entry for one subagent in a spawn batch.
// ChildSessionID points at the persisted subagent session so the parent
// agent can read the full transcript via history tools.
type SpawnBranch struct {
	Task           string
	ChildSessionID string
	Status         TaskStatus
	Report         string
	Error          string
}

// AgentTaskResult is the terminal output for one managed subagent run.
type AgentTaskResult struct {
	AgentID        string
	AgentSessionID string
	Message        string
	Status         TaskStatus
	Report         string
	Error          string
}

// StartAgentTask registers a managed subagent task. Queued tasks are visible
// to bg_status immediately but do not get a cancelable run context until
// MarkAgentTaskRunning is called.
func (m *Manager) StartAgentTask(parentCtx context.Context, botID, sessionID, agentID, agentSessionID, message, description string, queued bool) (string, context.Context, error) {
	status := TaskRunning
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if queued {
		status = TaskQueued
	} else {
		ctx, cancel = detachedContextWithTimeout(parentCtx, SpawnTaskTimeout)
	}

	m.mu.Lock()
	taskID := m.newTaskIDLocked(botID)
	task := &Task{
		ID:             taskID,
		Kind:           KindAgent,
		BotID:          botID,
		SessionID:      sessionID,
		Description:    description,
		AgentID:        agentID,
		AgentSessionID: agentSessionID,
		AgentMessage:   message,
		Status:         status,
		StartedAt:      time.Now(),
		cancel:         cancel,
	}
	m.tasks[taskID] = task
	m.mu.Unlock()

	m.logger.Info("background agent task registered",
		slog.String("task_id", taskID),
		slog.String("bot_id", botID),
		slog.String("agent_id", agentID),
		slog.String("status", string(status)),
	)
	if queued {
		m.emitTaskEvent(task, TaskEventQueued, "", "")
	} else {
		m.emitTaskEvent(task, TaskEventStarted, "", "")
	}
	return taskID, ctx, nil
}

// MarkAgentTaskRunning transitions a queued managed agent task into running
// and returns the cancelable run context. If the task was killed while queued,
// ok is false and no run should start.
func (m *Manager) MarkAgentTaskRunning(parentCtx context.Context, taskID string) (context.Context, bool, error) {
	ctx, cancel := detachedContextWithTimeout(parentCtx, SpawnTaskTimeout)
	m.mu.Lock()
	task := m.tasks[taskID]
	m.mu.Unlock()
	if task == nil || task.Kind != KindAgent {
		cancel()
		return nil, false, fmt.Errorf("agent task %s not found", taskID)
	}
	task.mu.Lock()
	if task.Status == TaskKilled {
		task.mu.Unlock()
		cancel()
		return nil, false, nil
	}
	if task.Status != TaskQueued {
		task.mu.Unlock()
		cancel()
		return nil, false, fmt.Errorf("agent task %s is not queued (status: %s)", taskID, task.Status)
	}
	task.Status = TaskRunning
	task.cancel = cancel
	task.mu.Unlock()

	m.emitTaskEvent(task, TaskEventStarted, "", "")
	return ctx, true, nil
}

// CompleteAgentTask finalises a managed agent task and enqueues a completion
// notification unless it was killed before completion.
func (m *Manager) CompleteAgentTask(taskID string, result AgentTaskResult) {
	m.mu.Lock()
	task := m.tasks[taskID]
	m.mu.Unlock()
	if task == nil || task.Kind != KindAgent {
		return
	}
	defer task.Cancel()

	status := result.Status
	if status == "" {
		if result.Error != "" {
			status = TaskFailed
		} else {
			status = TaskCompleted
		}
	}

	task.mu.Lock()
	if task.Status == TaskKilled {
		task.mu.Unlock()
		return
	}
	task.CompletedAt = time.Now()
	task.Status = status
	task.AgentReport = result.Report
	task.AgentError = result.Error
	if result.Message != "" {
		task.AgentMessage = result.Message
	}
	duration := task.CompletedAt.Sub(task.StartedAt)
	task.mu.Unlock()

	eventType := TaskEventCompleted
	if status != TaskCompleted {
		eventType = TaskEventFailed
	}
	m.emitTaskEvent(task, eventType, "", "")

	if !task.MarkNotified() {
		return
	}
	m.enqueueNotification(Notification{
		TaskID:         task.ID,
		Kind:           KindAgent,
		BotID:          task.BotID,
		SessionID:      task.SessionID,
		Status:         status,
		Description:    task.Description,
		AgentID:        task.AgentID,
		AgentSessionID: task.AgentSessionID,
		AgentMessage:   task.AgentMessage,
		AgentReport:    task.AgentReport,
		AgentError:     task.AgentError,
		Duration:       duration,
	})
}

// CompleteSpawnTask finalises a spawn task with its branch outcomes and
// enqueues the join notification. The task is completed when every branch
// completed and failed when any branch failed. Branch outcomes are recorded
// even for killed tasks, but killed tasks never notify.
func (m *Manager) CompleteSpawnTask(taskID string, branches []SpawnBranch) {
	m.mu.Lock()
	task := m.tasks[taskID]
	m.mu.Unlock()
	if task == nil || task.Kind != KindSpawn {
		return
	}
	defer task.Cancel() // release the safety-timeout context

	branches = clampSpawnBranches(branches)

	status := TaskCompleted
	for _, b := range branches {
		if b.Status != TaskCompleted {
			status = TaskFailed
			break
		}
	}

	task.mu.Lock()
	task.branches = branches
	if task.Status == TaskKilled {
		task.mu.Unlock()
		return
	}
	task.CompletedAt = time.Now()
	task.Status = status
	duration := task.CompletedAt.Sub(task.StartedAt)
	task.mu.Unlock()

	m.logger.Info("background spawn task finished",
		slog.String("task_id", task.ID),
		slog.String("status", string(status)),
		slog.Int("branches", len(branches)),
		slog.Duration("duration", duration),
	)

	eventType := TaskEventCompleted
	if status == TaskFailed {
		eventType = TaskEventFailed
	}
	m.emitTaskEvent(task, eventType, "", "")

	if !task.MarkNotified() {
		return
	}
	m.enqueueNotification(Notification{
		TaskID:      task.ID,
		Kind:        KindSpawn,
		BotID:       task.BotID,
		SessionID:   task.SessionID,
		Status:      status,
		Description: task.Description,
		Branches:    append([]SpawnBranch(nil), branches...),
		Duration:    duration,
	})
}

// clampSpawnBranches returns a copy of branches with each text field bounded:
// reports keep their tail (findings live at the end per the subagent response
// contract), task echoes and errors keep their head (identification and
// summary sit at the front).
func clampSpawnBranches(branches []SpawnBranch) []SpawnBranch {
	out := append([]SpawnBranch(nil), branches...)
	for i := range out {
		if len(out[i].Report) > spawnReportMaxBytes {
			out[i].Report = out[i].Report[len(out[i].Report)-spawnReportMaxBytes:]
		}
		out[i].Task = truncate(out[i].Task, spawnBranchTaskMaxBytes)
		out[i].Error = truncate(out[i].Error, spawnBranchErrorMaxBytes)
	}
	return out
}

// formatSpawnForAgent renders the join record of a spawn task.
func (n Notification) formatSpawnForAgent() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<task-notification>\n")
	fmt.Fprintf(&b, "  <task-id>%s</task-id>\n", n.TaskID)
	fmt.Fprintf(&b, "  <kind>spawn</kind>\n")
	fmt.Fprintf(&b, "  <status>%s</status>\n", n.Status)
	if n.Description != "" {
		fmt.Fprintf(&b, "  <description>%s</description>\n", n.Description)
	}
	fmt.Fprintf(&b, "  <duration>%s</duration>\n", n.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "  <branches>\n")
	for _, br := range n.Branches {
		fmt.Fprintf(&b, "    <branch status=%q", br.Status)
		if br.ChildSessionID != "" {
			fmt.Fprintf(&b, " session-id=%q", br.ChildSessionID)
		}
		fmt.Fprintf(&b, ">\n")
		fmt.Fprintf(&b, "      <task>%s</task>\n", br.Task)
		if br.Report != "" {
			fmt.Fprintf(&b, "      <report>\n%s\n      </report>\n", strings.TrimRight(br.Report, "\n"))
		}
		if br.Error != "" {
			fmt.Fprintf(&b, "      <error>%s</error>\n", br.Error)
		}
		fmt.Fprintf(&b, "    </branch>\n")
	}
	fmt.Fprintf(&b, "  </branches>\n")
	fmt.Fprintf(&b, "  <suggestion>Read a branch's full transcript with search_messages using its session-id.</suggestion>\n")
	fmt.Fprintf(&b, "</task-notification>")
	return b.String()
}

// formatAgentForAgent renders the terminal record for one managed subagent.
func (n Notification) formatAgentForAgent() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<task-notification>\n")
	fmt.Fprintf(&b, "  <task-id>%s</task-id>\n", n.TaskID)
	fmt.Fprintf(&b, "  <kind>agent</kind>\n")
	fmt.Fprintf(&b, "  <agent-id>%s</agent-id>\n", n.AgentID)
	if n.AgentSessionID != "" {
		fmt.Fprintf(&b, "  <session-id>%s</session-id>\n", n.AgentSessionID)
	}
	fmt.Fprintf(&b, "  <status>%s</status>\n", n.Status)
	if n.Description != "" {
		fmt.Fprintf(&b, "  <description>%s</description>\n", n.Description)
	}
	if n.AgentMessage != "" {
		fmt.Fprintf(&b, "  <message>%s</message>\n", n.AgentMessage)
	}
	fmt.Fprintf(&b, "  <duration>%s</duration>\n", n.Duration.Round(time.Millisecond))
	if n.AgentReport != "" {
		fmt.Fprintf(&b, "  <report>\n%s\n  </report>\n", strings.TrimRight(n.AgentReport, "\n"))
	}
	if n.AgentError != "" {
		fmt.Fprintf(&b, "  <error>%s</error>\n", n.AgentError)
	}
	fmt.Fprintf(&b, "  <suggestion>Use send_message with this agent-id to continue the same subagent.</suggestion>\n")
	fmt.Fprintf(&b, "</task-notification>")
	return b.String()
}

// runningSpawnCountLocked counts running spawn tasks for a bot+session.
// Caller must hold m.mu.
func (m *Manager) runningSpawnCountLocked(botID, sessionID string) int {
	count := 0
	for _, t := range m.tasks {
		if t.Kind != KindSpawn || t.BotID != botID || t.SessionID != sessionID {
			continue
		}
		t.mu.Lock()
		if t.Status == TaskRunning {
			count++
		}
		t.mu.Unlock()
	}
	return count
}

// StartSpawnTask registers a background task for a spawn (subagent batch)
// whose execution is driven by the spawn tool. It returns the task ID and a
// detached, cancelable context that subagent branches must derive from so
// Kill can stop in-flight work.
func (m *Manager) StartSpawnTask(parentCtx context.Context, botID, sessionID, description string) (string, context.Context, error) {
	ctx, cancel := detachedContextWithTimeout(parentCtx, SpawnTaskTimeout)

	m.mu.Lock()
	if m.runningSpawnCountLocked(botID, sessionID) >= MaxRunningSpawnTasks {
		m.mu.Unlock()
		cancel()
		return "", nil, fmt.Errorf("spawn limit reached: max %d concurrently running background spawn tasks per session", MaxRunningSpawnTasks)
	}
	taskID := m.newTaskIDLocked(botID)
	task := &Task{
		ID:          taskID,
		Kind:        KindSpawn,
		BotID:       botID,
		SessionID:   sessionID,
		Description: description,
		Status:      TaskRunning,
		StartedAt:   time.Now(),
		cancel:      cancel,
	}
	m.tasks[taskID] = task
	m.mu.Unlock()

	m.logger.Info("background spawn task started",
		slog.String("task_id", taskID),
		slog.String("bot_id", botID),
		slog.String("description", truncate(description, 120)),
	)
	m.emitTaskEvent(task, TaskEventStarted, "", "")
	return taskID, ctx, nil
}
