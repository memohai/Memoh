package sessionruntime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DispatchActiveCommand routes a response for a UI request embedded in the
// current run to that run's owner. A false handled result means the target is
// not part of the active run and the caller may use its deferred flow.
func (m *Manager) DispatchActiveCommand(ctx context.Context, botID, sessionID, commandType, targetID string, payload []byte) (bool, error) {
	if m == nil || m.backend == nil {
		return false, nil
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	targetID = strings.TrimSpace(targetID)
	if botID == "" || sessionID == "" || targetID == "" {
		return false, nil
	}
	if commandType != CommandToolApprovalResponse && commandType != CommandUserInputResponse {
		return false, fmt.Errorf("unsupported active runtime command %q", commandType)
	}
	snapshot, err := m.Snapshot(ctx, botID, sessionID)
	if err != nil {
		return false, err
	}
	run := snapshot.CurrentRunView
	if run == nil {
		return false, nil
	}
	canonicalTargetID, targetPresent := runtimeCommandTargetID(run, commandType, targetID)
	if !targetPresent {
		return false, nil
	}
	cmd := Command{
		Type: commandType, ID: activeCommandID(botID, sessionID, run, commandType, canonicalTargetID),
		BotID: botID, SessionID: sessionID, StreamID: strings.TrimSpace(run.StreamID),
		Generation: strings.TrimSpace(run.Generation), TargetID: canonicalTargetID,
		Payload: append([]byte(nil), payload...), PayloadHash: activeCommandPayloadHash(commandType, payload),
	}
	timeout := m.commandTimeout()
	if m.distributed != nil {
		loadCtx, cancel := context.WithTimeout(ctx, min(timeout, 100*time.Millisecond))
		result, ok, loadErr := m.loadCommandResult(loadCtx, cmd.ID)
		cancel()
		if loadErr != nil {
			return true, loadErr
		} else if ok {
			return true, commandResultErrorFor(cmd, result)
		}
	}
	if !isActiveRunStatus(run.Status) {
		if reconciled, reconcileErr := m.reconcileRoutedCommand(ctx, cmd); reconciled {
			if reconcileErr != nil {
				return true, reconcileErr
			}
			result := m.persistCommandResult(ctx, cmd, reconcileErr)
			return true, commandResultErrorFor(cmd, result)
		}
		return false, nil
	}
	createdAt, err := m.backend.Now(ctx)
	if err != nil {
		return true, fmt.Errorf("load runtime command time: %w", err)
	}
	cmd.CreatedAt = createdAt
	cmd.ExpiresAt = createdAt.Add(timeout)
	if m.distributed == nil {
		commandCtx, cancel, commandErr := m.activeCommandContext(ctx, cmd)
		defer cancel()
		if commandErr != nil {
			return true, commandErr
		}
		return true, m.applyRoutedCommand(commandCtx, cmd)
	}
	ownerID := strings.TrimSpace(run.OwnerID)
	if ownerID == "" {
		return true, errors.New("target runtime owner is unknown")
	}
	if ownerID == m.ownerID {
		result := m.executeRoutedCommand(ctx, cmd)
		return true, commandResultErrorFor(cmd, result)
	}
	dispatchErr := m.dispatchRemoteCommand(ctx, ownerID, cmd)
	if dispatchErr != nil {
		if reconciled, reconcileErr := m.reconcileRoutedCommand(ctx, cmd); reconciled {
			if reconcileErr != nil {
				return true, reconcileErr
			}
			result := m.persistCommandResult(ctx, cmd, reconcileErr)
			return true, commandResultErrorFor(cmd, result)
		}
	}
	return true, dispatchErr
}

func (m *Manager) dispatchRemoteCommand(ctx context.Context, ownerID string, cmd Command) error {
	ownerID = strings.TrimSpace(ownerID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if ownerID == "" {
		return errors.New("target runtime owner is unknown")
	}
	if cmd.ID == "" {
		return errors.New("runtime command id is required")
	}
	cmd.ReplyOwnerID = m.ownerID
	waiter := &commandWaiter{result: make(chan error, 1), payloadHash: cmd.PayloadHash}
	m.mu.Lock()
	if m.isClosed() {
		m.mu.Unlock()
		return ErrManagerClosed
	}
	waiters := m.pendingCommands[cmd.ID]
	if waiters == nil {
		waiters = make(map[*commandWaiter]struct{})
		m.pendingCommands[cmd.ID] = waiters
	}
	waiters[waiter] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if waiters := m.pendingCommands[cmd.ID]; waiters != nil {
			delete(waiters, waiter)
			if len(waiters) == 0 {
				delete(m.pendingCommands, cmd.ID)
			}
		}
		m.mu.Unlock()
	}()
	if err := m.distributed.PublishCommand(ctx, ownerID, cmd); err != nil {
		return err
	}
	return m.waitCommandResult(ctx, cmd, waiter.result, m.commandTimeout(), ownerID)
}

func activeCommandID(botID, sessionID string, run *CurrentRunView, commandType, targetID string) string {
	parts := []string{
		strings.TrimSpace(botID), strings.TrimSpace(sessionID), strings.TrimSpace(run.StreamID),
		strings.TrimSpace(run.Generation), strings.TrimSpace(commandType), strings.TrimSpace(targetID),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("active-response-%x", sum[:])
}

func commandPayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum[:])
}

