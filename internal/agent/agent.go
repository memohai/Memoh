package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

// Agent is the core agent that handles LLM interactions.
type Agent struct {
	client         *sdk.Client
	toolProviders  []tools.ToolProvider
	bridgeProvider bridge.Provider
	logger         *slog.Logger
}

// New creates a new Agent with the given dependencies.
func New(deps Deps) *Agent {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Agent{
		client:         sdk.NewClient(),
		bridgeProvider: deps.BridgeProvider,
		logger:         logger.With(slog.String("service", "agent")),
	}
}

// BridgeProvider returns the underlying bridge provider (workspace manager).
func (a *Agent) BridgeProvider() bridge.Provider {
	return a.bridgeProvider
}

// SetToolProviders sets the tool providers after construction.
// This allows breaking dependency cycles in the DI graph.
func (a *Agent) SetToolProviders(providers []tools.ToolProvider) {
	a.toolProviders = providers
}

// Stream runs the agent in streaming mode, emitting events to the returned channel.
func (a *Agent) Stream(ctx context.Context, cfg RunConfig) <-chan StreamEvent {
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)
		a.runStream(ctx, cfg, ch)
	}()
	return ch
}

// Generate runs the agent in non-streaming mode, returning the complete result.
func (a *Agent) Generate(ctx context.Context, cfg RunConfig) (*GenerateResult, error) {
	return a.runGenerate(ctx, cfg)
}

func (a *Agent) ExecuteTool(ctx context.Context, cfg RunConfig, call sdk.ToolCall) (sdk.ToolResultPart, error) {
	sdkTools, err := a.assembleTools(ctx, cfg, tools.StreamEmitter(func(tools.ToolStreamEvent) {}))
	if err != nil {
		return sdk.ToolResultPart{}, fmt.Errorf("assemble tools: %w", err)
	}
	for i := range sdkTools {
		tool := sdkTools[i]
		if tool.Name != call.ToolName {
			continue
		}
		if tool.Execute == nil {
			return sdk.ToolResultPart{}, fmt.Errorf("tool %q has no execute handler", call.ToolName)
		}
		execCtx := &sdk.ToolExecContext{
			Context:    ctx,
			ToolCallID: call.ToolCallID,
			ToolName:   call.ToolName,
		}
		output, err := tool.Execute(execCtx, call.Input)
		if err != nil {
			return sdk.ToolResultPart{
				ToolCallID: call.ToolCallID,
				ToolName:   call.ToolName,
				Result:     err.Error(),
				IsError:    true,
			}, nil
		}
		return sdk.ToolResultPart{
			ToolCallID: call.ToolCallID,
			ToolName:   call.ToolName,
			Result:     output,
		}, nil
	}
	return sdk.ToolResultPart{}, fmt.Errorf("tool %q not found", call.ToolName)
}

