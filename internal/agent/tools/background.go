package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
)

const (
	maxWaitDuration                = 300 * time.Second
	backgroundWaitProgressInterval = 30 * time.Second
)

// BackgroundProvider exposes background task observation and control tools.
type BackgroundProvider struct {
	bgManager *background.Manager
}

func NewBackgroundProvider(_ *slog.Logger, bgManager *background.Manager) *BackgroundProvider {
	return &BackgroundProvider{
		bgManager: bgManager,
	}
}

func (*BackgroundProvider) Usage(_ context.Context, _ SessionContext, available AvailableTools) string {
	var parts []string
	if ref, ok := available.Ref(ToolListBackground()); ok {
		parts = append(parts, ref+": list background tasks for this session")
	}
	if ref, ok := available.Ref(ToolWait()); ok {
		parts = append(parts, ref+": wait for a short fixed duration when there is no specific task to observe")
	}
	if ref, ok := available.Ref(ToolWaitUntil()); ok {
		parts = append(parts, ref+": wait for a background task to complete, fail, be killed, or appear stalled")
	}
	if ref, ok := available.Ref(ToolGetBackgroundStatus()); ok {
		parts = append(parts, ref+": inspect a background task and read its result")
	}
	if ref, ok := available.Ref(ToolKillBackground()); ok {
		parts = append(parts, ref+": stop a running or queued background task")
	}
	if len(parts) == 0 {
		return ""
	}
	parts = append(parts, "After starting long work in the background, call `wait_until(task_id)` and then `get_background_status(task_id)` to read `result`.")
	return usageSection("Background Tasks", parts)
}

func (p *BackgroundProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if p.bgManager == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        ToolListBackground().String(),
			Description: "List background tasks for the current session.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execListBackground(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        ToolWait().String(),
			Description: "Wait for a fixed duration in seconds. Use wait_until when you have a background task_id.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"duration": map[string]any{"type": "number", "description": "Seconds to wait. Must be > 0 and at most 300.", "minimum": 0, "maximum": 300},
				},
				"required": []string{"duration"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execWait(ctx.Context, sess, inputAsMap(input), ctx.SendProgress)
			},
		},
		{
			Name:        ToolWaitUntil().String(),
			Description: "Wait until a background task completes, fails, is killed, or appears stalled. Call get_background_status afterward to inspect result.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "description": "Background task ID"},
				},
				"required": []string{"task_id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execWaitUntil(ctx.Context, sess, inputAsMap(input), ctx.SendProgress)
			},
		},
		{
			Name:        ToolGetBackgroundStatus().String(),
			Description: "Get the status and details of a background task. For completed agent/spawn tasks, read the result field.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "description": "Background task ID"},
				},
				"required": []string{"task_id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execGetBackgroundStatus(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        ToolKillBackground().String(),
			Description: "Kill a running or queued background task.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "description": "Background task ID"},
				},
				"required": []string{"task_id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execKillBackground(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

func (p *BackgroundProvider) execListBackground(_ context.Context, session SessionContext, _ map[string]any) (any, error) {
	snapshots := p.bgManager.ListSnapshotsForSession(session.BotID, session.SessionID)
	entries := make([]map[string]any, 0, len(snapshots))
	for _, s := range snapshots {
		entry := map[string]any{
			"task_id":     s.TaskID,
			"kind":        string(s.Kind),
			"description": s.Description,
			"status":      statusString(s),
			"started_at":  session.FormatTime(s.StartedAt),
		}
		if s.Kind == background.KindAgent {
			entry["agent_id"] = s.AgentID
			entry["session_id"] = s.AgentSessionID
		}
		if s.Kind == background.KindExec {
			entry["command"] = truncateStr(s.Command, 120)
			entry["output_file"] = s.OutputFile
		}
		entries = append(entries, entry)
	}
	return map[string]any{"tasks": entries, "count": len(entries)}, nil
}

func (*BackgroundProvider) execWait(ctx context.Context, _ SessionContext, args map[string]any, sendProgress func(any)) (any, error) {
	duration, err := durationArg(args, "duration")
	if err != nil {
		return nil, err
	}
	progress := func() {
		emitWaitProgress(sendProgress, map[string]any{
			"status":   "waiting",
			"duration": duration.Seconds(),
		})
	}
	progress()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	ticker := time.NewTicker(backgroundWaitProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return map[string]any{"ok": false, "duration": duration.Seconds(), "error": ctx.Err().Error()}, ctx.Err()
		case <-timer.C:
			return map[string]any{"ok": true, "duration": duration.Seconds()}, nil
		case <-ticker.C:
			progress()
		}
	}
}

func (p *BackgroundProvider) execWaitUntil(ctx context.Context, session SessionContext, args map[string]any, sendProgress func(any)) (any, error) {
	taskID := strings.TrimSpace(StringArg(args, "task_id"))
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(background.BackgroundExecTimeout)*time.Second)
	defer cancel()
	s, err := p.waitForSessionTaskWithProgress(waitCtx, session.BotID, session.SessionID, taskID, sendProgress)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"task_id":    s.TaskID,
		"kind":       string(s.Kind),
		"status":     statusString(s),
		"stalled":    s.Stalled,
		"started_at": session.FormatTime(s.StartedAt),
	}
	if s.CompletedAt.IsZero() {
		return result, nil
	}
	result["completed_at"] = session.FormatTime(s.CompletedAt)
	result["duration"] = s.Duration.Round(time.Millisecond).String()
	return result, nil
}