func activeCommandPayloadHash(commandType string, payload []byte) string {
	var raw map[string]any
	if err := unmarshalRuntimeJSON(payload, &raw); err != nil {
		return commandPayloadHash(payload)
	}
	canonical := make(map[string]any)
	switch commandType {
	case CommandToolApprovalResponse:
		decision := strings.ToLower(runtimeCommandString(runtimeCommandMapValue(raw, "decision")))
		switch decision {
		case "approve", "approved":
			decision = "approved"
		case "reject", "rejected":
			decision = "rejected"
		}
		canonical["decision"] = decision
		canonical["reason"] = runtimeCommandString(runtimeCommandMapValue(raw, "reason"))
	case CommandUserInputResponse:
		canceled, _ := runtimeCommandMapValue(raw, "canceled").(bool)
		canonical["canceled"] = canceled
		if canceled {
			canonical["reason"] = runtimeCommandString(runtimeCommandMapValue(raw, "reason"))
			break
		}
		answers := canonicalRuntimeAnswers(runtimeCommandMapValue(raw, "answers"))
		if len(answers) > 0 {
			canonical["answers"] = answers
		} else {
			canonical["text_answer"] = runtimeCommandString(runtimeCommandMapValue(raw, "text_answer"))
		}
	default:
		return commandPayloadHash(payload)
	}
	encoded, err := marshalRuntimeJSON(canonical)
	if err != nil {
		return commandPayloadHash(payload)
	}
	return commandPayloadHash(encoded)
}

func runtimeCommandMapValue(values map[string]any, name string) any {
	want := strings.ReplaceAll(strings.ToLower(name), "_", "")
	for key, value := range values {
		if strings.ReplaceAll(strings.ToLower(key), "_", "") == want {
			return value
		}
	}
	return nil
}

func runtimeCommandString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func canonicalRuntimeAnswers(value any) []map[string]any {
	items, _ := value.([]any)
	answers := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		optionValues, _ := runtimeCommandMapValue(raw, "option_ids").([]any)
		optionIDs := make([]string, 0, len(optionValues))
		for _, optionID := range optionValues {
			optionIDs = append(optionIDs, runtimeCommandString(optionID))
		}
		answers = append(answers, map[string]any{
			"question_id": runtimeCommandString(runtimeCommandMapValue(raw, "question_id")),
			"option_ids":  optionIDs,
			"custom_text": runtimeCommandString(runtimeCommandMapValue(raw, "custom_text")),
			"text":        runtimeCommandString(runtimeCommandMapValue(raw, "text")),
		})
	}
	sort.SliceStable(answers, func(i, j int) bool {
		return answers[i]["question_id"].(string) < answers[j]["question_id"].(string)
	})
	return answers
}

func (m *Manager) applyCommand(ctx context.Context, cmd Command) {
	switch strings.TrimSpace(cmd.Type) {
	case CommandAbort, CommandToolApprovalResponse, CommandUserInputResponse:
		m.publishStoredCommandResult(ctx, cmd, m.executeRoutedCommand(ctx, cmd))
	case CommandSteer:
		commandCtx, cancel, err := m.activeCommandContext(ctx, cmd)
		if err != nil {
			_ = m.updateSteerStatus(context.WithoutCancel(ctx), runHandleForCommand(cmd), cmd.SteerID, SteerStatusRejected, steerNotAcknowledgedError)
			return
		}
		m.applySteerCommand(commandCtx, cmd)
		cancel()
	case CommandResult:
		m.completePendingCommand(cmd)
	}
}

