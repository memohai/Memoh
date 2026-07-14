package inbound

import (
	"context"
	"strings"
)

func (l *routeLease) BindCancel(cancel context.CancelFunc) {
	if l == nil || cancel == nil {
		return
	}
	l.cancelMu.Lock()
	if l.cancel != nil {
		l.cancelMu.Unlock()
		return
	}
	l.cancel = cancel
	callNow := l.cancelRequested
	l.cancelMu.Unlock()
	if callNow {
		cancel()
	}
}

func (l *routeLease) AdoptScope(scopeID string) bool {
	if l == nil || l.dispatcher == nil {
		return false
	}
	d := l.dispatcher
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.routeLifecycle[l.routeID]
	if state == nil || state.primaryID != l.leaseID || len(state.leases) != 1 ||
		len(state.backlog) != 0 || len(state.tickets) != 0 {
		return false
	}
	leaseState := state.leases[l.leaseID]
	if leaseState == nil || leaseState.lease.epoch != l.epoch || !leaseState.lease.primary {
		return false
	}
	state.scopeID = strings.TrimSpace(scopeID)
	return true
}

func (l *routeLease) requestCancel() {
	if l == nil {
		return
	}
	l.cancelMu.Lock()
	if l.cancelRequested {
		l.cancelMu.Unlock()
		return
	}
	l.cancelRequested = true
	cancel := l.cancel
	l.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (d *RouteDispatcher) CancelAll(routeID string) int {
	if d == nil || strings.TrimSpace(routeID) == "" {
		return 0
	}
	routeID = strings.TrimSpace(routeID)
	var leases []*routeLease
	d.lifecycleMu.Lock()
	state := d.routeLifecycle[routeID]
	if state != nil {
		for _, leaseState := range state.leases {
			leases = append(leases, leaseState.lease)
		}
	}
	d.lifecycleMu.Unlock()
	cancelRouteLeases(leases)
	return len(leases)
}

func (d *RouteDispatcher) CurrentScope(routeID string) (string, bool) {
	if d == nil || strings.TrimSpace(routeID) == "" {
		return "", false
	}
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	state := d.routeLifecycle[strings.TrimSpace(routeID)]
	if state == nil {
		return "", false
	}
	return state.scopeID, true
}

func (d *RouteDispatcher) AdvanceScope(routeID, scopeID string) int {
	if d == nil || strings.TrimSpace(routeID) == "" {
		return 0
	}
	routeID = strings.TrimSpace(routeID)
	scopeID = strings.TrimSpace(scopeID)
	d.lifecycleMu.Lock()
	state := d.routeLifecycle[routeID]
	if state != nil && state.scopeID == scopeID {
		d.lifecycleMu.Unlock()
		return 0
	}
	var leases []*routeLease
	if state != nil {
		leases = d.detachLifecycleState(routeID, state)
	}
	d.lifecycleState(routeID, scopeID)
	d.lifecycleMu.Unlock()
	cancelRouteLeases(leases)
	return len(leases)
}

func (d *RouteDispatcher) detachLifecycleState(routeID string, state *routeLifecycleState) []*routeLease {
	leases := make([]*routeLease, 0, len(state.leases))
	for _, leaseState := range state.leases {
		leases = append(leases, leaseState.lease)
		if leaseState.inbox != nil {
			drainInjectCh(leaseState.inbox)
			close(leaseState.inbox)
		}
	}
	delete(d.routeLifecycle, routeID)
	return leases
}

func cancelRouteLeases(leases []*routeLease) {
	for _, lease := range leases {
		lease.requestCancel()
	}
}
