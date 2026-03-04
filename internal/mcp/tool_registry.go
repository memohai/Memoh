package mcp

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type registryItem struct {
	executor ToolExecutor
	tool     ToolDescriptor
}

// ToolRegistry stores provider ownership and descriptor metadata.
type ToolRegistry struct {
	items map[string]registryItem
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		items: map[string]registryItem{},
	}
}

func (r *ToolRegistry) Register(executor ToolExecutor, tool ToolDescriptor) error {
	if executor == nil {
		return errors.New("tool executor is required")
	}
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return errors.New("tool name is required")
	}
	if tool.InputSchema == nil {
		tool.InputSchema = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	if _, exists := r.items[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	tool.Name = name
	r.items[name] = registryItem{
		executor: executor,
		tool:     tool,
	}
	return nil
}

func (r *ToolRegistry) Lookup(name string) (ToolExecutor, ToolDescriptor, bool) {
	item, ok := r.items[strings.TrimSpace(name)]
	if !ok {
		return nil, ToolDescriptor{}, false
	}
	return item.executor, item.tool, true
}

func (r *ToolRegistry) List() []ToolDescriptor {
	if len(r.items) == 0 {
		return []ToolDescriptor{}
	}
	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}
	sort.Strings(names)
	tools := make([]ToolDescriptor, 0, len(names))
	for _, name := range names {
		tools = append(tools, r.items[name].tool)
	}
	return tools
}