func (m *Manager) activeCommandContext(ctx context.Context, cmd Command) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, lookupCancel := context.WithTimeout(ctx, m.commandTimeout())
	lookupStarted := time.Now()
	now, err := m.backend.Now(lookupCtx)
	lookupElapsed := time.Since(lookupStarted)
	if err != nil {
		lookupCancel()
		return nil, func() {}, fmt.Errorf("load runtime command time: %w", err)
	}
	expiresAt := cmd.ExpiresAt
	if expiresAt.IsZero() || (!cmd.CreatedAt.IsZero() && !expiresAt.After(cmd.CreatedAt)) {
		lookupCancel()
		return nil, func() {}, ErrCommandExpired
	}
	remaining := expiresAt.Sub(now) - lookupElapsed
	if remaining <= 0 {
		lookupCancel()
		return nil, func() {}, ErrCommandExpired
	}
	lookupCancel()
	commandCtx, commandCancel := context.WithTimeout(ctx, remaining)
	return commandCtx, commandCancel, nil
}

func (m *Manager) applyRoutedCommand(ctx context.Context, cmd Command) error {
	ctrl := m.localControlForScope(cmd.BotID, cmd.SessionID, cmd.StreamID)
	if ctrl == nil || ctrl.botID != strings.TrimSpace(cmd.BotID) || ctrl.sessionID != strings.TrimSpace(cmd.SessionID) || ctrl.generation != strings.TrimSpace(cmd.Generation) {
		return ErrCommandTargetNotActive
	}
	commandCtx, cancel := ctrl.commandContext(ctx)
	defer cancel()
	if !ctrl.commandsActive() || commandCtx.Err() != nil {
		return ErrCommandTargetNotActive
	}
	handle := runHandleForCommand(cmd)
	if err := m.ValidateRunOwnership(commandCtx, handle); err != nil {
		if errors.Is(err, ErrRunOwnershipLost) {
			return ErrCommandTargetNotActive
		}
		return err
	}
	snapshot, ok, err := m.backend.Load(commandCtx, Key{BotID: cmd.BotID, SessionID: cmd.SessionID})
	if err != nil {
		return err
	}
	if !ok || snapshot.CurrentRunView == nil {
		return ErrCommandTargetNotActive
	}
	run := snapshot.CurrentRunView
	if run.StreamID != ctrl.streamID || run.Generation != ctrl.generation || !m.runOwnerMatches(run) || !isActiveRunStatus(run.Status) {
		return ErrCommandTargetNotActive
	}
	if strings.TrimSpace(cmd.Type) == CommandAbort {
		_, err := m.abortLocal(commandCtx, ctrl)
		return err
	}
	if !runtimeCommandTargetPresent(run, cmd.Type, cmd.TargetID) {
		return ErrCommandTargetNotActive
	}
	m.mu.Lock()
	handler := m.commandHandler
	m.mu.Unlock()
	if handler == nil {
		return errors.New("runtime command handler is not configured")
	}
	if !m.beginCommandTarget(cmd) {
		return ErrCommandBusy
	}
	defer m.endCommandTarget(cmd)
	err = handler(commandCtx, cmd)
	return err
}

func (m *Manager) executeRoutedCommand(ctx context.Context, cmd Command) Command {
	leader, executionDone := m.beginCommandExecution(cmd.ID)
	if !leader {
		select {
		case <-executionDone:
		case <-ctx.Done():
			return newCommandResult(cmd, ctx.Err())
		}
		if result, ok, err := m.loadCommandResultForExecution(ctx, cmd.ID); err != nil {
			return newCommandResult(cmd, err)
		} else if ok {
			if errors.Is(commandResultErrorFor(cmd, result), ErrCommandPayloadConflict) {
				return newCommandResult(cmd, ErrCommandPayloadConflict)
			}
			return result
		}
		return newCommandResult(cmd, errors.New("runtime command result is unavailable"))
	}
	defer m.finishCommandExecution(cmd.ID, executionDone)

	if result, ok, err := m.loadCommandResultForExecution(ctx, cmd.ID); err != nil {
		return newCommandResult(cmd, err)
	} else if ok {
		if errors.Is(commandResultErrorFor(cmd, result), ErrCommandPayloadConflict) {
			return newCommandResult(cmd, ErrCommandPayloadConflict)
		}
		return result
	}
	commandCtx, cancel, err := m.activeCommandContext(ctx, cmd)
	reconciled := false
	if err == nil {
		err = m.applyRoutedCommand(commandCtx, cmd)
		if errors.Is(err, ErrCommandTargetNotActive) {
			if handled, reconcileErr := m.reconcileRoutedCommand(commandCtx, cmd); handled {
				reconciled = true
				err = reconcileErr
			}
		}
	}
	cancel()
	if reconciled && err != nil {
		return newCommandResult(cmd, err)
	}
	return m.persistCommandResult(ctx, cmd, err)
}

