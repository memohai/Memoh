package inbound

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/slash"
)

type activeStreamRegistry struct {
	mu     sync.Mutex
	nextID uint64
	scopes map[string]string
	owners map[string]map[uint64]activeStreamOwner
}

var errStaleRouteScope = errors.New("route session generation is stale")

const unverifiedRouteScope = "\x00unverified"

type activeStreamOwner struct {
	scopeID string
	cancel  context.CancelFunc
}

type routeTransitionRegistry struct {
	mu     sync.Mutex
	routes map[string]*sync.Mutex
}

func (r *routeTransitionRegistry) Lock(routeID string) func() {
	r.mu.Lock()
	if r.routes == nil {
		r.routes = make(map[string]*sync.Mutex)
	}
	lock := r.routes[strings.TrimSpace(routeID)]
	if lock == nil {
		lock = &sync.Mutex{}
		r.routes[strings.TrimSpace(routeID)] = lock
	}
	r.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (r *activeStreamRegistry) Register(key, scopeID string, cancel context.CancelFunc) (uint64, bool) {
	if cancel == nil {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scopes == nil {
		r.scopes = make(map[string]string)
	}
	if r.owners == nil {
		r.owners = make(map[string]map[uint64]activeStreamOwner)
	}
	if current, known := r.scopes[key]; known && current != scopeID {
		return 0, false
	}
	r.scopes[key] = scopeID
	r.nextID++
	if r.nextID == 0 {
		r.nextID++
	}
	owners := r.owners[key]
	if owners == nil {
		owners = make(map[uint64]activeStreamOwner)
		r.owners[key] = owners
	}
	owners[r.nextID] = activeStreamOwner{scopeID: scopeID, cancel: cancel}
	return r.nextID, true
}

func (r *activeStreamRegistry) Remove(key string, ownerID uint64) {
	if ownerID == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	owners := r.owners[key]
	delete(owners, ownerID)
	if len(owners) == 0 {
		delete(r.owners, key)
	}
}

func (r *activeStreamRegistry) CancelAll(key string) int {
	r.mu.Lock()
	owners := r.owners[key]
	delete(r.owners, key)
	cancels := make([]context.CancelFunc, 0, len(owners))
	for _, owner := range owners {
		cancels = append(cancels, owner.cancel)
	}
	r.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

func (r *activeStreamRegistry) CurrentScope(key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	scopeID, known := r.scopes[key]
	return scopeID, known
}

func (r *activeStreamRegistry) AdvanceScope(key, scopeID string) int {
	r.mu.Lock()
	if r.scopes == nil {
		r.scopes = make(map[string]string)
	}
	if current, known := r.scopes[key]; known && current == scopeID {
		r.mu.Unlock()
		return 0
	}
	r.scopes[key] = scopeID
	cancels := r.detachOwnersOutsideScope(key, scopeID)
	r.mu.Unlock()
	cancelActiveStreamOwners(cancels)
	return len(cancels)
}

func (r *activeStreamRegistry) detachOwnersOutsideScope(key, scopeID string) []context.CancelFunc {
	owners := r.owners[key]
	cancels := make([]context.CancelFunc, 0, len(owners))
	for ownerID, owner := range owners {
		if owner.scopeID == scopeID {
			continue
		}
		cancels = append(cancels, owner.cancel)
		delete(owners, ownerID)
	}
	if len(owners) == 0 {
		delete(r.owners, key)
	}
	return cancels
}

func cancelActiveStreamOwners(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}

func (r *activeStreamRegistry) Count(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.owners[key])
}

func (p *ChannelInboundProcessor) admitRouteTurn(
	ctx context.Context,
	routeID string,
	intent routeIntent,
	turn *deferredTurn,
) routeAdmission {
	if p.dispatcher == nil || turn == nil {
		return routeAdmission{Kind: routeAdmissionRejected}
	}
	routeID = strings.TrimSpace(routeID)
	scopeID := strings.TrimSpace(turn.sessionID)
	unlock := p.routeTransitions.Lock(routeID)
	defer unlock()
	if err := p.validateRouteScopeLocked(ctx, turn.identity.BotID, routeID, scopeID); err != nil {
		return routeAdmission{Kind: routeAdmissionStale, Err: err}
	}
	return p.dispatcher.Admit(routeID, intent, turn)
}

func (p *ChannelInboundProcessor) registerActiveStream(
	ctx context.Context,
	botID string,
	key string,
	routeID string,
	scopeID string,
	cancel context.CancelFunc,
) (uint64, error) {
	unlock := p.routeTransitions.Lock(routeID)
	defer unlock()
	if err := p.validateRouteScopeLocked(ctx, botID, routeID, scopeID); err != nil {
		return 0, err
	}
	ownerID, accepted := p.activeStreams.Register(key, scopeID, cancel)
	if !accepted {
		return 0, errStaleRouteScope
	}
	return ownerID, nil
}

func (p *ChannelInboundProcessor) advanceRouteGenerationLocked(
	botID string,
	routeID string,
	scopeID string,
	reservedLease *routeLease,
) (int, error) {
	canceled := 0
	adopted := reservedLease == nil
	if reservedLease != nil {
		adopted = reservedLease.AdoptScope(scopeID)
	}
	if !adopted || reservedLease == nil {
		if p.dispatcher != nil {
			canceled += p.dispatcher.AdvanceScope(routeID, scopeID)
		}
	}
	key := strings.TrimSpace(botID) + ":" + strings.TrimSpace(routeID)
	canceled += p.activeStreams.AdvanceScope(key, strings.TrimSpace(scopeID))
	if !adopted {
		return canceled, errStaleRouteScope
	}
	return canceled, nil
}

func (p *ChannelInboundProcessor) validateRouteScopeLocked(ctx context.Context, botID, routeID, scopeID string) error {
	scopeID = strings.TrimSpace(scopeID)
	if p.sessionEnsurer != nil {
		active, err := p.sessionEnsurer.GetActiveSession(ctx, routeID)
		if err != nil {
			return fmt.Errorf("revalidate active route session: %w", err)
		}
		activeScopeID := strings.TrimSpace(active.ID)
		if _, advanceErr := p.advanceRouteGenerationLocked(botID, routeID, activeScopeID, nil); advanceErr != nil {
			return advanceErr
		}
		if activeScopeID != scopeID {
			return errStaleRouteScope
		}
		return nil
	}
	if current, known := p.dispatcher.CurrentScope(routeID); known && current != scopeID {
		return errStaleRouteScope
	}
	key := strings.TrimSpace(botID) + ":" + strings.TrimSpace(routeID)
	if current, known := p.activeStreams.CurrentScope(key); known && current != scopeID {
		return errStaleRouteScope
	}
	return nil
}

func (p *ChannelInboundProcessor) reserveSessionlessRouteStart(routeID string, turn *deferredTurn) (routeAdmission, func()) {
	if p.dispatcher == nil || turn == nil {
		return routeAdmission{Kind: routeAdmissionRejected}, nil
	}
	unlock := p.routeTransitions.Lock(routeID)
	if scopeID, known := p.dispatcher.CurrentScope(routeID); known {
		turn.sessionID = scopeID
	}
	admission := p.dispatcher.Admit(routeID, routeIntentStartOnly, turn)
	if admission.Kind != routeAdmissionStartPrimary || admission.Lease == nil {
		unlock()
		return admission, nil
	}
	return admission, unlock
}

func (p *ChannelInboundProcessor) ensureRouteSessionLocked(
	ctx context.Context,
	botID string,
	routeID string,
	channelType string,
	spec NewSessionSpec,
	reservedLease *routeLease,
) (SessionResult, int, error) {
	if active, err := p.sessionEnsurer.GetActiveSession(ctx, routeID); err == nil && strings.TrimSpace(active.ID) != "" {
		canceled, advanceErr := p.advanceRouteGenerationLocked(botID, routeID, active.ID, reservedLease)
		return active, canceled, advanceErr
	}
	created, err := p.sessionEnsurer.CreateNewSession(ctx, botID, routeID, channelType, spec)
	if err != nil {
		return SessionResult{}, 0, err
	}
	return p.activateCreatedRouteSessionLocked(ctx, botID, routeID, created, reservedLease)
}

func (p *ChannelInboundProcessor) ensureRouteSession(
	ctx context.Context,
	botID string,
	routeID string,
	channelType string,
	spec NewSessionSpec,
) (SessionResult, int, error) {
	unlock := p.routeTransitions.Lock(routeID)
	defer unlock()
	return p.ensureRouteSessionLocked(ctx, botID, routeID, channelType, spec, nil)
}

func (p *ChannelInboundProcessor) createNewRouteSession(
	ctx context.Context,
	botID string,
	routeID string,
	channelType string,
	spec NewSessionSpec,
) (SessionResult, int, error) {
	unlock := p.routeTransitions.Lock(routeID)
	defer unlock()
	created, err := p.sessionEnsurer.CreateNewSession(ctx, botID, routeID, channelType, spec)
	if err != nil {
		return SessionResult{}, 0, err
	}
	return p.activateCreatedRouteSessionLocked(ctx, botID, routeID, created, nil)
}

func (p *ChannelInboundProcessor) activateCreatedRouteSessionLocked(
	ctx context.Context,
	botID string,
	routeID string,
	created SessionResult,
	reservedLease *routeLease,
) (SessionResult, int, error) {
	active, err := p.sessionEnsurer.GetActiveSession(ctx, routeID)
	if err != nil {
		canceled, _ := p.advanceRouteGenerationLocked(botID, routeID, unverifiedRouteScope, nil)
		return SessionResult{}, canceled, fmt.Errorf("verify created route session: %w", err)
	}
	if strings.TrimSpace(active.ID) != strings.TrimSpace(created.ID) {
		canceled, _ := p.advanceRouteGenerationLocked(botID, routeID, active.ID, nil)
		return SessionResult{}, canceled, fmt.Errorf("created session %q is not active for route", strings.TrimSpace(created.ID))
	}
	canceled, err := p.advanceRouteGenerationLocked(botID, routeID, created.ID, reservedLease)
	return created, canceled, err
}

func (p *ChannelInboundProcessor) dispatchDeferredTurn(
	ctx context.Context,
	turn *deferredTurn,
	reservedLease *routeLease,
) error {
	if turn == nil {
		return errors.New("deferred turn missing")
	}
	if reservedLease != nil {
		return p.runDeferredTurn(ctx, turn, reservedLease)
	}
	if p.dispatcher == nil || isLocalChannelType(turn.msg.Channel) {
		return p.runDeferredTurn(ctx, turn, nil)
	}

	routeID := strings.TrimSpace(turn.resolved.RouteID)
	admission := p.admitRouteTurn(ctx, routeID, routeIntentForTurn(turn), turn)
	switch admission.Kind {
	case routeAdmissionStartPrimary, routeAdmissionStartParallel:
		return p.runDeferredTurn(ctx, turn, admission.Lease)
	case routeAdmissionQueued:
		p.sendModeConfirmation(ctx, turn.sender, turn.msg, turn.identity, "queue")
		return nil
	case routeAdmissionInject:
		turn.ActivateOnce(ctx, p)
		receipt := turn.receipt
		delivery := admission.Ticket.Deliver(conversation.InjectMessage{
			Text:        turn.text,
			Attachments: receipt.Attachments,
			HeaderifiedText: flow.FormatUserHeader(flow.UserMessageHeaderInput{
				MessageID:         strings.TrimSpace(turn.msg.Message.ID),
				ChannelIdentityID: strings.TrimSpace(turn.identity.ChannelIdentityID),
				DisplayName:       strings.TrimSpace(turn.identity.DisplayName),
				Channel:           turn.msg.Channel.String(),
				ConversationType:  strings.TrimSpace(turn.msg.Conversation.Type),
				ConversationName:  strings.TrimSpace(turn.msg.Conversation.Name),
				Target:            strings.TrimSpace(turn.msg.ReplyTarget),
				AttachmentPaths:   collectAttachmentPaths(turn.attachments),
				Time:              time.Now().UTC(),
			}, turn.text),
			Receipt: receipt,
		})
		if delivery == injectionDeliveryAccepted {
			p.sendModeConfirmation(ctx, turn.sender, turn.msg, turn.identity, "inject")
		} else {
			p.sendModeConfirmation(ctx, turn.sender, turn.msg, turn.identity, "queue")
		}
		return nil
	case routeAdmissionRejected:
		if turn.hasPendingSkill {
			return p.sendSlashError(ctx, turn.sender, turn.msg, slash.CodeUnsupportedSkillSlashContext)
		}
		return errors.New("route admission rejected")
	case routeAdmissionStale:
		if admission.Err != nil {
			return admission.Err
		}
		return errStaleRouteScope
	default:
		return errors.New("unsupported route admission")
	}
}

func routeIntentForTurn(turn *deferredTurn) routeIntent {
	if turn.hasPendingSkill && turn.inboundMode != ModeParallel {
		return routeIntentStartOnly
	}
	switch turn.inboundMode {
	case ModeQueue:
		return routeIntentQueue
	case ModeParallel:
		return routeIntentParallel
	default:
		return routeIntentContinue
	}
}

func (p *ChannelInboundProcessor) releaseRouteLease(ctx context.Context, lease *routeLease) {
	if lease == nil {
		return
	}
	p.startRouteHandoff(ctx, lease.Release())
}

func (p *ChannelInboundProcessor) startRouteHandoff(ctx context.Context, handoff *routeHandoff) {
	if handoff == nil {
		return
	}
	err := handoff.Start(func(lease *routeLease, turn *deferredTurn) error {
		go func() {
			runCtx := context.WithoutCancel(ctx) //nolint:contextcheck // handoff owns detached work beyond the releasing turn
			if turn.ctx != nil {
				runCtx = turn.ctx //nolint:contextcheck // queued work retains its original detached admission values
			}
			if err := p.runDeferredTurn(runCtx, turn, lease); err != nil && p.logger != nil {
				p.logger.Error("deferred turn processing failed",
					slog.String("route_id", strings.TrimSpace(lease.routeID)),
					slog.Any("error", err))
			}
		}()
		return nil
	})
	if err != nil && p.logger != nil {
		p.logger.Error("route handoff failed", slog.Any("error", err))
	}
}
