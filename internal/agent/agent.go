package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/tools"
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

func (a *Agent) runStream(ctx context.Context, cfg RunConfig, ch chan<- StreamEvent) {
	tools, err := a.assembleTools(ctx, cfg)
	if err != nil {
		ch <- StreamEvent{Type: EventError, Error: fmt.Sprintf("assemble tools: %v", err)}
		return
	}
	tools, readMediaState := decorateReadMediaTools(cfg.Model, tools)

	// Loop detection setup
	var textLoopGuard *TextLoopGuard
	var textLoopProbeBuffer *TextLoopProbeBuffer
	var toolLoopGuard *ToolLoopGuard
	toolLoopAbortCallIDs := make(map[string]struct{})
	if cfg.LoopDetection.Enabled {
		textLoopGuard = NewTextLoopGuard(LoopDetectedStreakThreshold, LoopDetectedMinNewGramsPerChunk, SentialOptions{})
		textLoopProbeBuffer = NewTextLoopProbeBuffer(LoopDetectedProbeChars, func(text string) {
			result := textLoopGuard.Inspect(text)
			if result.Abort {
				a.logger.Warn("text loop detected, will abort")
			}
		})
		toolLoopGuard = NewToolLoopGuard(ToolLoopRepeatThreshold, ToolLoopWarningsBeforeAbort)
	}

	// Wrap tools with loop detection
	if toolLoopGuard != nil {
		tools = wrapToolsWithLoopGuard(tools, toolLoopGuard, toolLoopAbortCallIDs)
	}

	tagResolvers := DefaultTagResolvers()
	tagExtractor := NewStreamTagExtractor(tagResolvers)

	var prepareStep func(*sdk.GenerateParams) *sdk.GenerateParams
	if readMediaState != nil {
		prepareStep = readMediaState.prepareStep
	}
	opts := a.buildGenerateOptions(cfg, tools, prepareStep)

	streamResult, err := a.client.StreamText(ctx, opts...)
	if err != nil {
		ch <- StreamEvent{Type: EventError, Error: fmt.Sprintf("stream start: %v", err)}
		return
	}

	ch <- StreamEvent{Type: EventAgentStart}

	var allText strings.Builder
	aborted := false

	for part := range streamResult.Stream {
		if ctx.Err() != nil {
			aborted = true
			break
		}

		switch p := part.(type) {
		case *sdk.StartPart:
			_ = p // stream start already emitted

		case *sdk.TextStartPart:
			ch <- StreamEvent{Type: EventTextStart}

		case *sdk.TextDeltaPart:
			result := tagExtractor.Push(p.Text)
			if result.VisibleText != "" {
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Push(result.VisibleText)
				}
				ch <- StreamEvent{Type: EventTextDelta, Delta: result.VisibleText}
				allText.WriteString(result.VisibleText)
			}
			emitTagEvents(ch, result.Events)

		case *sdk.TextEndPart:
			remainder := tagExtractor.FlushRemainder()
			if remainder.VisibleText != "" {
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Push(remainder.VisibleText)
				}
				ch <- StreamEvent{Type: EventTextDelta, Delta: remainder.VisibleText}
				allText.WriteString(remainder.VisibleText)
			}
			if textLoopProbeBuffer != nil {
				textLoopProbeBuffer.Flush()
			}
			emitTagEvents(ch, remainder.Events)
			ch <- StreamEvent{Type: EventTextEnd}

		case *sdk.ReasoningStartPart:
			ch <- StreamEvent{Type: EventReasoningStart}

		case *sdk.ReasoningDeltaPart:
			ch <- StreamEvent{Type: EventReasoningDelta, Delta: p.Text}

		case *sdk.ReasoningEndPart:
			ch <- StreamEvent{Type: EventReasoningEnd}

		case *sdk.StreamToolCallPart:
			remainder := tagExtractor.FlushRemainder()
			if remainder.VisibleText != "" {
				if textLoopProbeBuffer != nil {
					textLoopProbeBuffer.Push(remainder.VisibleText)
				}
				ch <- StreamEvent{Type: EventTextDelta, Delta: remainder.VisibleText}
				allText.WriteString(remainder.VisibleText)
			}
			if textLoopProbeBuffer != nil {
				textLoopProbeBuffer.Flush()
			}
			emitTagEvents(ch, remainder.Events)
			ch <- StreamEvent{
				Type:       EventToolCallStart,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Input:      p.Input,
			}

		case *sdk.StreamToolResultPart:
			shouldAbort := false
			if _, ok := toolLoopAbortCallIDs[p.ToolCallID]; ok {
				delete(toolLoopAbortCallIDs, p.ToolCallID)
				shouldAbort = true
			}
			ch <- StreamEvent{
				Type:       EventToolCallEnd,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Input:      p.Input,
				Result:     p.Output,
			}
			if shouldAbort {
				a.logger.Warn("tool loop abort triggered", slog.String("tool_call_id", p.ToolCallID))
				aborted = true
			}

		case *sdk.StreamToolErrorPart:
			ch <- StreamEvent{
				Type:       EventToolCallEnd,
				ToolName:   p.ToolName,
				ToolCallID: p.ToolCallID,
				Error:      p.Error.Error(),
			}

		case *sdk.StreamFilePart:
			mediaType := p.File.MediaType
			if mediaType == "" {
				mediaType = "image/png"
			}
			ch <- StreamEvent{
				Type: EventAttachment,
				Attachments: []FileAttachment{{
					Type: "image",
					URL:  fmt.Sprintf("data:%s;base64,%s", mediaType, p.File.Data),
					Mime: mediaType,
				}},
			}

		case *sdk.ErrorPart:
			ch <- StreamEvent{Type: EventError, Error: p.Error.Error()}
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

	if textLoopProbeBuffer != nil {
		textLoopProbeBuffer.Flush()
	}

	finalMessages := streamResult.Messages
	if readMediaState != nil {
		finalMessages = readMediaState.mergeMessages(streamResult.Steps, finalMessages)
	}
	finalMessages = StripTagsFromMessages(finalMessages)

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
	if aborted {
		termEvent.Type = EventAgentAbort
	} else {
		termEvent.Type = EventAgentEnd
	}
	ch <- termEvent
}