func (m *Manager) reconcileRoutedCommand(ctx context.Context, cmd Command) (bool, error) {
	if m == nil {
		return false, nil
	}
	m.mu.Lock()
	reconciler := m.commandReconciler
	m.mu.Unlock()
	if reconciler == nil {
		return false, nil
	}
	return reconciler(ctx, cmd)
}

func newCommandResult(request Command, err error) Command {
	result := Command{
		Type: CommandResult, ID: request.ID, BotID: request.BotID, SessionID: request.SessionID,
		StreamID: request.StreamID, Generation: request.Generation, TargetID: request.TargetID,
		PayloadHash: request.PayloadHash, CreatedAt: time.Now().UTC(),
	}
	if err == nil {
		return result
	}
	result.Error = err.Error()
	switch {
	case errors.Is(err, ErrCommandTargetNotActive):
		result.ErrorCode = "target_not_active"
	case errors.Is(err, ErrCommandExpired):
		result.ErrorCode = "command_expired"
	case errors.Is(err, context.Canceled):
		result.ErrorCode = "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		result.ErrorCode = "deadline_exceeded"
	case errors.Is(err, ErrCommandBusy):
		result.ErrorCode = "command_busy"
	case errors.Is(err, ErrCommandPayloadConflict):
		result.ErrorCode = "payload_conflict"
	default:
		result.ErrorCode = "runtime_command_failed"
	}
	return result
}

func (m *Manager) persistCommandResult(ctx context.Context, request Command, err error) Command {
	result := newCommandResult(request, err)
	// A stable command ID must not pin transient execution failures. Only a
	// successful domain transition is safe to reuse without re-evaluation;
	// conflicting retries are still rejected by the stored success hash.
	if err != nil {
		return result
	}
	if m == nil || m.distributed == nil || strings.TrimSpace(request.ID) == "" {
		return result
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.commandTimeout())
	defer cancel()
	if storeErr := m.distributed.StoreCommandResult(persistCtx, result, m.commandResultTTL()); storeErr != nil {
		m.logger.Warn("store runtime command result failed", slog.Any("error", storeErr), slog.String("command_id", request.ID))
		return result
	}
	stored, ok, loadErr := m.distributed.LoadCommandResult(persistCtx, request.ID)
	if loadErr != nil {
		m.logger.Warn("reload runtime command result failed", slog.Any("error", loadErr), slog.String("command_id", request.ID))
		return result
	}
	if ok {
		return stored
	}
	return result
}

func (m *Manager) loadCommandResultForExecution(ctx context.Context, commandID string) (Command, bool, error) {
	loadCtx, cancel := context.WithTimeout(ctx, m.commandTimeout())
	defer cancel()
	return m.loadCommandResult(loadCtx, commandID)
}

func (m *Manager) publishCommandResult(ctx context.Context, request Command, err error) {
	replyOwnerID := strings.TrimSpace(request.ReplyOwnerID)
	if replyOwnerID == "" {
		return
	}
	if m.distributed == nil {
		return
	}
	result := m.persistCommandResult(ctx, request, err)
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.commandTimeout())
	defer cancel()
	if publishErr := m.distributed.PublishCommand(publishCtx, replyOwnerID, result); publishErr != nil {
		m.logger.Warn("publish runtime command result failed", slog.Any("error", publishErr), slog.String("command_id", request.ID))
	}
}