// sendEvent sends an event to the stream channel. It returns false if the
// context was cancelled (consumer stopped reading), allowing the caller to
// abort cleanly instead of leaking the goroutine on a blocked channel send.
func sendEvent(ctx context.Context, ch chan<- StreamEvent, evt StreamEvent) bool {
	select {
	case ch <- evt:
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *Agent) runStream(ctx context.Context, cfg RunConfig, ch chan<- StreamEvent) {
	streamCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// Stream emitter: tools targeting the current conversation push
	// side-effect events (attachments, reactions, speech) directly here.
	// Uses sendEvent to avoid goroutine leaks when the consumer stops reading.
	streamEmitter := tools.StreamEmitter(func(evt tools.ToolStreamEvent) {
		sendEvent(ctx, ch, toolStreamEventToAgentEvent(evt))
	})

	var sdkTools []sdk.Tool
	if cfg.SupportsToolCall {
		var err error
		sdkTools, err = a.assembleTools(streamCtx, cfg, streamEmitter)
		if err != nil {
			sendEvent(ctx, ch, StreamEvent{Type: EventError, Error: fmt.Sprintf("assemble tools: %v", err)})
			return
		}
	}
	sdkTools, readMediaState := decorateReadMediaTools(cfg.Model, sdkTools)

	aborted := false

	// Loop detection setup
	var textLoopGuard *TextLoopGuard
	var textLoopProbeBuffer *TextLoopProbeBuffer
	var toolLoopGuard *ToolLoopGuard
	toolLoopAbortCallIDs := newToolAbortRegistry()
	if cfg.LoopDetection.Enabled {
		textLoopGuard = NewTextLoopGuard(LoopDetectedStreakThreshold, LoopDetectedMinNewGramsPerChunk, SentialOptions{})
		textLoopProbeBuffer = NewTextLoopProbeBuffer(LoopDetectedProbeChars, func(text string) {
			result := textLoopGuard.Inspect(text)
			if result.Abort {
				a.logger.Warn("text loop detected, will abort")
				aborted = true
				cancel(ErrTextLoopDetected)
			}
		})
		toolLoopGuard = NewToolLoopGuard(ToolLoopRepeatThreshold, ToolLoopWarningsBeforeAbort)
	}

	// Wrap tools with loop detection
	if toolLoopGuard != nil {
		sdkTools = wrapToolsWithLoopGuard(sdkTools, toolLoopGuard, toolLoopAbortCallIDs)
	}

	var prepareStep func(*sdk.GenerateParams) *sdk.GenerateParams
	if readMediaState != nil {
		prepareStep = readMediaState.prepareStep
	}

	initialMsgCount := len(cfg.Messages)

	if cfg.InjectCh != nil {
		basePrepare := prepareStep
		prepareStep = func(p *sdk.GenerateParams) *sdk.GenerateParams {
			if basePrepare != nil {
				if override := basePrepare(p); override != nil {
					p = override
				}
			}
			for {
				select {
				case injected, ok := <-cfg.InjectCh:
					if !ok {
						break
					}
					text := strings.TrimSpace(injected.HeaderifiedText)
					if text == "" {
						text = strings.TrimSpace(injected.Text)
					}
					if text != "" || (cfg.SupportsImageInput && len(injected.ImageParts) > 0) {
						insertAfter := len(p.Messages) - initialMsgCount
						var extra []sdk.MessagePart
						if cfg.SupportsImageInput {
							for _, img := range injected.ImageParts {
								if strings.TrimSpace(img.Image) != "" {
									extra = append(extra, img)
								}
							}
						}
						p.Messages = append(p.Messages, sdk.UserMessage(text, extra...))
						if cfg.InjectedRecorder != nil {
							cfg.InjectedRecorder(text, insertAfter)
						}
						a.logger.Info("injected user message into agent stream",
							slog.String("bot_id", cfg.Identity.BotID),
							slog.Int("insert_after", insertAfter),
							slog.Int("image_parts", len(extra)),
						)
					}
					continue
				default:
				}
				break
			}
			return p
		}
	}

	// Drain background task notifications at step boundaries.
	// Each notification is injected as a user message so the model
	// discovers completed background work naturally.
	if cfg.BackgroundManager != nil {
		basePrepare := prepareStep
		baseSystem := cfg.System // capture original system prompt to avoid accumulation
		prepareStep = func(p *sdk.GenerateParams) *sdk.GenerateParams {
			if basePrepare != nil {
				if override := basePrepare(p); override != nil {
					p = override
				}
			}
			p = drainBackgroundNotifications(p, cfg.BackgroundManager, baseSystem, cfg.Identity.BotID, cfg.Identity.SessionID, a.logger)
			return p
		}
	}

	opts := a.buildGenerateOptions(cfg, sdkTools, prepareStep)

	retryCfg := cfg.Retry
	if retryCfg.MaxAttempts <= 0 {
		retryCfg = DefaultRetryConfig()
	}

	var streamResult *sdk.StreamResult
	for attempt := 0; attempt < retryCfg.MaxAttempts; attempt++ {
		var err error
		streamResult, err = a.client.StreamText(streamCtx, opts...)
		if err == nil {
			break
		}
		if !isRetryableStreamError(err) {
			sendEvent(ctx, ch, StreamEvent{Type: EventError, Error: fmt.Sprintf("stream start: %v", err)})
			return
		}
		a.logger.Warn("stream start failed, retrying",
			slog.Int("attempt", attempt+1),
			slog.Int("max_attempts", retryCfg.MaxAttempts),
			slog.String("error", err.Error()),
		)
		if !sendEvent(ctx, ch, StreamEvent{
			Type:       EventRetry,
			Attempt:    attempt + 1,
			MaxAttempt: retryCfg.MaxAttempts,
			RetryError: err.Error(),
		}) {
			return
		}
		if attempt+1 >= retryCfg.MaxAttempts {
			sendEvent(ctx, ch, StreamEvent{Type: EventError, Error: fmt.Sprintf("stream start: all %d attempts failed (last: %v)", retryCfg.MaxAttempts, err)})
			return
		}
		delay := retryDelay(attempt, retryCfg)
		if delay > 0 {
			if err := sleepWithContext(streamCtx, delay); err != nil {
				sendEvent(ctx, ch, StreamEvent{Type: EventError, Error: fmt.Sprintf("stream start: context cancelled during retry: %v", err)})
				return
			}
		}
	}

	sendEvent(ctx, ch, StreamEvent{Type: EventAgentStart})

	var allText strings.Builder
	stepNumber := 0

	for part := range streamResult.Stream {
		if streamCtx.Err() != nil {
			aborted = true
			break
		}

		switch p := part.(type) {
		case *sdk.StartPart:
			_ = p // stream start already emitted

		case *sdk.TextStartPart:
			if !sendEvent(ctx, ch, StreamEvent{Type: EventTextStart}) {
				aborted = true
			}

		case *sdk.TextDeltaPart:
			if p.Text != "" {
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Push(p.Text)
				}
				if !sendEvent(ctx, ch, StreamEvent{Type: EventTextDelta, Delta: p.Text}) {
					aborted = true
				}
				allText.WriteString(p.Text)
			}

		case *sdk.TextEndPart:
			if textLoopProbeBuffer != nil {
				textLoopProbeBuffer.Flush()
			}
			stepNumber++
			if !sendEvent(ctx, ch, StreamEvent{Type: EventTextEnd}) ||
				!sendEvent(ctx, ch, StreamEvent{
					Type:           EventProgress,
					StepNumber:     stepNumber,
					ProgressStatus: "text",
				}) {
				aborted = true
			}

		case *sdk.ReasoningStartPart:
			if !sendEvent(ctx, ch, StreamEvent{Type: EventReasoningStart}) {
				aborted = true
			}

		case *sdk.ReasoningDeltaPart:
			if !sendEvent(ctx, ch, StreamEvent{Type: EventReasoningDelta, Delta: p.Text}) {
				aborted = true
			}

		case *sdk.ReasoningEndPart:
			if !sendEvent(ctx, ch, StreamEvent{Type: EventReasoningEnd}) {
				aborted = true
			}

		case *sdk.ToolInputStartPart:
			// ToolInputStartPart fires before tool input args have streamed.
			// We suppress it here because downstream consumers (IM adapters and
			// Web UI) only care about the fully-assembled call announced by
			// StreamToolCallPart below. Emitting a start event twice for the
			// same CallID would produce duplicate "running" messages in IMs.
			if textLoopProbeBuffer != nil {
				textLoopProbeBuffer.Flush()
			}

		case *sdk.StreamToolCallPart:
			if textLoopProbeBuffer != nil {
				textLoopProbeBuffer.Flush()
			}
			if !sendEvent(ctx, ch, StreamEvent{
				Type:       EventToolCallStart,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Input:      p.Input,
			}) {
				aborted = true
			}

		case *sdk.ToolProgressPart:
			if !sendEvent(ctx, ch, StreamEvent{
				Type:       EventToolCallProgress,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Progress:   p.Content,
			}) {
				aborted = true
			}

		case *sdk.ToolApprovalRequestPart:
			if !sendEvent(ctx, ch, StreamEvent{
				Type:       EventToolApprovalRequest,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				ApprovalID: p.ApprovalID,
				ShortID:    approvalShortID(p.Metadata),
				Status:     "pending",
				Input:      p.Input,
				Metadata:   p.Metadata,
			}) {
				aborted = true
			}

		case *sdk.StreamToolResultPart:
			shouldAbort := toolLoopAbortCallIDs.Take(p.ToolCallID)
			stepNumber++
			if !sendEvent(ctx, ch, StreamEvent{
				Type:       EventToolCallEnd,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Input:      p.Input,
				Result:     p.Output,
			}) || !sendEvent(ctx, ch, StreamEvent{
				Type:           EventProgress,
				StepNumber:     stepNumber,
				ToolName:       p.ToolName,
				ProgressStatus: "tool_result",
			}) {
				aborted = true
			}
			if shouldAbort {
				a.logger.Warn("tool loop abort triggered", slog.String("tool_call_id", p.ToolCallID))
				cancel(ErrToolLoopDetected)
				aborted = true
			}

		case *sdk.StreamToolErrorPart:
			// Take before errors.Is so registry IDs from the loop guard are always cleared.
			tookLoopAbort := toolLoopAbortCallIDs.Take(p.ToolCallID)
			shouldAbort := errors.Is(p.Error, ErrToolLoopDetected) || tookLoopAbort
			if !sendEvent(ctx, ch, StreamEvent{
				Type:       EventToolCallEnd,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Error:      p.Error.Error(),
			}) {
				aborted = true
			}
			if shouldAbort {
				a.logger.Warn("tool loop abort triggered", slog.String("tool_call_id", p.ToolCallID))
				cancel(ErrToolLoopDetected)
				aborted = true
			}

		case *sdk.StreamFilePart:
			mediaType := p.File.MediaType
			if mediaType == "" {
				mediaType = "image/png"
			}
			if !sendEvent(ctx, ch, StreamEvent{
				Type: EventAttachment,
				Attachments: []FileAttachment{{
					Type: "image",
					URL:  fmt.Sprintf("data:%s;base64,%s", mediaType, p.File.Data),
					Mime: mediaType,
				}},
			}) {
				aborted = true
			}

		case *sdk.ErrorPart:
			errMsg := p.Error.Error()
			sendEvent(ctx, ch, StreamEvent{Type: EventError, Error: errMsg})

			// Mid-stream retry: if the error is retryable, attempt to continue
			// the agent run from the accumulated state. This also handles
			// errors at step 0 (e.g. timeout awaiting response headers) since
			// no work has been completed yet and retrying from the start is safe.
			if isRetryableStreamError(p.Error) {
				streamResult, aborted = a.runMidStreamRetry(
					ctx, streamCtx, cancel, toolLoopAbortCallIDs,
					ch, cfg, sdkTools, prepareStep, streamResult,
					stepNumber, errMsg, &allText, textLoopProbeBuffer,
				)
			} else {
				aborted = true
			}

		case *sdk.AbortPart:
			aborted = true

		case *sdk.FinishPart:
			// handled after loop
		}

		if aborted {
			break
		}
	}

	if aborted {
		for range streamResult.Stream {
		}
	}

	if textLoopProbeBuffer != nil {
		textLoopProbeBuffer.Flush()
	}

	finalMessages := streamResult.Messages
	if readMediaState != nil {
		finalMessages = readMediaState.mergeMessages(streamResult.Steps, finalMessages)
	}
	if streamResult.DeferredToolApproval != nil {
		finalMessages = annotateDeferredApproval(finalMessages, *streamResult.DeferredToolApproval)
	}
	var totalUsage sdk.Usage
	for _, step := range streamResult.Steps {
		totalUsage.InputTokens += step.Usage.InputTokens
		totalUsage.OutputTokens += step.Usage.OutputTokens
		totalUsage.TotalTokens += step.Usage.TotalTokens
		totalUsage.ReasoningTokens += step.Usage.ReasoningTokens
		totalUsage.CachedInputTokens += step.Usage.CachedInputTokens
		totalUsage.InputTokenDetails.NoCacheTokens += step.Usage.InputTokenDetails.NoCacheTokens
		totalUsage.InputTokenDetails.CacheReadTokens += step.Usage.InputTokenDetails.CacheReadTokens
		totalUsage.InputTokenDetails.CacheWriteTokens += step.Usage.InputTokenDetails.CacheWriteTokens
		totalUsage.OutputTokenDetails.TextTokens += step.Usage.OutputTokenDetails.TextTokens
		totalUsage.OutputTokenDetails.ReasoningTokens += step.Usage.OutputTokenDetails.ReasoningTokens
	}
	usageJSON, _ := json.Marshal(totalUsage)

	termEvent := StreamEvent{
		Messages: mustMarshal(finalMessages),
		Usage:    usageJSON,
	}
	if streamResult.DeferredToolApproval != nil {
		termEvent.ApprovalID = streamResult.DeferredToolApproval.ApprovalID
		termEvent.ShortID = approvalShortID(streamResult.DeferredToolApproval.Metadata)
		termEvent.Status = "pending"
		termEvent.Metadata = streamResult.DeferredToolApproval.Metadata
		if toolName, ok := streamResult.DeferredToolApproval.Metadata["tool_name"].(string); ok {
			termEvent.ToolName = toolName
		}
		if toolCallID, ok := streamResult.DeferredToolApproval.Metadata["tool_call_id"].(string); ok {
			termEvent.ToolCallID = toolCallID
		}
	}
	if aborted {
		termEvent.Type = EventAgentAbort
	} else {
		termEvent.Type = EventAgentEnd
		// Warn if LLM produced no text and no tool calls — likely a context overflow.
		if allText.Len() == 0 && stepNumber == 0 {
			a.logger.Warn("agent produced empty response (no text, no tool calls)",
				slog.String("bot_id", cfg.Identity.BotID),
				slog.Int("input_messages", len(cfg.Messages)),
				slog.Int("input_tokens", totalUsage.InputTokens),
			)
		}
	}
	sendEvent(ctx, ch, termEvent)
}

