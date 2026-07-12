package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextassembly"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/messageconv"
)

func (r *Resolver) prepareContinuationRunConfig(
	ctx context.Context,
	req conversation.ChatRequest,
	base agent.RunConfig,
	fallback historyfrag.ScopeFallback,
	summaryScope contextfrag.Scope,
	eventCh chan<- WSStreamEvent,
	continuationToolCallID string,
	contextTokenBudget int,
) (resolvedContext, error) {
	built, err := r.buildContinuationHistoryContext(ctx, fallback, summaryScope, continuationToolCallID)
	if err != nil {
		return resolvedContext{}, err
	}
	built, attempted, err := r.drainContinuationHistoryContext(
		ctx,
		req,
		fallback,
		summaryScope,
		continuationToolCallID,
		built,
		contextTokenBudget,
	)
	if err != nil {
		return resolvedContext{}, err
	}
	projection := built.Projection
	plan, err := buildInitialPromptPlan(projection, nil, contextTokenBudget)
	if err != nil {
		return resolvedContext{}, err
	}
	baseline, err := plan.BaselineMessages()
	if err != nil {
		return resolvedContext{}, err
	}
	messages := sdkMessagesToModelMessages(baseline)

	base.ContextFrags = historyContextFragsForMessages(messages, built.HistoryRecords)
	base.Messages = baseline
	base.Query = ""
	base.LiveToolStream = eventCh != nil
	base.CanRequestUserInput = r.canDeliverUserInputWS(eventCh)
	base, state := withInitialPromptMaterializer(base, plan, projection)
	if attempted {
		state.ClaimCompaction()
	}
	base = r.prepareRunConfig(ctx, base)
	return resolvedContext{
		runConfig:              base,
		compactableTokens:      built.Allocation.CompactableTokens,
		compactableTokensKnown: true,
		contextTokenBudget:     contextTokenBudget,
		promptState:            state,
	}, nil
}

func (r *Resolver) buildContinuationHistoryContext(
	ctx context.Context,
	fallback historyfrag.ScopeFallback,
	summaryScope contextfrag.Scope,
	continuationToolCallID string,
) (historyContextBuild, error) {
	loaded, err := r.loadHistoryRecords(ctx, fallback, summaryScope.SessionID, defaultMaxContextMinutes)
	if err != nil {
		return historyContextBuild{}, err
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded, err = r.replaceCompactedMessages(ctx, summaryScope.SessionID, summaryScope, loaded)
	if err != nil {
		return historyContextBuild{}, err
	}
	built, err := assembleHistoryContext(r.logger, loaded, nil)
	if err != nil {
		return historyContextBuild{}, err
	}
	built.Projection, err = requireContinuationToolOccurrence(built.Projection, continuationToolCallID)
	if err != nil {
		return historyContextBuild{}, err
	}
	return built, nil
}

func (r *Resolver) drainContinuationHistoryContext(
	ctx context.Context,
	req conversation.ChatRequest,
	fallback historyfrag.ScopeFallback,
	summaryScope contextfrag.Scope,
	continuationToolCallID string,
	initial historyContextBuild,
	contextTokenBudget int,
) (historyContextBuild, bool, error) {
	drained, _, attempted, err := drainPreSendCompaction(
		ctx,
		initial,
		initial.Allocation.CompactableTokens,
		preSendCompactionThreshold(contextTokenBudget),
		func(ctx context.Context, pressure int) (compaction.Result, error) {
			return r.runCompactionSyncResult(ctx, req, pressure, contextTokenBudget)
		},
		func(ctx context.Context) (historyContextBuild, int, error) {
			rebuilt, err := r.buildContinuationHistoryContext(ctx, fallback, summaryScope, continuationToolCallID)
			return rebuilt, rebuilt.Allocation.CompactableTokens, err
		},
	)
	return drained, attempted, err
}

func requireContinuationToolOccurrence(
	projection budgetSourceProjection,
	toolCallID string,
) (budgetSourceProjection, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return projection, nil
	}
	messages := make([]sdk.Message, len(projection.sources))
	for index, source := range projection.sources {
		messages[index] = source.Message
	}
	callIndex := -1
	resultIndex := -1
	for _, match := range messageconv.AnalyzeSDKToolOccurrences(messages).Matches {
		if match.CallID != toolCallID {
			continue
		}
		callIndex = match.CallCarrierIndex
		resultIndex = match.ResultCarrierIndex
	}
	if callIndex < 0 || resultIndex < 0 {
		return budgetSourceProjection{}, fmt.Errorf("continuation tool occurrence %q was not found", toolCallID)
	}
	projection.sources = append([]contextassembly.Source(nil), projection.sources...)
	projection.sources[callIndex].Retention = contextbudget.RetentionRequired
	projection.sources[resultIndex].Retention = contextbudget.RetentionRequired
	return projection, nil
}

func (r *Resolver) streamContinuation(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
	eventCh chan<- WSStreamEvent,
) error {
	stream := r.agent.Stream(ctx, rc.runConfig)
	return r.consumeContinuationStream(ctx, req, rc, eventCh, stream)
}

func (r *Resolver) consumeContinuationStream(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
	eventCh chan<- WSStreamEvent,
	stream <-chan agent.StreamEvent,
) error {
	stored := false
	for event := range stream {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if !stored && event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				if err := r.persistTerminalSnapshot(context.WithoutCancel(ctx), req, rc, snap); err != nil {
					r.maybeCompactContinuation(ctx, req, rc)
					return err
				}
				stored = true
			}
		}
		if eventCh != nil && shouldForwardAgentStreamEvent(rc, event) {
			select {
			case eventCh <- json.RawMessage(data):
			case <-ctx.Done():
				if !stored {
					r.maybeCompactContinuation(ctx, req, rc)
				}
				return ctx.Err()
			}
		}
	}
	if !stored {
		r.maybeCompactContinuation(ctx, req, rc)
	}
	return rc.promptMaterializationError()
}

func (r *Resolver) maybeCompactContinuation(ctx context.Context, req conversation.ChatRequest, rc resolvedContext) {
	if pressure, known, claimed := rc.claimCompactionPressure(); claimed && known && pressure > 0 {
		r.maybeCompact(context.WithoutCancel(ctx), req, rc, pressure)
	}
}