func (m *Manager) publishStoredCommandResult(ctx context.Context, request, result Command) {
	if m == nil || m.distributed == nil || strings.TrimSpace(request.ReplyOwnerID) == "" {
		return
	}
	if err := commandResultErrorFor(request, result); errors.Is(err, ErrCommandPayloadConflict) {
		result = Command{
			Type: CommandResult, ID: request.ID, BotID: request.BotID, SessionID: request.SessionID,
			StreamID: request.StreamID, Generation: request.Generation, TargetID: request.TargetID,
			PayloadHash: request.PayloadHash, ErrorCode: "payload_conflict", Error: ErrCommandPayloadConflict.Error(),
			CreatedAt: time.Now().UTC(),
		}
	}
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.commandTimeout())
	defer cancel()
	if err := m.distributed.PublishCommand(publishCtx, request.ReplyOwnerID, result); err != nil {
		m.logger.Warn("republish stored runtime command result failed", slog.Any("error", err), slog.String("command_id", request.ID))
	}
}

func (m *Manager) commandTimeout() time.Duration {
	if m != nil && m.commandAckTTL > 0 {
		return m.commandAckTTL
	}
	return defaultCommandAckTTL
}

func (m *Manager) commandResultTTL() time.Duration {
	ttl := 4 * m.commandTimeout()
	if ttl < 30*time.Second {
		return 30 * time.Second
	}
	return ttl
}

func (m *Manager) waitCommandResult(ctx context.Context, request Command, pending <-chan error, timeout time.Duration, retryOwnerIDs ...string) error {
	waitCtx, cancelWait := context.WithTimeout(ctx, timeout)
	defer cancelWait()
	retryOwnerID := ""
	if len(retryOwnerIDs) > 0 {
		retryOwnerID = strings.TrimSpace(retryOwnerIDs[0])
	}
	pollEvery := timeout / 4
	if pollEvery < time.Millisecond {
		pollEvery = time.Millisecond
	}
	if pollEvery > 50*time.Millisecond {
		pollEvery = 50 * time.Millisecond
	}
	poll := time.NewTicker(pollEvery)
	defer poll.Stop()
	retryEvery := timeout / 4
	if retryEvery < 10*time.Millisecond {
		retryEvery = 10 * time.Millisecond
	}
	if retryEvery > 250*time.Millisecond {
		retryEvery = 250 * time.Millisecond
	}
	var retry <-chan time.Time
	var retryTicker *time.Ticker
	if retryOwnerID != "" {
		retryTicker = time.NewTicker(retryEvery)
		retry = retryTicker.C
		defer retryTicker.Stop()
	}
	for {
		select {
		case err := <-pending:
			return err
		case <-waitCtx.Done():
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.New("runtime command was not acknowledged")
		case <-poll.C:
			result, ok, loadErr := m.loadCommandResult(waitCtx, request.ID)
			if loadErr == nil && ok {
				return commandResultErrorFor(request, result)
			}
		case <-retry:
			if err := m.distributed.PublishCommand(waitCtx, retryOwnerID, request); err != nil && waitCtx.Err() == nil {
				m.logger.Debug("retry runtime command publish failed", slog.Any("error", err), slog.String("command_id", request.ID))
			}
		}
	}
}

func (m *Manager) loadCommandResult(ctx context.Context, commandID string) (Command, bool, error) {
	if m == nil || m.distributed == nil || strings.TrimSpace(commandID) == "" {
		return Command{}, false, nil
	}
	return m.distributed.LoadCommandResult(ctx, commandID)
}

func (m *Manager) completePendingCommand(result Command) {
	m.mu.Lock()
	waiters := m.pendingCommands[strings.TrimSpace(result.ID)]
	if waiters != nil {
		delete(m.pendingCommands, strings.TrimSpace(result.ID))
	}
	m.mu.Unlock()
	if waiters == nil {
		return
	}
	for waiter := range waiters {
		waiter.result <- commandResultErrorFor(Command{PayloadHash: waiter.payloadHash}, result)
	}
}

func commandResultErrorFor(request, result Command) error {
	expected := strings.TrimSpace(request.PayloadHash)
	actual := strings.TrimSpace(result.PayloadHash)
	if expected != "" && actual != "" && expected != actual {
		return ErrCommandPayloadConflict
	}
	return commandResultError(result)
}