func (a *Agent) runGenerate(ctx context.Context, cfg RunConfig) (*GenerateResult, error) {
	genCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	loopAbort := newLoopAbortState()

	// Collecting emitter: tools push side-effect events here during generation.
	collected := newToolEventCollector()
	defer collected.Close()
	collectEmitter := tools.StreamEmitter(func(evt tools.ToolStreamEvent) {
		collected.Add(evt)
	})

	var sdkTools []sdk.Tool
	if cfg.SupportsToolCall {
		var err error
		sdkTools, err = a.assembleTools(genCtx, cfg, collectEmitter)
		if err != nil {
			return nil, fmt.Errorf("assemble tools: %w", err)
		}
	}
	sdkTools, readMediaState := decorateReadMediaTools(cfg.Model, sdkTools)

	var toolLoopGuard *ToolLoopGuard
	var textLoopGuard *TextLoopGuard
	toolLoopAbortCallIDs := newToolAbortRegistry()
	if cfg.LoopDetection.Enabled {
		toolLoopGuard = NewToolLoopGuard(ToolLoopRepeatThreshold, ToolLoopWarningsBeforeAbort)
		textLoopGuard = NewTextLoopGuard(LoopDetectedStreakThreshold, LoopDetectedMinNewGramsPerChunk, SentialOptions{})
	}

	if toolLoopGuard != nil {
		sdkTools = wrapToolsWithLoopGuard(sdkTools, toolLoopGuard, toolLoopAbortCallIDs)
	}

	var prepareStep func(*sdk.GenerateParams) *sdk.GenerateParams
	if readMediaState != nil {
		prepareStep = readMediaState.prepareStep
	}

	// Drain background task notifications at step boundaries (non-streaming).
	if cfg.BackgroundManager != nil {
		basePrepare := prepareStep
		baseSystem := cfg.System
		prepareStep = func(p *sdk.GenerateParams) *sdk.GenerateParams {
			if basePrepare != nil {
				if override := basePrepare(p); override != nil {
					p = override
				}
			}
			p = drainBackgroundNotifications(p, cfg.BackgroundManager, baseSystem, cfg.Identity.BotID, cfg.Identity.SessionID, a.logger)
			return p
		}
	}

	opts := a.buildGenerateOptions(cfg, sdkTools, prepareStep)
	opts = append(opts,
		sdk.WithOnStep(func(step *sdk.StepResult) *sdk.GenerateParams {
			if cfg.LoopDetection.Enabled {
				if toolLoopAbortCallIDs.Any() {
					loopAbort.Set(ErrToolLoopDetected)
					cancel(ErrToolLoopDetected)
					return nil
				}
				if textLoopGuard != nil && isNonEmptyString(step.Text) {
					result := textLoopGuard.Inspect(step.Text)
					if result.Abort {
						loopAbort.Set(ErrTextLoopDetected)
						cancel(ErrTextLoopDetected)
						return nil
					}
				}
			}
			return nil
		}),
	)

	genResult, err := a.client.GenerateTextResult(genCtx, opts...)
	if err != nil {
		if loopErr := detectGenerateLoopAbort(genCtx, err); loopErr != nil {
			return nil, loopErr
		}
		return nil, fmt.Errorf("generate: %w", err)
	}
	if loopErr := loopAbort.Err(); loopErr != nil {
		return nil, loopErr
	}

	// Drain collected tool-emitted side effects into the result.
	collectedEvents := collected.CloseAndSnapshot()
	var attachments []FileAttachment
	var reactions []ReactionItem
	var speeches []SpeechItem
	for _, evt := range collectedEvents {
		switch evt.Type {
		case tools.StreamEventAttachment:
			for _, a := range evt.Attachments {
				attachments = append(attachments, FileAttachment{
					Type: a.Type, Path: a.Path, URL: a.URL,
					Mime: a.Mime, Name: a.Name,
					ContentHash: a.ContentHash, Size: a.Size,
					Metadata: a.Metadata,
				})
			}
		case tools.StreamEventReaction:
			for _, r := range evt.Reactions {
				reactions = append(reactions, ReactionItem{Emoji: r.Emoji})
			}
		case tools.StreamEventSpeech:
			for _, s := range evt.Speeches {
				speeches = append(speeches, SpeechItem{Text: s.Text})
			}
		}
	}

	finalMessages := genResult.Messages
	if readMediaState != nil {
		finalMessages = readMediaState.mergeMessages(genResult.Steps, finalMessages)
	}
	return &GenerateResult{
		Messages:    finalMessages,
		Text:        genResult.Text,
		Attachments: attachments,
		Reactions:   reactions,
		Speeches:    speeches,
		Usage:       &genResult.Usage,
	}, nil
}

