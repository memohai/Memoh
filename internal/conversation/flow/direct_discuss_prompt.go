package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/contextassembly"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messageconv"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type directDiscussPromptPreparer struct {
	resolver           *Resolver
	runConfig          agent.RunConfig
	contextTokenBudget int
	compact            func(context.Context, conversation.ChatRequest, resolvedContext, int)
}

func (p *directDiscussPromptPreparer) PrepareDirectDiscussPrompt(
	ctx context.Context,
	recipe pipelinepkg.DirectDiscussPromptRecipe,
) (pipelinepkg.PreparedDirectDiscussPrompt, error) {
	if p == nil {
		return pipelinepkg.PreparedDirectDiscussPrompt{}, errors.New("direct discuss prompt preparer is nil")
	}
	snapshot, err := buildDirectDiscussPromptSnapshot(recipe.Initial)
	if err != nil {
		return pipelinepkg.PreparedDirectDiscussPrompt{}, err
	}
	request := conversation.ChatRequest{
		BotID:     p.runConfig.Identity.BotID,
		SessionID: p.runConfig.Identity.SessionID,
		UserID:    strings.TrimSpace(recipe.Initial.ActorUserID),
	}
	attempted := false
	if recipe.Rebuild != nil {
		snapshot, _, attempted, err = drainPreSendCompaction(
			ctx,
			snapshot,
			snapshot.compactableTokens,
			preSendCompactionThreshold(p.contextTokenBudget),
			func(ctx context.Context, pressure int) (compaction.Result, error) {
				if p.resolver == nil {
					return compaction.Result{Status: compaction.StatusNoop}, nil
				}
				return p.resolver.runCompactionSyncResult(ctx, request, pressure, p.contextTokenBudget)
			},
			func(ctx context.Context) (directDiscussPromptSnapshot, int, error) {
				input, rebuildErr := recipe.Rebuild(ctx)
				if rebuildErr != nil {
					return directDiscussPromptSnapshot{}, 0, rebuildErr
				}
				rebuilt, rebuildErr := buildDirectDiscussPromptSnapshot(input)
				return rebuilt, rebuilt.compactableTokens, rebuildErr
			},
		)
		if err != nil {
			return pipelinepkg.PreparedDirectDiscussPrompt{}, err
		}
	}
	projection := snapshot.projection
	nativeParts := p.resolveDirectDiscussNativeParts(ctx, snapshot.imageRefs)
	plan, err := newInitialPromptPlan(initialPromptPlanInput{
		Sources:         projection.sources,
		CurrentSourceID: projection.currentSourceID,
		ContextBudget:   p.contextTokenBudget,
		Notice:          historyTruncationNotice().TextContent(),
		StripTools:      len(projection.sources) > 10,
		NativeParts:     nativeParts,
	})
	if err != nil {
		return pipelinepkg.PreparedDirectDiscussPrompt{}, err
	}
	baseline, err := plan.BaselineMessages()
	if err != nil {
		return pipelinepkg.PreparedDirectDiscussPrompt{}, err
	}
	cfg := p.runConfig
	cfg.Messages = baseline
	cfg.SessionType = sessionpkg.TypeDiscuss
	cfg.Query = ""
	cfg.InlineImages = nil
	cfg.ContextQueryMaterialized = false
	cfg, state := withInitialPromptMaterializer(cfg, plan, projection)
	if attempted {
		state.ClaimCompaction()
	}
	cfg = cfg.RefreshContextFrag()

	rc := resolvedContext{
		compactableTokens:      snapshot.compactableTokens,
		compactableTokensKnown: true,
		contextTokenBudget:     p.contextTokenBudget,
		promptState:            state,
	}
	req := conversation.ChatRequest{
		BotID:     cfg.Identity.BotID,
		SessionID: cfg.Identity.SessionID,
		UserID:    strings.TrimSpace(snapshot.input.ActorUserID),
	}
	receipt := &directDiscussPromptReceipt{
		resolved: rc,
		compact: func(finishCtx context.Context, pressure int) {
			if p.compact != nil {
				p.compact(finishCtx, req, rc, pressure)
			}
		},
	}
	return pipelinepkg.PreparedDirectDiscussPrompt{RunConfig: cfg, Receipt: receipt}, nil
}

type directDiscussPromptSnapshot struct {
	input             pipelinepkg.DirectDiscussPromptInput
	projection        budgetSourceProjection
	imageRefs         map[string][]pipelinepkg.ImageAttachmentRef
	compactableTokens int
}

