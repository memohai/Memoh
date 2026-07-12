package inbound

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
)

type routeIntent uint8

const (
	routeIntentContinue routeIntent = iota
	routeIntentQueue
	routeIntentParallel
	routeIntentStartOnly
)

type routeAdmissionKind uint8

const (
	routeAdmissionRejected routeAdmissionKind = iota
	routeAdmissionStartPrimary
	routeAdmissionStartParallel
	routeAdmissionInject
	routeAdmissionQueued
)

type injectionDeliveryResult uint8

const (
	injectionDeliveryDeferred injectionDeliveryResult = iota
	injectionDeliveryAccepted
)

type routeAdmission struct {
	Kind   routeAdmissionKind
	Lease  *routeLease
	Ticket *injectionTicket
}

type routeHandoff struct {
	consumed bool
	lease    *routeLease
	work     *deferredTurn
}

var (
	errRouteHandoffConsumed     = errors.New("route handoff already consumed")
	errRouteHandoffStartMissing = errors.New("route handoff start function missing")
)

type routeLease struct {
	dispatcher *RouteDispatcher
	routeID    string
	leaseID    uint64
	epoch      uint64
	primary    bool
	inbox      <-chan conversation.InjectMessage
}

type injectionTicket struct {
	dispatcher *RouteDispatcher
	routeID    string
	ticketID   uint64
	leaseID    uint64
	epoch      uint64
}

type routeLifecycleState struct {
	nextTicketID uint64
	nextSequence uint64
	primaryID    uint64
	leases       map[uint64]*routeLeaseState
	tickets      map[uint64]*routeTicketState
	receipts     map[string]uint64
	ticketOrder  []uint64
	backlog      []sequencedTurn
}

type routeLeaseState struct {
	lease           *routeLease
	inbox           chan conversation.InjectMessage
	cancel          context.CancelFunc
	cancelRequested bool
	handoff         *routeHandoff
}

type routeTicketState struct {
	ticket    *injectionTicket
	sequence  uint64
	work      *deferredTurn
	status    routeTicketStatus
	receiptID string
}

type routeTicketStatus uint8

const (
	routeTicketReserved routeTicketStatus = iota
	routeTicketDelivered
	routeTicketCommitted
	routeTicketFallback
)

type sequencedTurn struct {
	sequence uint64
	work     *deferredTurn
}

func (d *RouteDispatcher) Admit(routeID string, intent routeIntent, work *deferredTurn) routeAdmission {
	if d == nil || strings.TrimSpace(routeID) == "" || work == nil {
		return routeAdmission{Kind: routeAdmissionRejected}
	}
	routeID = strings.TrimSpace(routeID)
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.lifecycleState(routeID)
	sequence := state.takeSequence()
	hasActiveLease := len(state.leases) > 0
	hasPrimary := state.primaryID != 0

	switch intent {
	case routeIntentStartOnly:
		if hasPrimary || len(state.backlog) > 0 {
			return routeAdmission{Kind: routeAdmissionRejected}
		}
		return routeAdmission{Kind: routeAdmissionStartPrimary, Lease: d.newPrimaryLease(routeID, state)}
	case routeIntentParallel:
		if !hasActiveLease {
			return routeAdmission{Kind: routeAdmissionStartPrimary, Lease: d.newPrimaryLease(routeID, state)}
		}
		return routeAdmission{Kind: routeAdmissionStartParallel, Lease: d.newParallelLease(routeID, state)}
	case routeIntentQueue:
		if !hasActiveLease {
			return routeAdmission{Kind: routeAdmissionStartPrimary, Lease: d.newPrimaryLease(routeID, state)}
		}
		state.insertBacklog(sequencedTurn{sequence: sequence, work: work})
		return routeAdmission{Kind: routeAdmissionQueued}
	case routeIntentContinue:
		if hasPrimary {
			return routeAdmission{Kind: routeAdmissionInject, Ticket: d.newInjectionTicket(routeID, state, sequence, work)}
		}
		if !hasActiveLease || len(state.backlog) == 0 {
			return routeAdmission{Kind: routeAdmissionStartPrimary, Lease: d.newPrimaryLease(routeID, state)}
		}
		state.insertBacklog(sequencedTurn{sequence: sequence, work: work})
		return routeAdmission{Kind: routeAdmissionQueued}
	default:
		return routeAdmission{Kind: routeAdmissionRejected}
	}
}

func (d *RouteDispatcher) lifecycleState(routeID string) *routeLifecycleState {
	state := d.routeLifecycle[routeID]
	if state != nil {
		return state
	}
	state = &routeLifecycleState{
		leases:   make(map[uint64]*routeLeaseState),
		tickets:  make(map[uint64]*routeTicketState),
		receipts: make(map[string]uint64),
	}
	d.routeLifecycle[routeID] = state
	return state
}