func (*Agent) buildGenerateOptions(cfg RunConfig, tools []sdk.Tool, prepareStep func(*sdk.GenerateParams) *sdk.GenerateParams) []sdk.GenerateOption {
	opts := []sdk.GenerateOption{
		sdk.WithModel(cfg.Model),
		sdk.WithMessages(cfg.Messages),
		sdk.WithSystem(cfg.System),
		sdk.WithMaxSteps(-1),
	}
	if len(tools) > 0 && cfg.SupportsToolCall {
		opts = append(opts, sdk.WithTools(tools))
	}
	if cfg.ToolApprovalHandler != nil {
		opts = append(opts, sdk.WithApprovalHandler(cfg.ToolApprovalHandler))
	}

	// Wrap the existing prepareStep (if any) with mid-task context pruning.
	// When the message array grows large during multi-tool runs, this prunes
	// older tool results to keep the context window manageable.
	basePrepare := prepareStep
	keepSteps := cfg.MidTaskPruneKeepSteps
	if keepSteps <= 0 {
		keepSteps = MidTaskPruneKeepStepsDefault
	}
	threshold := cfg.MidTaskPruneThreshold
	if threshold <= 0 {
		threshold = MidTaskPruneThresholdDefault
	}
	midTaskPrune := func(p *sdk.GenerateParams) *sdk.GenerateParams {
		if basePrepare != nil {
			if override := basePrepare(p); override != nil {
				p = override
			}
		}
		return pruneOldToolResults(p, keepSteps, threshold)
	}
	opts = append(opts, sdk.WithPrepareStep(midTaskPrune))

	opts = append(opts, models.BuildReasoningOptions(models.SDKModelConfig{
		ClientType: models.ResolveClientType(cfg.Model),
		ReasoningConfig: &models.ReasoningConfig{
			Enabled: cfg.ReasoningEffort != "",
			Effort:  cfg.ReasoningEffort,
		},
	})...)
	return opts
}