func buildDirectDiscussPromptSnapshot(input pipelinepkg.DirectDiscussPromptInput) (directDiscussPromptSnapshot, error) {
	projection, imageRefs, err := projectDirectDiscussPromptSources(input)
	if err != nil {
		return directDiscussPromptSnapshot{}, err
	}
	compactableTokens := 0
	for _, source := range projection.sources {
		compactableTokens += max(source.CompactableTokens, 0)
	}
	return directDiscussPromptSnapshot{
		input:             input,
		projection:        projection,
		imageRefs:         imageRefs,
		compactableTokens: compactableTokens,
	}, nil
}

func projectDirectDiscussPromptSources(
	input pipelinepkg.DirectDiscussPromptInput,
) (budgetSourceProjection, map[string][]pipelinepkg.ImageAttachmentRef, error) {
	projection := budgetSourceProjection{
		sources:       make([]contextassembly.Source, len(input.Sources)),
		sourceFrags:   make([]contextfrag.ContextFrag, len(input.Sources)),
		hasSourceFrag: make([]bool, len(input.Sources)),
	}
	imageRefs := make(map[string][]pipelinepkg.ImageAttachmentRef)
	seen := make(map[string]struct{}, len(input.Sources))
	for index, inputSource := range input.Sources {
		sourceID := strings.TrimSpace(inputSource.ID)
		if sourceID == "" {
			return budgetSourceProjection{}, nil, fmt.Errorf("direct discuss prompt source %d has no identity", index)
		}
		if _, exists := seen[sourceID]; exists {
			return budgetSourceProjection{}, nil, fmt.Errorf("direct discuss prompt source %q is not unique", sourceID)
		}
		seen[sourceID] = struct{}{}
		message, err := clonePromptMessage(messageconv.CanonicalSDKMessage(inputSource.Message))
		if err != nil {
			return budgetSourceProjection{}, nil, fmt.Errorf("clone direct discuss prompt source %q: %w", sourceID, err)
		}
		retention := contextbudget.RetentionCandidate
		if inputSource.Required {
			retention = contextbudget.RetentionRequired
		}
		projection.sources[index] = contextassembly.Source{
			ID:        sourceID,
			Message:   message,
			Retention: retention,
		}
		if inputSource.Compactable {
			projection.sources[index].CompactableTokens = messageconv.EstimateSDKMessageTokens(message)
		}
		if inputSource.SummaryFrag != nil {
			if !inputSource.Required || inputSource.Compactable {
				return budgetSourceProjection{}, nil, fmt.Errorf("direct discuss summary source %q must be required and non-compactable", sourceID)
			}
			projection.sourceFrags[index] = *inputSource.SummaryFrag
			projection.hasSourceFrag[index] = true
		}
		if len(inputSource.ImageRefs) > 0 {
			imageRefs[sourceID] = append([]pipelinepkg.ImageAttachmentRef(nil), inputSource.ImageRefs...)
		}
	}
	projection.currentSourceID = strings.TrimSpace(input.CurrentSourceID)
	return projection, imageRefs, nil
}

func (p *directDiscussPromptPreparer) resolveDirectDiscussNativeParts(
	ctx context.Context,
	imageRefs map[string][]pipelinepkg.ImageAttachmentRef,
) map[string][]sdk.MessagePart {
	if p == nil || p.resolver == nil || !p.runConfig.SupportsImageInput || len(imageRefs) == 0 {
		return nil
	}
	partsBySource := make(map[string][]sdk.MessagePart, len(imageRefs))
	for sourceID, refs := range imageRefs {
		images := p.resolver.InlineImageAttachments(ctx, p.runConfig.Identity.BotID, refs)
		for _, image := range images {
			if strings.TrimSpace(image.Image) != "" {
				partsBySource[sourceID] = append(partsBySource[sourceID], image)
			}
		}
	}
	return partsBySource
}

type directDiscussPromptReceipt struct {
	resolved resolvedContext
	compact  func(context.Context, int)
}

func (r *directDiscussPromptReceipt) Finish(ctx context.Context) error {
	if r == nil {
		return nil
	}
	materializationErr := r.resolved.promptMaterializationError()
	pressure, known, claimed := r.resolved.claimCompactionPressure()
	if claimed && known && pressure > 0 && r.compact != nil {
		r.compact(context.WithoutCancel(ctx), pressure)
	}
	return materializationErr
}