func (p *BackgroundProvider) waitForSessionTaskWithProgress(ctx context.Context, botID, sessionID, taskID string, sendProgress func(any)) (background.TaskSnapshot, error) {
	if sendProgress == nil {
		return p.bgManager.WaitForSessionTask(ctx, botID, sessionID, taskID)
	}

	type waitResult struct {
		snapshot background.TaskSnapshot
		err      error
	}
	resultCh := make(chan waitResult, 1)
	go func() {
		s, err := p.bgManager.WaitForSessionTask(ctx, botID, sessionID, taskID)
		resultCh <- waitResult{snapshot: s, err: err}
	}()

	progress := func() {
		emitWaitProgress(sendProgress, map[string]any{
			"status":  "waiting",
			"task_id": taskID,
		})
	}
	progress()
	ticker := time.NewTicker(backgroundWaitProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case result := <-resultCh:
			return result.snapshot, result.err
		case <-ctx.Done():
			return background.TaskSnapshot{}, ctx.Err()
		case <-ticker.C:
			progress()
		}
	}
}

func emitWaitProgress(sendProgress func(any), payload map[string]any) {
	if sendProgress != nil {
		sendProgress(payload)
	}
}

func (p *BackgroundProvider) execGetBackgroundStatus(_ context.Context, session SessionContext, args map[string]any) (any, error) {
	taskID := strings.TrimSpace(StringArg(args, "task_id"))
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	task := p.bgManager.GetForSession(session.BotID, session.SessionID, taskID)
	if task == nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	return backgroundStatusMap(session, task.Snapshot()), nil
}

func (p *BackgroundProvider) execKillBackground(_ context.Context, session SessionContext, args map[string]any) (any, error) {
	taskID := strings.TrimSpace(StringArg(args, "task_id"))
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	if err := p.bgManager.KillForSession(session.BotID, session.SessionID, taskID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "message": fmt.Sprintf("Task %s has been killed.", taskID)}, nil
}

func backgroundStatusMap(session SessionContext, s background.TaskSnapshot) map[string]any {
	result := map[string]any{
		"task_id":     s.TaskID,
		"kind":        string(s.Kind),
		"description": s.Description,
		"status":      statusString(s),
		"started_at":  session.FormatTime(s.StartedAt),
		"stalled":     s.Stalled,
	}
	if !s.CompletedAt.IsZero() {
		result["completed_at"] = session.FormatTime(s.CompletedAt)
		result["duration"] = s.Duration.Round(time.Millisecond).String()
	}
	switch s.Kind {
	case background.KindAgent:
		result["agent_id"] = s.AgentID
		result["session_id"] = s.AgentSessionID
		if s.AgentMessage != "" {
			result["input"] = s.AgentMessage
		}
		result["result"] = s.AgentReport
		if s.AgentError != "" {
			result["error"] = s.AgentError
		}
	case background.KindSpawn:
		branches := make([]map[string]any, 0, len(s.Branches))
		for _, br := range s.Branches {
			item := map[string]any{
				"task":   br.Task,
				"status": string(br.Status),
				"result": br.Report,
			}
			if br.ChildSessionID != "" {
				item["session_id"] = br.ChildSessionID
			}
			if br.Error != "" {
				item["error"] = br.Error
			}
			branches = append(branches, item)
		}
		result["result"] = map[string]any{"branches": branches}
		// Keep the branch list at the top level for existing UI rendering.
		if len(branches) > 0 {
			result["branches"] = branches
		}
	case background.KindVideo:
		videoResult := make(map[string]any, len(s.Result)+1)
		for k, v := range s.Result {
			videoResult[k] = v
		}
		if s.Error != "" {
			videoResult["error"] = s.Error
			result["error"] = s.Error
		}
		result["result"] = videoResult
		if s.OutputTail != "" {
			result["output_tail"] = s.OutputTail
		}
	default:
		result["command"] = s.Command
		result["output_file"] = s.OutputFile
		execResult := map[string]any{"output_file": s.OutputFile}
		if s.Status != background.TaskRunning && s.Status != background.TaskQueued {
			result["exit_code"] = s.ExitCode
			result["output_tail"] = s.OutputTail
			execResult["exit_code"] = s.ExitCode
			execResult["output_tail"] = s.OutputTail
		}
		result["result"] = execResult
	}
	return result
}

func statusString(s background.TaskSnapshot) string {
	if s.Stalled {
		return "stalled"
	}
	return string(s.Status)
}

func durationArg(args map[string]any, key string) (time.Duration, error) {
	raw, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	var seconds float64
	switch v := raw.(type) {
	case float64:
		seconds = v
	case float32:
		seconds = float64(v)
	case int:
		seconds = float64(v)
	case int64:
		seconds = float64(v)
	case jsonNumber:
		parsed, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", key, err)
		}
		seconds = parsed
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, fmt.Errorf("%s must be > 0", key)
	}
	duration := time.Duration(seconds * float64(time.Second))
	if duration > maxWaitDuration {
		duration = maxWaitDuration
	}
	return duration, nil
}

type jsonNumber interface {
	Float64() (float64, error)
}
