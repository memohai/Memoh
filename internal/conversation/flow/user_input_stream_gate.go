package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/userinput"
)

type userInputStreamGate struct {
	active bool
	events []json.RawMessage
}

func (g *userInputStreamGate) hold(event agentpkg.StreamEvent, data []byte) bool {
	if !g.active && (event.Type == agentpkg.EventUserInputRequest || strings.EqualFold(strings.TrimSpace(event.ToolName), userinput.ToolNameAskUser)) {
		g.active = true
	}
	if !g.active {
		return false
	}
	g.events = append(g.events, append(json.RawMessage(nil), data...))
	return true
}

func (g *userInputStreamGate) release(send func(json.RawMessage) error) error {
	for _, event := range g.events {
		if err := send(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resolver) streamContinuation(
	ctx context.Context,
	cfg agentpkg.RunConfig,
	req conversation.ChatRequest,
	modelID string,
	eventCh chan<- WSStreamEvent,
) error {
	stream := r.agent.Stream(ctx, cfg)
	stored := false
	var lastSnapshot terminalSnapshot
	var hasSnapshot bool
	var persistenceErr error
	var deferredTerminal json.RawMessage
	var gate userInputStreamGate
	forward := func(data json.RawMessage) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if eventCh == nil {
			return nil
		}
		select {
		case eventCh <- data:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for event := range stream {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		held := gate.hold(event, data)
		if !stored && event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				lastSnapshot = snap
				hasSnapshot = true
				_, stored, persistenceErr = r.persistTerminalSnapshotWithStatus(
					context.WithoutCancel(ctx),
					req,
					resolvedContext{model: models.GetResponse{ID: modelID}},
					snap,
				)
				if persistenceErr != nil && !stored && !held {
					deferredTerminal = append(json.RawMessage(nil), data...)
					continue
				}
			}
		}
		if held {
			continue
		}
		if err := forward(data); err != nil {
			return err
		}
	}
	if !stored && hasSnapshot && dbstore.IsPersistenceRetrySafe(persistenceErr) {
		_, stored, persistenceErr = r.persistTerminalSnapshotWithStatus(
			context.WithoutCancel(ctx),
			req,
			resolvedContext{model: models.GetResponse{ID: modelID}},
			lastSnapshot,
		)
	}
	if stored && persistenceErr != nil {
		r.logger.Error("continuation post-commit persistence side effect failed", slog.Any("error", persistenceErr))
		persistenceErr = nil
	}
	if persistenceErr != nil && !stored {
		return persistenceErr
	}
	if len(deferredTerminal) > 0 {
		if err := forward(deferredTerminal); err != nil {
			return err
		}
	}
	if gate.active {
		if !stored {
			return errors.New("ask_user terminal response was not persisted")
		}
		if err := gate.release(forward); err != nil {
			return err
		}
	}
	return persistenceErr
}
