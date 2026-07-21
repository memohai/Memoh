// Package inprocess adapts turn.Service onto the in-process flow.Resolver.
// It is the migration-phase implementation; a cross-process transport will
// replace it behind the same contract.
package inprocess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/userinput"
)

// ChatStreamer is the narrow slice of flow.Runner this adapter needs.
// Satisfied by *flow.Resolver.
type ChatStreamer interface {
	StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
}

// toolApprovalResponder matches flow.Resolver's tool-approval resume method.
type toolApprovalResponder interface {
	RespondToolApproval(ctx context.Context, input flow.ToolApprovalResponseInput, eventCh chan<- flow.WSStreamEvent) error
}

// userInputResponder matches flow.Resolver's user-input resume method.
type userInputResponder interface {
	RespondUserInput(ctx context.Context, input flow.UserInputResponseInput, eventCh chan<- flow.WSStreamEvent) error
}

// Adapter implements turn.Service by translating commands into
// conversation.ChatRequest and driving the resolver's stream.
type Adapter struct {
	runner      ChatStreamer
	discuss     *discussDeps
	allowedTeam string
	idem        *idempotencyRegistry
}

// Option configures optional adapter capabilities.
type Option func(*Adapter)

// WithAllowedTeam restricts the adapter to a single team. The in-process
// runtime's database pool is session-bound to one team GUC, so commands
// for any other team must fail closed (turn.ErrTeamNotServed) instead of
// silently operating on the bound team's data. The composition root
// injects the self-hosted singleton team; a hosted multi-team runtime
// replaces this with request-scoped team binding.
func WithAllowedTeam(teamID string) Option {
	return func(a *Adapter) {
		a.allowedTeam = teamID
	}
}

// WithDiscuss enables discuss-mode turns backed by the native agent and
// the resolver's run-config/persistence surface.
func WithDiscuss(agent AgentStreamer, resolver DiscussResolver) Option {
	return func(a *Adapter) {
		a.discuss = &discussDeps{agent: agent, resolver: resolver}
	}
}