func (s *routeLifecycleState) takeSequence() uint64 {
	s.nextSequence++
	return s.nextSequence
}

func (d *RouteDispatcher) newPrimaryLease(routeID string, state *routeLifecycleState) *routeLease {
	d.nextLeaseID++
	d.nextEpoch++
	inbox := make(chan conversation.InjectMessage, injectChBuffer)
	lease := &routeLease{
		dispatcher: d,
		routeID:    routeID,
		leaseID:    d.nextLeaseID,
		epoch:      d.nextEpoch,
		primary:    true,
		inbox:      inbox,
	}
	state.leases[lease.leaseID] = &routeLeaseState{lease: lease, inbox: inbox}
	state.primaryID = lease.leaseID
	return lease
}

func (d *RouteDispatcher) newParallelLease(routeID string, state *routeLifecycleState) *routeLease {
	d.nextLeaseID++
	lease := &routeLease{
		dispatcher: d,
		routeID:    routeID,
		leaseID:    d.nextLeaseID,
	}
	state.leases[lease.leaseID] = &routeLeaseState{lease: lease}
	return lease
}

func (d *RouteDispatcher) newInjectionTicket(
	routeID string,
	state *routeLifecycleState,
	sequence uint64,
	work *deferredTurn,
) *injectionTicket {
	state.nextTicketID++
	primary := state.leases[state.primaryID].lease
	ticket := &injectionTicket{
		dispatcher: d,
		routeID:    routeID,
		ticketID:   state.nextTicketID,
		leaseID:    primary.leaseID,
		epoch:      primary.epoch,
	}
	state.tickets[ticket.ticketID] = &routeTicketState{
		ticket:   ticket,
		sequence: sequence,
		work:     work,
		status:   routeTicketReserved,
	}
	state.ticketOrder = append(state.ticketOrder, ticket.ticketID)
	return ticket
}

func (t *injectionTicket) Deliver(message conversation.InjectMessage) injectionDeliveryResult {
	if t == nil || t.dispatcher == nil {
		return injectionDeliveryDeferred
	}
	d := t.dispatcher
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.routeLifecycle[t.routeID]
	if state == nil {
		return injectionDeliveryDeferred
	}
	ticketState := state.tickets[t.ticketID]
	if ticketState == nil || ticketState.status != routeTicketReserved ||
		state.primaryID != t.leaseID {
		return injectionDeliveryDeferred
	}
	leaseState := state.leases[t.leaseID]
	if leaseState == nil || leaseState.lease.epoch != t.epoch || leaseState.inbox == nil {
		return injectionDeliveryDeferred
	}
	receiptID := strings.TrimSpace(message.Receipt.ID)
	if receiptID == "" {
		ticketState.status = routeTicketFallback
		state.insertBacklog(sequencedTurn{sequence: ticketState.sequence, work: ticketState.work})
		return injectionDeliveryDeferred
	}
	if _, duplicate := state.receipts[receiptID]; duplicate {
		ticketState.status = routeTicketFallback
		state.insertBacklog(sequencedTurn{sequence: ticketState.sequence, work: ticketState.work})
		return injectionDeliveryDeferred
	}
	select {
	case leaseState.inbox <- message:
		ticketState.status = routeTicketDelivered
		ticketState.receiptID = receiptID
		state.receipts[receiptID] = t.ticketID
		return injectionDeliveryAccepted
	default:
		ticketState.status = routeTicketFallback
		state.insertBacklog(sequencedTurn{sequence: ticketState.sequence, work: ticketState.work})
		return injectionDeliveryDeferred
	}
}

func (l *routeLease) Inbox() <-chan conversation.InjectMessage {
	if l == nil {
		return nil
	}
	return l.inbox
}

func (l *routeLease) BindCancel(cancel context.CancelFunc) {
	if l == nil || l.dispatcher == nil || cancel == nil {
		return
	}
	d := l.dispatcher
	callNow := false
	d.lifecycleMu.Lock()
	if state := d.routeLifecycle[l.routeID]; state != nil {
		if leaseState := state.leases[l.leaseID]; leaseState != nil && leaseState.lease.epoch == l.epoch {
			leaseState.cancel = cancel
			callNow = leaseState.cancelRequested
		}
	}
	d.lifecycleMu.Unlock()
	if callNow {
		cancel()
	}
}