// assembleTools collects tools from all registered ToolProviders.
// emitter is injected into the session context so that tools targeting the
// current conversation can push side-effect events (attachments, reactions,
// speech) directly into the agent stream.
func (a *Agent) assembleTools(ctx context.Context, cfg RunConfig, emitter tools.StreamEmitter) ([]sdk.Tool, error) {
	if len(a.toolProviders) == 0 {
		return nil, nil
	}
	skillsMap := make(map[string]tools.SkillDetail, len(cfg.Skills))
	for _, s := range cfg.Skills {
		skillsMap[s.Name] = tools.SkillDetail{
			Description: s.Description,
			Content:     s.Content,
			Path:        s.Path,
		}
	}
	session := tools.SessionContext{
		BotID:              cfg.Identity.BotID,
		ChatID:             cfg.Identity.ChatID,
		SessionID:          cfg.Identity.SessionID,
		SessionType:        cfg.SessionType,
		ChannelIdentityID:  cfg.Identity.ChannelIdentityID,
		SessionToken:       cfg.Identity.SessionToken,
		CurrentPlatform:    cfg.Identity.CurrentPlatform,
		ReplyTarget:        cfg.Identity.ReplyTarget,
		SupportsImageInput: cfg.SupportsImageInput,
		IsSubagent:         cfg.Identity.IsSubagent,
		Skills:             skillsMap,
		TimezoneLocation:   cfg.Identity.TimezoneLocation,
		Emitter:            emitter,
	}

	var allTools []sdk.Tool
	for _, provider := range a.toolProviders {
		providerTools, err := provider.Tools(ctx, session)
		if err != nil {
			a.logger.Warn("tool provider failed", slog.Any("error", err))
			continue
		}
		allTools = append(allTools, providerTools...)
	}
	if cfg.ToolApprovalHandler != nil {
		allTools = markApprovalTools(allTools)
	}
	return allTools, nil
}