func commandResultError(result Command) error {
	if strings.TrimSpace(result.Error) == "" {
		return nil
	}
	switch result.ErrorCode {
	case "target_not_active":
		return fmt.Errorf("%w: %s", ErrCommandTargetNotActive, result.Error)
	case "command_expired":
		return fmt.Errorf("%w: %s", ErrCommandExpired, result.Error)
	case "context_canceled":
		return fmt.Errorf("%w: %s", context.Canceled, result.Error)
	case "deadline_exceeded":
		return fmt.Errorf("%w: %s", context.DeadlineExceeded, result.Error)
	case "command_busy":
		return fmt.Errorf("%w: %s", ErrCommandBusy, result.Error)
	case "payload_conflict":
		return fmt.Errorf("%w: %s", ErrCommandPayloadConflict, result.Error)
	default:
		return errors.New(result.Error)
	}
}

func commandTargetKey(cmd Command) string {
	return strings.Join([]string{
		strings.TrimSpace(cmd.BotID), strings.TrimSpace(cmd.SessionID), strings.TrimSpace(cmd.StreamID),
		strings.TrimSpace(cmd.Generation), strings.TrimSpace(cmd.Type), strings.TrimSpace(cmd.TargetID),
	}, "\x00")
}

func (m *Manager) beginCommandTarget(cmd Command) bool {
	key := commandTargetKey(cmd)
	if key == "\x00\x00\x00\x00\x00" {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.inflightCommandTargets[key]; exists {
		return false
	}
	m.inflightCommandTargets[key] = struct{}{}
	return true
}

func (m *Manager) endCommandTarget(cmd Command) {
	key := commandTargetKey(cmd)
	m.mu.Lock()
	delete(m.inflightCommandTargets, key)
	m.mu.Unlock()
}

func (m *Manager) beginCommandExecution(commandID string) (bool, chan struct{}) {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		done := make(chan struct{})
		return true, done
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if done := m.commandExecutions[commandID]; done != nil {
		return false, done
	}
	done := make(chan struct{})
	m.commandExecutions[commandID] = done
	return true, done
}

func (m *Manager) finishCommandExecution(commandID string, done chan struct{}) {
	commandID = strings.TrimSpace(commandID)
	m.mu.Lock()
	if commandID != "" && m.commandExecutions[commandID] == done {
		close(done)
		delete(m.commandExecutions, commandID)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	close(done)
}

func isDurableRoutedCommand(cmd Command) bool {
	switch strings.TrimSpace(cmd.Type) {
	case CommandAbort, CommandToolApprovalResponse, CommandUserInputResponse:
		return strings.TrimSpace(cmd.ID) != ""
	default:
		return false
	}
}

func (m *Manager) admitCommand(cmd Command) bool {
	if !isDurableRoutedCommand(cmd) {
		return true
	}
	commandID := strings.TrimSpace(cmd.ID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.admittedCommands[commandID]; exists {
		return false
	}
	m.admittedCommands[commandID] = struct{}{}
	return true
}

func (m *Manager) releaseCommandAdmission(cmd Command) {
	if !isDurableRoutedCommand(cmd) {
		return
	}
	m.mu.Lock()
	delete(m.admittedCommands, strings.TrimSpace(cmd.ID))
	m.mu.Unlock()
}

func runtimeCommandTargetID(run *CurrentRunView, commandType, targetID string) (string, bool) {
	targetID = strings.TrimSpace(targetID)
	if run == nil || targetID == "" {
		return "", false
	}
	for _, message := range run.Messages {
		switch commandType {
		case CommandToolApprovalResponse:
			if message.Approval != nil && (strings.TrimSpace(message.Approval.ApprovalID) == targetID || strconv.Itoa(message.Approval.ShortID) == targetID) {
				canonical := strings.TrimSpace(message.Approval.ApprovalID)
				if canonical == "" {
					canonical = strconv.Itoa(message.Approval.ShortID)
				}
				return canonical, true
			}
		case CommandUserInputResponse:
			if message.UserInput != nil && (strings.TrimSpace(message.UserInput.UserInputID) == targetID || strconv.Itoa(message.UserInput.ShortID) == targetID) {
				canonical := strings.TrimSpace(message.UserInput.UserInputID)
				if canonical == "" {
					canonical = strconv.Itoa(message.UserInput.ShortID)
				}
				return canonical, true
			}
		}
	}
	return "", false
}

func runtimeCommandTargetPresent(run *CurrentRunView, commandType, targetID string) bool {
	_, ok := runtimeCommandTargetID(run, commandType, targetID)
	return ok
}