func (a *Agent) runGenerate(ctx context.Context, cfg RunConfig) (*GenerateResult, error) {
	tools, err := a.assembleTools(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("assemble tools: %w", err)
	}
	tools, readMediaState := decorateReadMediaTools(cfg.Model, tools)

	var toolLoopGuard *ToolLoopGuard
	var textLoopGuard *TextLoopGuard
	toolLoopAbortCallIDs := make(map[string]struct{})
	if cfg.LoopDetection.Enabled {
		toolLoopGuard = NewToolLoopGuard(ToolLoopRepeatThreshold, ToolLoopWarningsBeforeAbort)
		textLoopGuard = NewTextLoopGuard(LoopDetectedStreakThreshold, LoopDetectedMinNewGramsPerChunk, SentialOptions{})
	}

	if toolLoopGuard != nil {
		tools = wrapToolsWithLoopGuard(tools, toolLoopGuard, toolLoopAbortCallIDs)
	}

	var prepareStep func(*sdk.GenerateParams) *sdk.GenerateParams
	if readMediaState != nil {
		prepareStep = readMediaState.prepareStep
	}
	opts := a.buildGenerateOptions(cfg, tools, prepareStep)
	opts = append(opts,
		sdk.WithOnStep(func(step *sdk.StepResult) *sdk.GenerateParams {
			if cfg.LoopDetection.Enabled {
				if len(toolLoopAbortCallIDs) > 0 {
					return nil // stop
				}
				if textLoopGuard != nil && isNonEmptyString(step.Text) {
					result := textLoopGuard.Inspect(step.Text)
					if result.Abort {
						return nil // stop
					}
				}
			}
			return nil
		}),
	)

	genResult, err := a.client.GenerateTextResult(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	resolvers := DefaultTagResolvers()
	cleanedText, events := ExtractTagsFromText(genResult.Text, resolvers)

	var attachments []FileAttachment
	var reactions []ReactionItem
	var speeches []SpeechItem
	for _, ev := range events {
		switch ev.Tag {
		case "attachments":
			for _, d := range ev.Data {
				if att, ok := d.(FileAttachment); ok {
					attachments = append(attachments, att)
				}
			}
		case "reactions":
			for _, d := range ev.Data {
				if r, ok := d.(ReactionItem); ok {
					reactions = append(reactions, r)
				}
			}
		case "speech":
			for _, d := range ev.Data {
				if s, ok := d.(SpeechItem); ok {
					speeches = append(speeches, s)
				}
			}
		}
	}

	finalMessages := genResult.Messages
	if readMediaState != nil {
		finalMessages = readMediaState.mergeMessages(genResult.Steps, finalMessages)
	}
	finalMessages = StripTagsFromMessages(finalMessages)

	return &GenerateResult{
		Messages:    finalMessages,
		Text:        cleanedText,
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
	if len(tools) > 0 {
		opts = append(opts, sdk.WithTools(tools))
	}
	if prepareStep != nil {
		opts = append(opts, sdk.WithPrepareStep(prepareStep))
	}
	opts = append(opts, BuildReasoningOptions(ModelConfig{
		ClientType: resolveClientType(cfg.Model),
		ReasoningConfig: &ReasoningConfig{
			Enabled: cfg.ReasoningEffort != "",
			Effort:  cfg.ReasoningEffort,
		},
	})...)
	return opts
}

func resolveClientType(model *sdk.Model) string {
	if model == nil || model.Provider == nil {
		return ClientTypeOpenAICompletions
	}
	name := model.Provider.Name()
	switch {
	case strings.Contains(name, "anthropic"):
		return ClientTypeAnthropicMessages
	case strings.Contains(name, "google"):
		return ClientTypeGoogleGenerativeAI
	case strings.Contains(name, "responses"):
		return ClientTypeOpenAIResponses
	default:
		return ClientTypeOpenAICompletions
	}
}

// assembleTools collects tools from all registered ToolProviders.
func (a *Agent) assembleTools(ctx context.Context, cfg RunConfig) ([]sdk.Tool, error) {
	if len(a.toolProviders) == 0 {
		return nil, nil
	}
	skillsMap := make(map[string]tools.SkillDetail, len(cfg.Skills))
	for _, s := range cfg.Skills {
		skillsMap[s.Name] = tools.SkillDetail{
			Description: s.Description,
			Content:     s.Content,
		}
	}
	session := tools.SessionContext{
		BotID:              cfg.Identity.BotID,
		ChatID:             cfg.Identity.ChatID,
		SessionID:          cfg.Identity.SessionID,
		ChannelIdentityID:  cfg.Identity.ChannelIdentityID,
		SessionToken:       cfg.Identity.SessionToken,
		CurrentPlatform:    cfg.Identity.CurrentPlatform,
		ReplyTarget:        cfg.Identity.ReplyTarget,
		SupportsImageInput: cfg.SupportsImageInput,
		IsSubagent:         cfg.Identity.IsSubagent,
		Skills:             skillsMap,
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
	return allTools, nil
}

func emitTagEvents(ch chan<- StreamEvent, events []TagEvent) {
	for _, ev := range events {
		switch ev.Tag {
		case "attachments":
			var atts []FileAttachment
			for _, d := range ev.Data {
				if att, ok := d.(FileAttachment); ok {
					atts = append(atts, att)
				}
			}
			if len(atts) > 0 {
				ch <- StreamEvent{Type: EventAttachment, Attachments: atts}
			}
		case "reactions":
			var reactions []ReactionItem
			for _, d := range ev.Data {
				if r, ok := d.(ReactionItem); ok {
					reactions = append(reactions, r)
				}
			}
			if len(reactions) > 0 {
				ch <- StreamEvent{Type: EventReaction, Reactions: reactions}
			}
		case "speech":
			var speeches []SpeechItem
			for _, d := range ev.Data {
				if s, ok := d.(SpeechItem); ok {
					speeches = append(speeches, s)
				}
			}
			if len(speeches) > 0 {
				ch <- StreamEvent{Type: EventSpeech, Speeches: speeches}
			}
		}
	}
}

func wrapToolsWithLoopGuard(tools []sdk.Tool, guard *ToolLoopGuard, abortCallIDs map[string]struct{}) []sdk.Tool {
	wrapped := make([]sdk.Tool, len(tools))
	for i, tool := range tools {
		originalExecute := tool.Execute
		toolName := tool.Name
		wrapped[i] = tool
		wrapped[i].Execute = func(ctx *sdk.ToolExecContext, input any) (any, error) {
			warn, abort := guard.Guard(toolName, input)
			if abort {
				abortCallIDs[ctx.ToolCallID] = struct{}{}
				return map[string]any{
					"isError": true,
					"content": []map[string]any{{
						"type": "text",
						"text": ToolLoopDetectedAbortMessage,
					}},
				}, errors.New(ToolLoopDetectedAbortMessage)
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
