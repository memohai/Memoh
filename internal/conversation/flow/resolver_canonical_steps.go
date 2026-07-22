package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type canonicalStepPersistenceState struct {
	mu sync.Mutex

	resolver  *Resolver
	persister messagepkg.CanonicalTurnPersister
	turn      messagepkg.CanonicalTurn
	req       conversation.ChatRequest
	modelID   string

	pendingInjected   []conversation.ModelMessage
	persisted         []messagepkg.Message
	persistedSDKCount int
	assistantStored   bool
	committed         bool
}

func (r *Resolver) beginCanonicalStepPersistence(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
) (*canonicalStepPersistenceState, error) {
	if r == nil || r.messageService == nil || strings.TrimSpace(req.BotID) == "" || strings.TrimSpace(req.SessionID) == "" {
		return nil, nil
	}
	persister, ok := r.messageService.(messagepkg.CanonicalTurnPersister)
	if !ok {
		return nil, nil
	}

	replacement := replacementPersistenceFromContext(ctx)
	var turn messagepkg.CanonicalTurn
	var initial []messagepkg.Message
	if replacement == nil && (req.UserMessagePersisted || req.ReusePersistedUserMessage) {
		var historyTurn messagepkg.HistoryTurn
		var err error
		if requestID := strings.TrimSpace(req.PersistedUserMessageID); requestID != "" {
			historyTurn, err = r.messageService.GetVisibleTurnByMessage(ctx, req.SessionID, requestID)
		} else {
			historyTurn, err = r.messageService.GetLatestVisibleTurnBySession(ctx, req.SessionID)
		}
		if err != nil {
			return nil, fmt.Errorf("load canonical turn for continuation: %w", err)
		}
		if strings.TrimSpace(historyTurn.RequestMessageID) == "" {
			return nil, errors.New("canonical continuation turn has no request message")
		}
		turn = messagepkg.CanonicalTurn{
			ID:               historyTurn.ID,
			BotID:            historyTurn.BotID,
			SessionID:        historyTurn.SessionID,
			RequestMessageID: historyTurn.RequestMessageID,
		}
	} else {
		startReq := req
		startReq.UserMessagePersisted = false
		startReq.ReusePersistedUserMessage = false
		startReq.PersistedUserMessageID = ""
		startReq.SkipHistoryTurn = false
		startMessages := prependTurnUserMessage(startReq, nil)
		startMessages = r.prepareRoundMessagesForPersistence(startReq, startMessages, storeRoundOptions{SkipMemory: true})
		inputs, _, err := r.buildPersistInputs(ctx, startReq, startMessages, rc.model.ID, storeRoundOptions{SkipMemory: true})
		if err != nil {
			return nil, err
		}
		if len(inputs) != 1 || !strings.EqualFold(strings.TrimSpace(inputs[0].Role), "user") {
			return nil, fmt.Errorf("canonical turn start produced %d messages, want one user message", len(inputs))
		}
		options, err := r.buildRoundPersistenceOptions(ctx, req, replacement)
		if err != nil {
			return nil, err
		}
		var requestMessage messagepkg.Message
		turn, requestMessage, err = persister.StartCanonicalTurn(ctx, messagepkg.CanonicalTurnStart{
			Request:     inputs[0],
			Replacement: options.Replacement,
		})
		if err != nil {
			return nil, fmt.Errorf("start canonical turn: %w", err)
		}
		initial = append(initial, requestMessage)
		if replacement != nil {
			replacement.atomicCommitted = true
		}
	}

	appendReq := req
	appendReq.UserMessagePersisted = true
	appendReq.ReusePersistedUserMessage = false
	appendReq.PersistedUserMessageID = turn.RequestMessageID
	appendReq.SkipHistoryTurn = false
	appendReq.OutboundAssetCollector = nil
	return &canonicalStepPersistenceState{
		resolver:  r,
		persister: persister,
		turn:      turn,
		req:       appendReq,
		modelID:   rc.model.ID,
		persisted: initial,
		committed: true,
	}, nil
}