func markApprovalTools(tools []sdk.Tool) []sdk.Tool {
	for i := range tools {
		switch tools[i].Name {
		case "write", "edit", "exec":
			tools[i].RequireApproval = true
		}
	}
	return tools
}

func approvalShortID(metadata map[string]any) int {
	if metadata == nil {
		return 0
	}
	switch v := metadata["short_id"].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func annotateDeferredApproval(messages []sdk.Message, approval sdk.ToolApprovalResult) []sdk.Message {
	if approval.ApprovalID == "" {
		return messages
	}
	toolCallID, _ := approval.Metadata["tool_call_id"].(string)
	if strings.TrimSpace(toolCallID) == "" {
		return messages
	}
	annotated := make([]sdk.Message, len(messages))
	copy(annotated, messages)
	for msgIdx := range annotated {
		if annotated[msgIdx].Role != sdk.MessageRoleAssistant {
			continue
		}
		for partIdx := range annotated[msgIdx].Content {
			call, ok := annotated[msgIdx].Content[partIdx].(sdk.ToolCallPart)
			if !ok || strings.TrimSpace(call.ToolCallID) != strings.TrimSpace(toolCallID) {
				continue
			}
			if call.ProviderMetadata == nil {
				call.ProviderMetadata = map[string]any{}
			}
			call.ProviderMetadata["approval"] = map[string]any{
				"approval_id": approval.ApprovalID,
				"short_id":    approvalShortID(approval.Metadata),
				"status":      "pending",
				"can_approve": true,
			}
			annotated[msgIdx].Content[partIdx] = call
			return annotated
		}
	}
	return annotated
}

// toolStreamEventToAgentEvent converts a tool-layer ToolStreamEvent into an
// agent-layer StreamEvent suitable for the output channel.
func toolStreamEventToAgentEvent(evt tools.ToolStreamEvent) StreamEvent {
	switch evt.Type {
	case tools.StreamEventAttachment:
		atts := make([]FileAttachment, 0, len(evt.Attachments))
		for _, a := range evt.Attachments {
			atts = append(atts, FileAttachment{
				Type: a.Type, Path: a.Path, URL: a.URL,
				Mime: a.Mime, Name: a.Name,
				ContentHash: a.ContentHash, Size: a.Size,
				Metadata: a.Metadata,
			})
		}
		return StreamEvent{Type: EventAttachment, Attachments: atts}
	case tools.StreamEventReaction:
		rs := make([]ReactionItem, 0, len(evt.Reactions))
		for _, r := range evt.Reactions {
			rs = append(rs, ReactionItem{Emoji: r.Emoji})
		}
		return StreamEvent{Type: EventReaction, Reactions: rs}
	case tools.StreamEventSpeech:
		ss := make([]SpeechItem, 0, len(evt.Speeches))
		for _, s := range evt.Speeches {
			ss = append(ss, SpeechItem{Text: s.Text})
		}
		return StreamEvent{Type: EventSpeech, Speeches: ss}
	case tools.StreamEventSpawnHeartbeat:
		return StreamEvent{Type: EventProgress, ProgressStatus: "spawn_running"}
	default:
		return StreamEvent{}
	}
}

// drainBackgroundNotifications non-blockingly drains pending background task
// notifications for the given bot+session and injects them as user messages
// into the next LLM step at step boundaries.
func drainBackgroundNotifications(
	p *sdk.GenerateParams,
	mgr *background.Manager,
	baseSystem string,
	botID, sessionID string,
	logger *slog.Logger,
) *sdk.GenerateParams {
	// Inject running tasks summary into system prompt so the model
	// knows about ongoing background work even after compaction.
	// Always start from baseSystem to avoid accumulating summaries across steps.
	if summary := mgr.RunningTasksSummary(botID, sessionID); summary != "" {
		p.System = baseSystem + "\n\n" + summary
	} else {
		p.System = baseSystem
	}

	notifications := mgr.DrainNotifications(botID, sessionID)
	for _, n := range notifications {
		p.Messages = append(p.Messages, sdk.UserMessage(n.MessageText()))
		logger.Info("injected background task notification",
			slog.String("task_id", n.TaskID),
			slog.String("status", string(n.Status)),
			slog.Bool("stalled", n.Stalled),
			slog.String("bot_id", botID),
		)
	}
	return p
}

func wrapToolsWithLoopGuard(tools []sdk.Tool, guard *ToolLoopGuard, abortCallIDs *toolAbortRegistry) []sdk.Tool {
	wrapped := make([]sdk.Tool, len(tools))
	for i, tool := range tools {
		originalExecute := tool.Execute
		toolName := tool.Name
		wrapped[i] = tool
		wrapped[i].Execute = func(ctx *sdk.ToolExecContext, input any) (any, error) {
			warn, abort := guard.Guard(toolName, input)
			if abort {
				abortCallIDs.Add(ctx.ToolCallID)
				return map[string]any{
					"isError": true,
					"content": []map[string]any{{
						"type": "text",
						"text": ToolLoopDetectedAbortMessage,
					}},
				}, ErrToolLoopDetected
			}
			if warn {
				return map[string]any{
					ToolLoopWarningKey: true,
					"content": []map[string]any{{
						"type": "text",
						"text": ToolLoopWarningText,
					}},
				}, nil
			}
			return originalExecute(ctx, input)
		}
	}
	return wrapped
}

const (
	// MidTaskPruneKeepStepsDefault is the number of recent tool-call steps to keep
	// intact when pruning older tool results during a multi-step agent run.
	MidTaskPruneKeepStepsDefault = 4
	// MidTaskPruneThresholdDefault is the minimum number of messages before pruning activates.
	MidTaskPruneThresholdDefault = 20
)

// pruneOldToolResults prunes older tool result messages in the SDK params to
// keep the context window manageable during long multi-tool agent runs. It
// keeps the most recent keepSteps tool-call cycles intact and replaces older
// tool results with size summaries.
func pruneOldToolResults(p *sdk.GenerateParams, keepSteps, threshold int) *sdk.GenerateParams {
	msgs := p.Messages
	if len(msgs) < threshold {
		return p
	}

	// Count complete tool-call cycles (tool-result pair) from the end to find the cutoff.
	toolResultCount := 0
	cutoffIdx := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == sdk.MessageRoleTool {
			// Check that the preceding assistant message contains the matching tool call
			// to ensure we count complete cycles, not orphaned results.
			hasMatchingCall := false
			for j := i - 1; j >= 0; j-- {
				if msgs[j].Role == sdk.MessageRoleAssistant {
					// If there's another tool result between this and the assistant msg,
					// it means this assistant message belongs to a different cycle.
					if j+1 < i && msgs[j+1].Role == sdk.MessageRoleTool {
						break
					}
					hasMatchingCall = true
					break
				}
				if msgs[j].Role == sdk.MessageRoleUser {
					break
				}
			}
			if hasMatchingCall {
				toolResultCount++
				if toolResultCount > keepSteps {
					cutoffIdx = i
					break
				}
			}
		}
	}
	if cutoffIdx >= len(msgs) {
		return p // not enough tool messages to prune
	}

	// Build a new slice so the original messages can be GC'd.
	pruned := make([]sdk.Message, 0, len(msgs))
	pruned = append(pruned, msgs[:cutoffIdx]...)
	for i := cutoffIdx; i < len(msgs); i++ {
		if msgs[i].Role != sdk.MessageRoleTool {
			pruned = append(pruned, msgs[i])
			continue
		}
		// Measure content size from ToolResultPart entries.
		contentSize := 0
		for _, part := range msgs[i].Content {
			if tr, ok := part.(sdk.ToolResultPart); ok {
				contentSize += len(fmt.Sprintf("%v", tr.Result))
			}
		}
		if contentSize > 512 { // only prune if content is large enough
			// Build replacement parts preserving ToolResultPart type so that
			// provider serializers that validate part types per role stay happy.
			replacementParts := make([]sdk.MessagePart, 0, len(msgs[i].Content))
			for _, part := range msgs[i].Content {
				if tr, ok := part.(sdk.ToolResultPart); ok {
					replacementParts = append(replacementParts, sdk.ToolResultPart{
						ToolCallID: tr.ToolCallID,
						ToolName:   tr.ToolName,
						Result:     fmt.Sprintf("[tool result pruned: %d bytes]", contentSize),
					})
				} else {
					replacementParts = append(replacementParts, part)
				}
			}
			pruned = append(pruned, sdk.Message{
				Role:    msgs[i].Role,
				Content: replacementParts,
			})
		} else {
			pruned = append(pruned, msgs[i])
		}
	}

	p.Messages = pruned
	return p
}