func (l *routeLease) CommitPersisted(receiptID string) bool {
	if l == nil || l.dispatcher == nil || strings.TrimSpace(receiptID) == "" {
		return false
	}
	d := l.dispatcher
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.routeLifecycle[l.routeID]
	if state == nil || state.primaryID != l.leaseID {
		return false
	}
	leaseState := state.leases[l.leaseID]
	if leaseState == nil || leaseState.lease.epoch != l.epoch {
		return false
	}
	receiptID = strings.TrimSpace(receiptID)
	ticketID, ok := state.receipts[receiptID]
	if !ok {
		return false
	}
	ticketState := state.tickets[ticketID]
	if ticketState == nil || ticketState.status != routeTicketDelivered {
		return false
	}
	ticketState.status = routeTicketCommitted
	delete(state.receipts, receiptID)
	return true
}

func (l *routeLease) Release() *routeHandoff {
	if l == nil || l.dispatcher == nil {
		return nil
	}
	d := l.dispatcher
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.routeLifecycle[l.routeID]
	if state == nil {
		return nil
	}
	leaseState := state.leases[l.leaseID]
	if leaseState == nil || leaseState.lease.epoch != l.epoch || leaseState.lease.primary != l.primary {
		return nil
	}
	if l.primary {
		drainInjectCh(leaseState.inbox)
		close(leaseState.inbox)
		for _, ticketID := range state.ticketOrder {
			ticketState := state.tickets[ticketID]
			if ticketState == nil {
				continue
			}
			switch ticketState.status {
			case routeTicketReserved, routeTicketDelivered:
				ticketState.status = routeTicketFallback
				state.insertBacklog(sequencedTurn{sequence: ticketState.sequence, work: ticketState.work})
			}
			if ticketState.receiptID != "" {
				delete(state.receipts, ticketState.receiptID)
			}
			delete(state.tickets, ticketID)
		}
		state.ticketOrder = nil
		state.primaryID = 0
	}
	delete(state.leases, l.leaseID)
	if len(state.leases) > 0 {
		return nil
	}
	if len(state.backlog) == 0 {
		delete(d.routeLifecycle, l.routeID)
		return nil
	}
	next := state.backlog[0]
	state.backlog = state.backlog[1:]
	lease := d.newPrimaryLease(l.routeID, state)
	handoff := &routeHandoff{lease: lease, work: next.work}
	state.leases[lease.leaseID].handoff = handoff
	return handoff
}

func (h *routeHandoff) Start(start func(*routeLease, *deferredTurn) error) error {
	if h == nil {
		return nil
	}
	if start == nil {
		return errRouteHandoffStartMissing
	}
	current := h
	var firstErr error
	for current != nil {
		lease, work, ok := current.claim()
		if !ok {
			if firstErr != nil {
				return firstErr
			}
			return errRouteHandoffConsumed
		}
		if err := start(lease, work); err == nil {
			return firstErr
		} else if firstErr == nil {
			firstErr = err
		}
		current = lease.Release()
	}
	return firstErr
}

func (h *routeHandoff) Abort() *routeHandoff {
	lease, _, ok := h.claim()
	if !ok {
		return nil
	}
	return lease.Release()
}

func (h *routeHandoff) claim() (*routeLease, *deferredTurn, bool) {
	if h == nil || h.lease == nil || h.lease.dispatcher == nil {
		return nil, nil, false
	}
	d := h.lease.dispatcher
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.routeLifecycle[h.lease.routeID]
	if h.consumed || state == nil || state.primaryID != h.lease.leaseID {
		return nil, nil, false
	}
	leaseState := state.leases[h.lease.leaseID]
	if leaseState == nil || leaseState.lease.epoch != h.lease.epoch || leaseState.handoff != h {
		return nil, nil, false
	}
	h.consumed = true
	leaseState.handoff = nil
	return h.lease, h.work, true
}

func (d *RouteDispatcher) CancelAll(routeID string) int {
	if d == nil || strings.TrimSpace(routeID) == "" {
		return 0
	}
	routeID = strings.TrimSpace(routeID)
	var cancels []context.CancelFunc
	d.lifecycleMu.Lock()
	state := d.routeLifecycle[routeID]
	if state != nil {
		for _, leaseState := range state.leases {
			if leaseState.cancel == nil {
				leaseState.cancelRequested = true
				continue
			}
			cancels = append(cancels, leaseState.cancel)
		}
	}
	count := 0
	if state != nil {
		count = len(state.leases)
	}
	d.lifecycleMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return count
}

func (s *routeLifecycleState) insertBacklog(turn sequencedTurn) {
	index := sort.Search(len(s.backlog), func(i int) bool {
		return s.backlog[i].sequence > turn.sequence
	})
	s.backlog = append(s.backlog, sequencedTurn{})
	copy(s.backlog[index+1:], s.backlog[index:])
	s.backlog[index] = turn
}
