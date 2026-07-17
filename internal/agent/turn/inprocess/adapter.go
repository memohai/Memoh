// Package inprocess adapts turn.Service onto the in-process flow.Resolver.
// It is the migration-phase implementation; a cross-process transport will
// replace it behind the same contract.
package inprocess

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/conversation"
)

// ChatStreamer is the narrow slice of flow.Runner this adapter needs.
// Satisfied by *flow.Resolver.
type ChatStreamer interface {
	StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
}

// Adapter implements turn.Service by translating commands into
// conversation.ChatRequest and driving the resolver's stream.
type Adapter struct {
	runner ChatStreamer
}

// New creates an in-process turn service over the given runner.
func New(runner ChatStreamer) *Adapter {
	return &Adapter{runner: runner}
}

// StartTurn validates the command, starts the underlying stream, and
// returns a handle whose Events/Errs mirror the runner's channel pair.
func (a *Adapter) StartTurn(ctx context.Context, cmd turn.StartTurnCommand) (turn.RunHandle, error) {
	if cmd.TeamID == "" {
		return nil, errors.New("turn: TeamID is required")
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
		id:     uuid.NewString(),
		events: make(chan turn.Event, 16),
		errs:   make(chan error, 1),
		ctx:    runCtx,
		cancel: cancel,
		inject: injectCh,
		addAssets: func(refs []turn.OutboundAssetRef) {
			assetMu.Lock()
			defer assetMu.Unlock()
			assets = append(assets, fromAssetRefs(refs)...)
		},
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
}

func (h *runHandle) RunID() string             { return h.id }
func (h *runHandle) Events() <-chan turn.Event { return h.events }
func (h *runHandle) Errs() <-chan error        { return h.errs }
func (h *runHandle) Cancel()                   { h.cancel() }

func (h *runHandle) Inject(ctx context.Context, msg turn.InjectMessage) error {
	select {
	case h.inject <- toInjectMessage(msg):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-h.ctx.Done():
		return h.ctx.Err()
	}
}

func (h *runHandle) AddOutboundAssets(refs []turn.OutboundAssetRef) {
	h.addAssets(refs)
}

// pump forwards the runner's chunk/error pair into the handle's channels,
// wrapping each chunk as a turn.Event with a monotonically increasing Seq.
func (h *runHandle) pump(cmd turn.StartTurnCommand, chunkCh <-chan conversation.StreamChunk, errCh <-chan error) {
	defer close(h.events)
	defer close(h.errs)
	defer h.cancel()

	var seq int64
	for chunkCh != nil || errCh != nil {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				chunkCh = nil
				continue
			}
			seq++
			h.events <- turn.Event{
				RunID:     h.id,
				TeamID:    cmd.TeamID,
				SessionID: cmd.SessionID,
				Seq:       seq,
				Kind:      parseKind(chunk),
				Payload:   chunk,
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				h.errs <- err
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