func (s *canonicalStepPersistenceState) attachToRunConfig(cfg agentpkg.RunConfig) agentpkg.RunConfig {
	if s == nil {
		return cfg
	}
	nextInjected := cfg.InjectedRecorder
	cfg.InjectedRecorder = func(headerifiedText string, imageParts []sdk.ImagePart, insertAfter int) {
		if nextInjected != nil {
			nextInjected(headerifiedText, imageParts, insertAfter)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		parts := make([]sdk.MessagePart, len(imageParts))
		for index := range imageParts {
			parts[index] = imageParts[index]
		}
		s.pendingInjected = append(s.pendingInjected, sdkMessagesToModelMessages([]sdk.Message{sdk.UserMessage(headerifiedText, parts...)})...)
	}
	nextStep := cfg.OnStepCompleted
	cfg.OnStepCompleted = func(ctx context.Context, step *sdk.StepResult) error {
		if nextStep != nil {
			if err := nextStep(ctx, step); err != nil {
				return err
			}
		}
		return s.appendCompletedStep(ctx, step)
	}
	return cfg
}

func (s *canonicalStepPersistenceState) appendCompletedStep(ctx context.Context, step *sdk.StepResult) error {
	if s == nil || step == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := append([]conversation.ModelMessage(nil), s.pendingInjected...)
	messages = append(messages, sdkMessagesToModelMessages(step.Messages)...)
	if err := s.appendMessagesLocked(context.WithoutCancel(ctx), messages, step.DeferredToolApproval != nil); err != nil {
		return err
	}
	s.pendingInjected = s.pendingInjected[:0]
	s.persistedSDKCount += len(step.Messages)
	return nil
}

func (s *canonicalStepPersistenceState) appendTerminalSnapshot(ctx context.Context, snap terminalSnapshot) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.persistedSDKCount > len(snap.sdkMessages) {
		return errors.New("canonical step cursor is ahead of terminal messages")
	}
	messages := append([]conversation.ModelMessage(nil), s.pendingInjected...)
	messages = append(messages, sdkMessagesToModelMessages(snap.sdkMessages[s.persistedSDKCount:])...)
	if err := s.appendMessagesLocked(context.WithoutCancel(ctx), messages, snap.deferredToolID != ""); err != nil {
		return err
	}
	s.pendingInjected = s.pendingInjected[:0]
	s.persistedSDKCount = len(snap.sdkMessages)
	return nil
}

func (s *canonicalStepPersistenceState) appendMessagesLocked(
	ctx context.Context,
	messages []conversation.ModelMessage,
	allowPendingToolCalls bool,
) error {
	if len(messages) == 0 {
		return nil
	}
	if err := runPersistenceGuard(ctx); err != nil {
		return fmt.Errorf("runtime ownership check before canonical append: %w", err)
	}
	filterReq := s.req
	filterReq.UserMessagePersisted = false
	filterReq.ReusePersistedUserMessage = false
	filtered := s.resolver.prepareRoundMessagesForPersistence(filterReq, messages, storeRoundOptions{
		AllowPendingToolCalls: allowPendingToolCalls,
		SkipMemory:            true,
	})
	if len(filtered) == 0 {
		return nil
	}
	inputs, _, err := s.resolver.buildPersistInputs(ctx, s.req, filtered, s.modelID, storeRoundOptions{
		AllowPendingToolCalls: allowPendingToolCalls,
		SkipMemory:            true,
	})
	if err != nil {
		return err
	}
	persisted, err := s.persister.AppendCanonicalTurn(ctx, s.turn, inputs)
	if err != nil {
		return fmt.Errorf("append canonical turn: %w", err)
	}
	for _, message := range persisted {
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			s.assistantStored = true
		}
	}
	s.persisted = append(s.persisted, persisted...)
	return nil
}

func (s *canonicalStepPersistenceState) result() ([]messagepkg.Message, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]messagepkg.Message(nil), s.persisted...), s.committed
}

func (s *canonicalStepPersistenceState) hasAssistant() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.assistantStored
}
