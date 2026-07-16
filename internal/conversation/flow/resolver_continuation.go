package flow

import (
	"context"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/historyfrag"
)

func (r *Resolver) prepareContinuationRunConfig(
	ctx context.Context,
	base agent.RunConfig,
	fallback historyfrag.ScopeFallback,
	summaryScope contextfrag.Scope,
	eventCh chan<- WSStreamEvent,
) (agent.RunConfig, error) {
	loaded, err := r.loadHistoryRecords(ctx, fallback, summaryScope.SessionID, defaultMaxContextMinutes)
	if err != nil {
		return agent.RunConfig{}, err
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded, err = r.replaceCompactedMessages(ctx, summaryScope.SessionID, summaryScope, loaded, compactionArtifactBoundary{})
	if err != nil {
		return agent.RunConfig{}, err
	}
	messages, retained, _ := trimMessagesAndRecordsByTokens(r.logger, loaded, 0)
	messages = sanitizeMessages(messages)

	base.ContextFrags = historyContextFragsForMessages(messages, retained)
	// Close any tool call left open by an interrupted turn before the transcript
	// reaches providers that enforce strict assistant-tool adjacency. A process
	// restart can orphan a deferred ask_user / tool-approval call while a later
	// request still completes normally; repairing here (not in ContextFrags)
	// keeps the fragments faithful to history while the outgoing messages stay
	// provider-valid. Applies to every continuation path that resumes after a
	// deferred tool call.
	base.Messages = modelMessagesToSDKMessages(repairToolCallClosures(nonNilModelMessages(messages), syntheticToolClosureError))
	base.Query = ""
	base.LiveToolStream = eventCh != nil
	base.CanRequestUserInput = r.canDeliverUserInputWS(eventCh)
	return r.prepareRunConfig(ctx, base), nil
}
