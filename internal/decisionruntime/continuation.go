package decisionruntime

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/runtimefence"
	"github.com/memohai/memoh/internal/sessionruntime"
)

func (r *Router) runContinuation(ctx context.Context, prepared runtimefence.PreservedDecision, commandType string, payload []byte, output chan<- flow.WSStreamEvent, reconcile decisionReconciler, commit decisionCommit, run continuation) error {
	streamID := "decision-" + uuid.NewString()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	abortCh := make(chan struct{}, 1)
	injectCh := make(chan conversation.InjectMessage, 16)

	var fence runtimefence.Fence
	var err error
	if r.manager.IsDistributed() {
		fence, err = r.resolver.AllocateRuntimePersistenceFence(runCtx, prepared.BotID, prepared.SessionID)
		if err != nil {
			return err
		}
	}
	fencedCtx := runCtx
	if fence.Valid() {
		fencedCtx = runtimefence.WithContext(runCtx, fence)
	}
	authorityCtx, revokeOwnership := context.WithCancelCause(context.WithoutCancel(fencedCtx))
	var handle sessionruntime.RunHandle
	runtimeCtx := flow.WithTerminalHookAuthority(fencedCtx, agentpkg.TerminalHookAuthority{
		Context: authorityCtx,
		Validate: func(validateCtx context.Context) error {
			return r.manager.ValidateRunOwnership(validateCtx, handle)
		},
	})

	handle, err = r.manager.StartRunWithOptions(runtimeCtx, sessionruntime.RunStartOptions{
		BotID:           prepared.BotID,
		SessionID:       prepared.SessionID,
		StreamID:        streamID,
		OwnershipCancel: revokeOwnership,
		AbortCh:         abortCh,
		Cancel:          cancel,
		InjectCh:        injectCh,
		AdmissionBuilder: func(admissionCtx context.Context, admitted sessionruntime.RunHandle) (sessionruntime.RunAdmissionView, error) {
			if fence.Valid() {
				admissionCtx = runtimefence.WithContext(admissionCtx, fence)
			}
			if err := r.manager.ValidateRunOwnership(admissionCtx, admitted); err != nil {
				return sessionruntime.RunAdmissionView{}, err
			}
			if fence.Valid() {
				if err := r.resolver.ActivateRuntimePersistenceFenceWithOptions(admissionCtx, fence, runtimefence.ActivationOptions{PreserveDecision: &prepared}); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
			}
			if err := r.manager.ValidateRunOwnership(admissionCtx, admitted); err != nil {
				return sessionruntime.RunAdmissionView{}, err
			}
			if err := commit(admissionCtx); err != nil {
				return sessionruntime.RunAdmissionView{}, err
			}
			if decision := sessionruntime.ResolvedDecisionFromCommand(prepared.Kind, prepared.ID, commandType, payload); decision != nil {
				return sessionruntime.RunAdmissionView{ResolvedDecision: decision}, nil
			}
			return sessionruntime.RunAdmissionView{}, nil
		},
	})
	if err != nil {
		// A concurrent responder may have installed the active continuation after
		// our first dispatch. Route to it before reporting the admission failure.
		if handled, dispatchErr := r.manager.DispatchActiveCommand(ctx, prepared.BotID, prepared.SessionID, commandType, prepared.ID, payload); handled {
			return dispatchErr
		}
		if reconcile != nil {
			if reconciled, reconcileErr := reconcile(ctx); reconciled {
				return reconcileErr
			}
		}
		return err
	}
	releaseCompaction := r.resolver.DeferSessionCompaction(prepared.BotID, prepared.SessionID, streamID)
	defer releaseCompaction()

	eventCh := make(chan flow.WSStreamEvent, streamBufferSize)
	forwardDone := make(chan error, 1)
	go func() {
		forwardDone <- r.consumeEvents(runtimeCtx, handle, eventCh, output, cancel)
	}()

	runnerCtx := flow.WithTerminalEventDeliveryTimeout(runtimeCtx, terminalFinalizationTimeout)
	runnerCtx = flow.WithPersistenceGuard(runnerCtx, func(guardCtx context.Context) error {
		return r.manager.ValidateRunOwnership(guardCtx, handle)
	})
	runErr := func() error {
		defer close(eventCh)
		return run(runnerCtx, eventCh)
	}()
	if forwardErr := <-forwardDone; forwardErr != nil {
		runErr = forwardErr
	}
	if errors.Is(runErr, sessionruntime.ErrTerminalCommitPending) {
		r.logger.Warn("decision continuation terminal commit deferred for retry", slog.String("stream_id", streamID))
		return runErr
	}

	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(runtimeCtx), terminalFinalizationTimeout)
	defer finishCancel()
	if runErr != nil {
		if finishErr := r.manager.FinishRun(finishCtx, handle, sessionruntime.RunStatusErrored, runErr.Error()); finishErr != nil && !errors.Is(finishErr, sessionruntime.ErrRunOwnershipLost) {
			r.logger.Warn("finish decision continuation after error failed", slog.Any("error", finishErr), slog.String("stream_id", streamID))
		}
		return runErr
	}
	if err := r.manager.FinishRun(finishCtx, handle, "", ""); err != nil && !errors.Is(err, sessionruntime.ErrRunOwnershipLost) {
		return err
	}
	return nil
}
