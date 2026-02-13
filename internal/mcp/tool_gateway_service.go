package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	defaultToolRegistryCacheTTL = 5 * time.Second
)

type cachedToolRegistry struct {
	expiresAt time.Time
	registry  *ToolRegistry
}

// ToolGatewayService federates tools from executors and sources.
type ToolGatewayService struct {
	logger    *slog.Logger
	executors []ToolExecutor
	sources   []ToolSource
	cacheTTL  time.Duration

	mu    sync.Mutex
	cache map[string]cachedToolRegistry
}

func NewToolGatewayService(log *slog.Logger, executors []ToolExecutor, sources []ToolSource) *ToolGatewayService {
	if log == nil {
		log = slog.Default()
	}
	filteredExecutors := make([]ToolExecutor, 0, len(executors))
	for _, executor := range executors {
		if executor != nil {
			filteredExecutors = append(filteredExecutors, executor)
		}
	}
	filteredSources := make([]ToolSource, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filteredSources = append(filteredSources, source)
		}
	}
	return &ToolGatewayService{
		logger:    log.With(slog.String("service", "tool_gateway")),
		executors: filteredExecutors,
		sources:   filteredSources,
		cacheTTL:  defaultToolRegistryCacheTTL,
		cache:     map[string]cachedToolRegistry{},
	}
}

func (s *ToolGatewayService) InitializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "memoh-tools-gateway",
			"version": "1.0.0",
		},
	}
}

func (s *ToolGatewayService) ListTools(ctx context.Context, session ToolSessionContext) ([]ToolDescriptor, error) {
	registry, err := s.getRegistry(ctx, session, false)
	if err != nil {
		return nil, err
	}
	return registry.List(), nil
}

func (s *ToolGatewayService) CallTool(ctx context.Context, session ToolSessionContext, payload ToolCallPayload) (map[string]any, error) {
	toolName := strings.TrimSpace(payload.Name)
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}

	registry, err := s.getRegistry(ctx, session, false)
	if err != nil {
		return nil, err
	}
	executor, _, ok := registry.Lookup(toolName)
	if !ok {
		// Refresh once for dynamic executors/sources.
		registry, err = s.getRegistry(ctx, session, true)
		if err != nil {
			return nil, err
		}
		executor, _, ok = registry.Lookup(toolName)
		if !ok {
			return BuildToolErrorResult("tool not found: " + toolName), nil
		}
	}

	arguments := payload.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}
	result, err := executor.CallTool(ctx, session, toolName, arguments)
	if err != nil {
		if errors.Is(err, ErrToolNotFound) {
			return BuildToolErrorResult("tool not found: " + toolName), nil
		}
		return BuildToolErrorResult(err.Error()), nil
	}
	if result == nil {
		return BuildToolSuccessResult(map[string]any{"ok": true}), nil
	}
	return result, nil
}

func (s *ToolGatewayService) getRegistry(ctx context.Context, session ToolSessionContext, force bool) (*ToolRegistry, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, fmt.Errorf("bot id is required")
	}
	if !force {
		s.mu.Lock()
		cached, ok := s.cache[botID]
		if ok && time.Now().Before(cached.expiresAt) && cached.registry != nil {
			s.mu.Unlock()
			return cached.registry, nil
		}
		s.mu.Unlock()
	}

	registry := NewToolRegistry()
	for _, executor := range s.executors {
		tools, err := executor.ListTools(ctx, session)
		if err != nil {
			s.logger.Warn("list tools from executor failed", slog.Any("error", err))
			continue
		}
		for _, tool := range tools {
			if err := registry.Register(executor, tool); err != nil {
				s.logger.Warn("skip duplicated/invalid tool", slog.String("tool", tool.Name), slog.Any("error", err))
			}
		}
	}
	for _, source := range s.sources {
		tools, err := source.ListTools(ctx, session)
		if err != nil {
			s.logger.Warn("list tools from source failed", slog.Any("error", err))
			continue
		}
		for _, tool := range tools {
			if err := registry.Register(source, tool); err != nil {
				s.logger.Warn("skip duplicated/invalid tool", slog.String("tool", tool.Name), slog.Any("error", err))
			}
		}
	}

	s.mu.Lock()
	s.cache[botID] = cachedToolRegistry{
		expiresAt: time.Now().Add(s.cacheTTL),
		registry:  registry,
	}
	s.mu.Unlock()
	return registry, nil
}
