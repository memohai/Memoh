// Package schedule provides the MCP schedule provider (list, get, create, trigger).
package schedule

import (
	"context"
	"log/slog"
	"strings"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	sched "github.com/memohai/memoh/internal/schedule"
)

const (
	toolScheduleList   = "list_schedule"
	toolScheduleGet    = "get_schedule"
	toolScheduleCreate = "create_schedule"
	toolScheduleUpdate = "update_schedule"
	toolScheduleDelete = "delete_schedule"
)

// Scheduler provides schedule list, get, create, update, delete (used by schedule tool executor).
type Scheduler interface {
	List(ctx context.Context, botID string) ([]sched.Schedule, error)
	Get(ctx context.Context, id string) (sched.Schedule, error)
	Create(ctx context.Context, botID string, req sched.CreateRequest) (sched.Schedule, error)
	Update(ctx context.Context, id string, req sched.UpdateRequest) (sched.Schedule, error)
	Delete(ctx context.Context, id string) error
}

// Executor is the MCP tool executor for list_schedule, get_schedule, create_schedule, update_schedule, delete_schedule.
type Executor struct {
	service Scheduler
	logger  *slog.Logger
}

// NewExecutor creates a schedule tool executor.
func NewExecutor(log *slog.Logger, service Scheduler) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		service: service,
		logger:  log.With(slog.String("provider", "schedule_tool")),
	}
}

// ListTools returns list_schedule, get_schedule, create_schedule, update_schedule, delete_schedule descriptors.
func (p *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if p.service == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolScheduleList,
			Description: "List schedules for current bot",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        toolScheduleGet,
			Description: "Get a schedule by id",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Schedule ID"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        toolScheduleCreate,
			Description: "Create a new schedule",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"pattern":     map[string]any{"type": "string"},
					"max_calls": map[string]any{
						"type":        []string{"integer", "null"},
						"description": "Optional max calls, null means unlimited",
					},
					"enabled": map[string]any{"type": "boolean"},
					"command": map[string]any{"type": "string"},
				},
				"required": []string{"name", "description", "pattern", "command"},
			},
		},
		{
			Name:        toolScheduleUpdate,
			Description: "Update an existing schedule",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string"},
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"pattern":     map[string]any{"type": "string"},
					"max_calls":   map[string]any{"type": []string{"integer", "null"}},
					"enabled":     map[string]any{"type": "boolean"},
					"command":     map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        toolScheduleDelete,
			Description: "Delete a schedule by id",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Schedule ID"},
				},
				"required": []string{"id"},
			},
		},
	}, nil
}

// CallTool runs list_schedule, get_schedule, create_schedule, update_schedule, or delete_schedule.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if p.service == nil {
		return mcpgw.BuildToolErrorResult("schedule service not available"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	switch toolName {
	case toolScheduleList:
		items, err := p.service.List(ctx, botID)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{
			"items": items,
		}), nil
	case toolScheduleGet:
		id := mcpgw.StringArg(arguments, "id")
		if id == "" {
			return mcpgw.BuildToolErrorResult("id is required"), nil
		}
		item, err := p.service.Get(ctx, id)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		if item.BotID != botID {
			return mcpgw.BuildToolErrorResult("bot mismatch"), nil
		}
		return mcpgw.BuildToolSuccessResult(item), nil
	case toolScheduleCreate:
		name := mcpgw.StringArg(arguments, "name")
		description := mcpgw.StringArg(arguments, "description")
		pattern := mcpgw.StringArg(arguments, "pattern")
		command := mcpgw.StringArg(arguments, "command")
		if name == "" || description == "" || pattern == "" || command == "" {
			return mcpgw.BuildToolErrorResult("name, description, pattern, command are required"), nil
		}

		req := sched.CreateRequest{
			Name:        name,
			Description: description,
			Pattern:     pattern,
			Command:     command,
		}
		maxCalls, err := parseNullableIntArg(arguments, "max_calls")
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		req.MaxCalls = maxCalls
		if enabled, ok, err := mcpgw.BoolArg(arguments, "enabled"); err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		} else if ok {
			req.Enabled = &enabled
		}
		item, err := p.service.Create(ctx, botID, req)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(item), nil
	case toolScheduleUpdate:
		id := mcpgw.StringArg(arguments, "id")
		if id == "" {
			return mcpgw.BuildToolErrorResult("id is required"), nil
		}
		req := sched.UpdateRequest{}
		maxCalls, err := parseNullableIntArg(arguments, "max_calls")
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		req.MaxCalls = maxCalls
		if value := mcpgw.StringArg(arguments, "name"); value != "" {
			req.Name = &value
		}
		if value := mcpgw.StringArg(arguments, "description"); value != "" {
			req.Description = &value
		}
		if value := mcpgw.StringArg(arguments, "pattern"); value != "" {
			req.Pattern = &value
		}
		if value := mcpgw.StringArg(arguments, "command"); value != "" {
			req.Command = &value
		}
		if enabled, ok, err := mcpgw.BoolArg(arguments, "enabled"); err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		} else if ok {
			req.Enabled = &enabled
		}
		item, err := p.service.Update(ctx, id, req)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		if item.BotID != botID {
			return mcpgw.BuildToolErrorResult("bot mismatch"), nil
		}
		return mcpgw.BuildToolSuccessResult(item), nil
	case toolScheduleDelete:
		id := mcpgw.StringArg(arguments, "id")
		if id == "" {
			return mcpgw.BuildToolErrorResult("id is required"), nil
		}
		item, err := p.service.Get(ctx, id)
		if err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		if item.BotID != botID {
			return mcpgw.BuildToolErrorResult("bot mismatch"), nil
		}
		if err := p.service.Delete(ctx, id); err != nil {
			return mcpgw.BuildToolErrorResult(err.Error()), nil
		}
		return mcpgw.BuildToolSuccessResult(map[string]any{"success": true}), nil
	default:
		return nil, mcpgw.ErrToolNotFound
	}
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
	value, _, err := mcpgw.IntArg(arguments, key)
	if err != nil {
		return sched.NullableInt{}, err
	}
	req.Value = &value
	return req, nil
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