// New creates an in-process turn service over the given runner.
func New(runner ChatStreamer, opts ...Option) *Adapter {
	a := &Adapter{runner: runner, idem: newIdempotencyRegistry(idempotencyCapacity)}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// newRunID mints a run identifier.
func newRunID() string { return uuid.NewString() }

// StartTurn validates the command, starts the underlying stream, and
// returns a handle whose Events/Errs mirror the runner's channel pair.
func (a *Adapter) StartTurn(ctx context.Context, cmd turn.StartTurnCommand) (turn.RunHandle, error) {
	if cmd.TeamID == "" {
		return nil, errors.New("turn: TeamID is required")
	}
	if a.allowedTeam != "" && cmd.TeamID != a.allowedTeam {
		return nil, fmt.Errorf("%w: %s", turn.ErrTeamNotServed, cmd.TeamID)
	}
	if cmd.Mode == turn.ModeDiscuss && (a.discuss == nil || a.discuss.agent == nil || a.discuss.resolver == nil) {
		return nil, errors.New("turn: discuss runtime not configured")
	}
	var releaseClaim func()
	if cmd.IdempotencyKey != "" {
		if !a.idem.claim(cmd.TeamID, cmd.IdempotencyKey) {
			return nil, fmt.Errorf("%w: %s", turn.ErrDuplicateTurn, cmd.IdempotencyKey)
		}
		teamID, key := cmd.TeamID, cmd.IdempotencyKey
		releaseClaim = func() { a.idem.release(teamID, key) }
	}
	if cmd.Mode == turn.ModeDiscuss {
		return a.startDiscussTurn(ctx, cmd, releaseClaim)
	}
	runCtx, cancel := context.WithCancel(ctx)

	injectCh := make(chan conversation.InjectMessage, 16)
	var (
		assetMu sync.Mutex
		assets  []conversation.OutboundAssetRef
	)

	req := chatRequestFromCommand(cmd)
	req.InjectCh = injectCh
	req.OutboundAssetCollector = func() []conversation.OutboundAssetRef {
		assetMu.Lock()
		defer assetMu.Unlock()
		out := make([]conversation.OutboundAssetRef, len(assets))
		copy(out, assets)
		return out
	}

	chunkCh, errCh := a.runner.StreamChat(runCtx, req)

	h := &runHandle{
		id:     newRunID(),
		events: make(chan turn.Event, 16),
		errs:   make(chan error, 1),
		ctx:    runCtx,
		cancel: cancel,
		inject: injectCh,
		addAssets: func(refs []turn.OutboundAssetRef) {
			assetMu.Lock()
			defer assetMu.Unlock()
			assets = append(assets, refs...)
		},
		releaseClaim: releaseClaim,
	}
	go h.pump(cmd, chunkCh, errCh)
	return h, nil
}

type runHandle struct {
	id        string
	events    chan turn.Event
	errs      chan error
	ctx       context.Context
	cancel    context.CancelFunc
	inject    chan conversation.InjectMessage
	addAssets func([]turn.OutboundAssetRef)

	// injectMu guards inject against send-after-close: the pump closes the
	// channel when the run ends so the resolver's forwarding goroutine
	// (which ranges over it) can exit instead of leaking per turn.
	injectMu     sync.Mutex
	injectClosed bool

	// failed records that the run ended in an error or cancellation; finish
	// then releases the idempotency claim so a redelivery can retry.
	failed       atomic.Bool
	releaseClaim func()
}

func (h *runHandle) RunID() string             { return h.id }
func (h *runHandle) Events() <-chan turn.Event { return h.events }
func (h *runHandle) Errs() <-chan error        { return h.errs }
func (h *runHandle) Cancel()                   { h.cancel() }

func (h *runHandle) Inject(ctx context.Context, msg turn.InjectMessage) error {
	h.injectMu.Lock()
	defer h.injectMu.Unlock()
	if h.injectClosed {
		if err := h.ctx.Err(); err != nil {
			return err
		}
		return errors.New("turn: run already finished")
	}
	select {
	case h.inject <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-h.ctx.Done():
		return h.ctx.Err()
	}
}

// closeInject closes the inject channel exactly once. Safe against a
// concurrent Inject: the pump cancels the run context before closing, so
// any in-flight Inject unblocks via ctx.Done and drops the mutex first.
func (h *runHandle) closeInject() {
	h.injectMu.Lock()
	defer h.injectMu.Unlock()
	if h.injectClosed {
		return
	}
	h.injectClosed = true
	close(h.inject)
}

// finish releases the idempotency claim for runs that did not complete
// cleanly. Runs the last thing in the pump's defer stack.
func (h *runHandle) finish() {
	if h.failed.Load() && h.releaseClaim != nil {
		h.releaseClaim()
	}
}

// RespondToolApproval resumes a turn deferred on tool approval by
// delegating to the resolver (RFC ResumeApprovalCommand).
func (a *Adapter) RespondToolApproval(ctx context.Context, input turn.ToolApprovalResponse, eventCh chan<- json.RawMessage) error {
	responder, ok := a.runner.(toolApprovalResponder)
	if !ok {
		return errors.New("turn: runner does not support tool approval resume")
	}
	return responder.RespondToolApproval(ctx, toolApprovalInputFromResponse(input), eventCh)
}

// RespondUserInput resumes a turn deferred on ask_user by delegating to
// the resolver (RFC ResumeUserInputCommand).
func (a *Adapter) RespondUserInput(ctx context.Context, input turn.UserInputResponse, eventCh chan<- json.RawMessage) error {
	responder, ok := a.runner.(userInputResponder)
	if !ok {
		return errors.New("turn: runner does not support user input resume")
	}
	return responder.RespondUserInput(ctx, userInputInputFromResponse(input), eventCh)
}

// plainTextAdvancer matches flow.Resolver's plain-text ask_user advance method.
type plainTextAdvancer interface {
	AdvancePlainTextUserInput(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error)
}

// AdvancePlainTextUserInput resumes a pending ask_user question from a plain
// text reply by delegating to the resolver.
func (a *Adapter) AdvancePlainTextUserInput(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error) {
	advancer, ok := a.runner.(plainTextAdvancer)
	if !ok {
		return userinput.AdvanceTextResult{}, errors.New("turn: runner does not support plain text user input")
	}
	return advancer.AdvancePlainTextUserInput(ctx, input)
}

func (h *runHandle) AddOutboundAssets(refs []turn.OutboundAssetRef) {
	h.addAssets(refs)
}

// pump forwards the runner's chunk/error pair into the handle's channels,
// wrapping each chunk as a turn.Event with a monotonically increasing Seq.
func (h *runHandle) pump(cmd turn.StartTurnCommand, chunkCh <-chan conversation.StreamChunk, errCh <-chan error) {
	// Deferred order (LIFO): detect external cancellation, cancel, close
	// inject, release a failed claim, then close the channel pair — so by
	// the time a consumer observes the closed channels, the idempotency
	// claim of a failed/canceled run has already been released.
	defer close(h.events)
	defer close(h.errs)
	defer h.finish()
	defer h.closeInject()
	defer func() {
		// A canceled run may look like a clean completion here: the
		// resolver reacts to ctx cancellation by closing both channels.
		// Check before cancel() masks the distinction.
		if h.ctx.Err() != nil {
			h.failed.Store(true)
		}
		h.cancel()
	}()

	var seq int64
	for chunkCh != nil || errCh != nil {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				chunkCh = nil
				continue
			}
			seq++
			select {
			case h.events <- turn.Event{
				RunID:     h.id,
				TeamID:    cmd.TeamID,
				SessionID: cmd.SessionID,
				Seq:       seq,
				Kind:      parseKind(chunk),
				Payload:   chunk,
			}:
			case <-h.ctx.Done():
				h.failed.Store(true)
				return
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				h.failed.Store(true)
				select {
				case h.errs <- err:
				case <-h.ctx.Done():
					return
				}
			}
		}
	}
}

// parseKind extracts the "type" field from a raw chunk, best effort.
func parseKind(p json.RawMessage) string {
	var env struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(p, &env) != nil {
		return ""
	}
	return env.Type
}