// runMidStreamRetry attempts to continue the agent stream after a retryable
// mid-stream error. It re-invokes StreamText with the accumulated messages
// and drains the new stream into the same output channel.
//
// sendCtx is used for sendEvent so consumer disconnect (parent ctx) still
// controls channel back-pressure; streamCtx is passed to the SDK for the same
// cancellation semantics as the main stream (including loop-detect cancel).
func (a *Agent) runMidStreamRetry(
	sendCtx context.Context,
	streamCtx context.Context,
	cancel context.CancelCauseFunc,
	toolLoopAbortCallIDs *toolAbortRegistry,
	ch chan<- StreamEvent,
	cfg RunConfig,
	sdkTools []sdk.Tool,
	prepareStep func(*sdk.GenerateParams) *sdk.GenerateParams,
	prevResult *sdk.StreamResult,
	stepNumber int,
	errMsg string,
	allText *strings.Builder,
	textLoopProbeBuffer *TextLoopProbeBuffer,
) (*sdk.StreamResult, bool) {
	// Drain the previous stream before reading prevResult.Messages.
	// This avoids racing with the SDK's final StreamResult write.
	if prevResult.Stream != nil {
		for range prevResult.Stream {
		}
	}

	retryCfg := DefaultRetryConfig()
	for attempt := 0; attempt < retryCfg.MaxAttempts; attempt++ {
		a.logger.Warn("mid-stream error, retrying",
			slog.Int("step", stepNumber),
			slog.Int("attempt", attempt+1),
			slog.Int("max_attempts", retryCfg.MaxAttempts),
			slog.String("error", errMsg),
		)
		if !sendEvent(sendCtx, ch, StreamEvent{
			Type:       EventRetry,
			Attempt:    attempt + 1,
			MaxAttempt: retryCfg.MaxAttempts,
			RetryError: errMsg,
		}) {
			return prevResult, true
		}

		delay := retryDelay(attempt, retryCfg)
		if delay > 0 {
			if err := sleepWithContext(streamCtx, delay); err != nil {
				return prevResult, true // aborted
			}
		}

		// Re-invoke StreamText with accumulated messages.
		// Use buildGenerateOptions so retry benefits from mid-task pruning,
		// media resolution, and other prepare-step logic — same as initial stream.
		retryCfgCopy := cfg
		retryCfgCopy.Messages = prevResult.Messages
		retryOpts := a.buildGenerateOptions(retryCfgCopy, sdkTools, prepareStep)

		retryResult, retryErr := a.client.StreamText(streamCtx, retryOpts...)
		if retryErr != nil {
			a.logger.Warn("mid-stream retry failed to start",
				slog.Int("attempt", attempt+1),
				slog.String("error", retryErr.Error()),
			)
			// Update errMsg so the next retry event shows the latest error.
			errMsg = retryErr.Error()
			continue
		}

		// Drain the retry stream into the main event loop
		aborted := false
		for retryPart := range retryResult.Stream {
			if streamCtx.Err() != nil {
				aborted = true
				break
			}
			switch rp := retryPart.(type) {
			case *sdk.TextStartPart:
				if !sendEvent(sendCtx, ch, StreamEvent{Type: EventTextStart}) {
					aborted = true
				}
			case *sdk.TextDeltaPart:
				if rp.Text != "" {
					if textLoopProbeBuffer != nil {
						textLoopProbeBuffer.Push(rp.Text)
					}
					if !sendEvent(sendCtx, ch, StreamEvent{Type: EventTextDelta, Delta: rp.Text}) {
						aborted = true
					}
					allText.WriteString(rp.Text)
				}
			case *sdk.TextEndPart:
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Flush()
				}
				stepNumber++
				if !sendEvent(sendCtx, ch, StreamEvent{Type: EventTextEnd}) {
					aborted = true
				}
			case *sdk.ToolInputStartPart:
				// See ToolInputStartPart note above: suppress the early start
				// and rely on StreamToolCallPart (which carries the fully
				// assembled Input) as the single source of truth.
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Flush()
				}
			case *sdk.StreamToolCallPart:
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Flush()
				}
				if !sendEvent(sendCtx, ch, StreamEvent{
					Type:       EventToolCallStart,
					ToolName:   rp.ToolName,
					ToolCallID: rp.ToolCallID,
					Input:      rp.Input,
				}) {
					aborted = true
				}
			case *sdk.StreamToolResultPart:
				shouldAbort := toolLoopAbortCallIDs.Take(rp.ToolCallID)
				stepNumber++
				if !sendEvent(sendCtx, ch, StreamEvent{
					Type:       EventToolCallEnd,
					ToolName:   rp.ToolName,
					ToolCallID: rp.ToolCallID,
					Input:      rp.Input,
					Result:     rp.Output,
				}) || !sendEvent(sendCtx, ch, StreamEvent{
					Type:           EventProgress,
					StepNumber:     stepNumber,
					ToolName:       rp.ToolName,
					ProgressStatus: "tool_result",
				}) {
					aborted = true
				}
				if shouldAbort {
					a.logger.Warn("tool loop abort triggered", slog.String("tool_call_id", rp.ToolCallID))
					cancel(ErrToolLoopDetected)
					aborted = true
				}
			case *sdk.StreamToolErrorPart:
				tookLoopAbort := toolLoopAbortCallIDs.Take(rp.ToolCallID)
				shouldAbort := errors.Is(rp.Error, ErrToolLoopDetected) || tookLoopAbort
				if !sendEvent(sendCtx, ch, StreamEvent{
					Type:       EventToolCallEnd,
					ToolName:   rp.ToolName,
					ToolCallID: rp.ToolCallID,
					Error:      rp.Error.Error(),
				}) {
					aborted = true
				}
				if shouldAbort {
					a.logger.Warn("tool loop abort triggered", slog.String("tool_call_id", rp.ToolCallID))
					cancel(ErrToolLoopDetected)
					aborted = true
				}
			case *sdk.ErrorPart:
				sendEvent(sendCtx, ch, StreamEvent{Type: EventError, Error: rp.Error.Error()})
				aborted = true
			case *sdk.AbortPart:
				aborted = true
			case *sdk.FinishPart:
				// handled after loop
			}
			if aborted {
				break
			}
		}
		if aborted {
			for range retryResult.Stream {
			}
		}
		return retryResult, aborted || detectGenerateLoopAbort(streamCtx, streamCtx.Err()) != nil
	}
	// All retry attempts failed
	return prevResult, true
}

// sleepWithContext sleeps for the given duration or returns context error.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func detectGenerateLoopAbort(ctx context.Context, err error) error {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return nil
	}

	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, ErrToolLoopDetected):
		return ErrToolLoopDetected
	case errors.Is(cause, ErrTextLoopDetected):
		return ErrTextLoopDetected
	default:
		return nil
	}
}

type loopAbortState struct {
	mu  sync.Mutex
	err error
}

func newLoopAbortState() *loopAbortState {
	return &loopAbortState{}
}

func (s *loopAbortState) Set(err error) {
	if s == nil || err == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *loopAbortState) Err() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
