package tools

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	sched "github.com/memohai/memoh/internal/schedule"
)

type ScheduleProvider struct {
	service Scheduler
	logger  *slog.Logger
}

// Scheduler is the interface for schedule CRUD operations.
type Scheduler interface {
	List(ctx context.Context, botID string) ([]sched.Schedule, error)
	Get(ctx context.Context, id string) (sched.Schedule, error)
	Create(ctx context.Context, botID string, req sched.CreateRequest) (sched.Schedule, error)
	Update(ctx context.Context, id string, req sched.UpdateRequest) (sched.Schedule, error)
	Delete(ctx context.Context, id string) error
}

func NewScheduleProvider(log *slog.Logger, service Scheduler) *ScheduleProvider {
	if log == nil {
		log = slog.Default()
	}
	return &ScheduleProvider{
		service: service,
		logger:  log.With(slog.String("tool", "schedule")),
	}
}

// Usage describes how the schedule tool group works together. Injected only
// when the schedule tools are registered (main-agent sessions with a schedule
// service); subagents register none, so this guidance never shows there.
func (p *ScheduleProvider) Usage(_ context.Context, _ SessionContext, available AvailableTools) string {
	var parts []string
	delivery := "include an instruction to deliver results to a person or channel when messaging is available"
	if available.Has(ToolSend) {
		delivery = "use " + toolRef(ToolSend) + " inside the command to deliver results to a person or channel"
	} else if available.Has(ToolSpeak) {
		delivery = "use an available messaging tool inside the command to deliver results to a person or channel"
	}
	if available.Has(ToolCreateSchedule) {
		parts = append(parts, "Create recurring work with "+toolRef(ToolCreateSchedule)+": put a natural-language instruction in `command`; when the cron `pattern` fires, the task runs in its own session and you receive a message containing that command — "+delivery+".")
	}
	var manage []string
	for _, name := range []ToolName{ToolListSchedule, ToolGetSchedule, ToolUpdateSchedule, ToolDeleteSchedule} {
		if available.Has(name) {
			manage = append(manage, toolRef(name))
		}
	}
	if len(manage) > 0 {
		parts = append(parts, "Inspect and manage existing tasks with "+strings.Join(manage, ", ")+".")
	}
	if len(parts) == 0 {
		return ""
	}
	return "### Scheduled tasks\n\n" +
		strings.Join(parts, " ")
}

func (p *ScheduleProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.service == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name: ToolListSchedule.String(), Description: "List schedules for current bot",
			Parameters: emptyObjectSchema(),
			Execute: func(ctx *sdk.ToolExecContext, _ any) (any, error) {
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, errors.New("bot_id is required")
				}
				items, err := p.service.List(ctx.Context, botID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"items": items}, nil
			},
		},
		{
			Name: ToolGetSchedule.String(), Description: "Get a schedule by id",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Schedule ID"},
				},
				"required": []string{"id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, errors.New("bot_id is required")
				}
				id := StringArg(args, "id")
				if id == "" {
					return nil, errors.New("id is required")
				}
				item, err := p.service.Get(ctx.Context, id)
				if err != nil {
					return nil, err
				}
				if item.BotID != botID {
					return nil, errors.New("bot mismatch")
				}
				return item, nil
			},
		},
		{
			Name: ToolCreateSchedule.String(), Description: "Create a new cron-scheduled task. Fill `command` with a natural-language instruction; when the cron `pattern` fires, the task runs in its own session and you receive a message containing that `command`. Include delivery instructions in the command when results should be sent to a person or channel. Set `max_calls` to null for unlimited runs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
					"pattern": map[string]any{"type": "string"}, "command": map[string]any{"type": "string"},
					"max_calls": map[string]any{"anyOf": []map[string]any{{"type": "integer"}, {"type": "null"}}, "description": "Optional max calls, null means unlimited"},
					"enabled":   map[string]any{"type": "boolean"},
				},
				"required": []string{"name", "description", "pattern", "command"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, errors.New("bot_id is required")
				}
				name := StringArg(args, "name")
				description := StringArg(args, "description")
				pattern := StringArg(args, "pattern")
				command := StringArg(args, "command")
				if name == "" || description == "" || pattern == "" || command == "" {
					return nil, errors.New("name, description, pattern, command are required")
				}
				req := sched.CreateRequest{Name: name, Description: description, Pattern: pattern, Command: command}
				maxCalls, err := parseNullableIntArg(args, "max_calls")
				if err != nil {
					return nil, err
				}
				req.MaxCalls = maxCalls
				if enabled, ok, err := BoolArg(args, "enabled"); err != nil {
					return nil, err
				} else if ok {
					req.Enabled = &enabled
				}
				item, err := p.service.Create(ctx.Context, botID, req)
				if err != nil {
					return nil, err
				}
				return item, nil
			},
		},
		{
			Name: ToolUpdateSchedule.String(), Description: "Update an existing schedule",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"}, "pattern": map[string]any{"type": "string"},
					"command":   map[string]any{"type": "string"},
					"max_calls": map[string]any{"anyOf": []map[string]any{{"type": "integer"}, {"type": "null"}}},
					"enabled":   map[string]any{"type": "boolean"},
				},
				"required": []string{"id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, errors.New("bot_id is required")
				}
				id := StringArg(args, "id")
				if id == "" {
					return nil, errors.New("id is required")
				}
				req := sched.UpdateRequest{}
				maxCalls, err := parseNullableIntArg(args, "max_calls")
				if err != nil {
					return nil, err
				}
				req.MaxCalls = maxCalls
				if v := StringArg(args, "name"); v != "" {
					req.Name = &v
				}
				if v := StringArg(args, "description"); v != "" {
					req.Description = &v
				}
				if v := StringArg(args, "pattern"); v != "" {
					req.Pattern = &v
				}
				if v := StringArg(args, "command"); v != "" {
					req.Command = &v
				}
				if enabled, ok, err := BoolArg(args, "enabled"); err != nil {
					return nil, err
				} else if ok {
					req.Enabled = &enabled
				}
				item, err := p.service.Update(ctx.Context, id, req)
				if err != nil {
					return nil, err
				}
				if item.BotID != botID {
					return nil, errors.New("bot mismatch")
				}
				return item, nil
			},
		},
		{
			Name: ToolDeleteSchedule.String(), Description: "Delete a schedule by id",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Schedule ID"},
				},
				"required": []string{"id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, errors.New("bot_id is required")
				}
				id := StringArg(args, "id")
				if id == "" {
					return nil, errors.New("id is required")
				}
				item, err := p.service.Get(ctx.Context, id)
				if err != nil {
					return nil, err
				}
				if item.BotID != botID {
					return nil, errors.New("bot mismatch")
				}
				if err := p.service.Delete(ctx.Context, id); err != nil {
					return nil, err
				}
				return map[string]any{"success": true}, nil
			},
		},
	}, nil
}

func parseNullableIntArg(arguments map[string]any, key string) (sched.NullableInt, error) {
	req := sched.NullableInt{}
	if arguments == nil {
		return req, nil
	}
	raw, exists := arguments[key]
	if !exists {
		return req, nil
	}
	req.Set = true
	if raw == nil {
		req.Value = nil
		return req, nil
	}
	value, _, err := IntArg(arguments, key)
	if err != nil {
		return sched.NullableInt{}, err
	}
	req.Value = &value
	return req, nil
}

func emptyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
