package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func sessionTurnKey(botID, sessionID string) string {
	return strings.TrimSpace(botID) + ":" + strings.TrimSpace(sessionID)
}

func (r *Resolver) enterSessionTurn(ctx context.Context, botID, sessionID string) func() {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return func() {}
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	if r.sessionTurnRefs == nil {
		r.sessionTurnRefs = make(map[string]int)
	}
	r.sessionTurnRefs[key]++
	r.sessionTurnMu.Unlock()

	return r.makeSessionTurnReleaser(ctx, key, botID, sessionID)
}

func (r *Resolver) pinPersistBranch(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error) {
	if r == nil || r.queries == nil || strings.TrimSpace(req.PersistBranchID) != "" {
		return req, nil
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return req, nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return req, fmt.Errorf("pin persist branch: invalid session id: %w", err)
	}
	branchID, err := r.queries.GetActiveSessionBranch(ctx, pgSessionID)
	if err != nil {
		return req, fmt.Errorf("pin persist branch: get active branch: %w", err)
	}
	if !branchID.Valid {
		branchID, err = r.queries.GetRootSessionBranch(ctx, pgSessionID)
		if errors.Is(err, pgx.ErrNoRows) {
			branchID, err = r.queries.CreateRootSessionBranch(ctx, pgSessionID)
			if err != nil {
				if existing, getErr := r.queries.GetRootSessionBranch(ctx, pgSessionID); getErr == nil {
					branchID = existing
				} else {
					return req, fmt.Errorf("pin persist branch: create root branch: %w", err)
				}
			}
		} else if err != nil {
			return req, fmt.Errorf("pin persist branch: get root branch: %w", err)
		}
		rows, err := r.queries.SetActiveSessionBranch(ctx, dbsqlc.SetActiveSessionBranchParams{
			SessionID: pgSessionID,
			BranchID:  branchID,
		})
		if err != nil {
			return req, fmt.Errorf("pin persist branch: set active branch: %w", err)
		}
		if rows == 0 {
			return req, errors.New("pin persist branch: session branch not found")
		}
	}
	if branchID.Valid {
		req.PersistBranchID = branchID.String()
	}
	return req, nil
}

func (r *Resolver) pinPersistTurn(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error) {
	if r == nil || r.queries == nil || strings.TrimSpace(req.PersistTurnID) != "" {
		return req, nil
	}
	req, err := r.pinPersistBranch(ctx, req)
	if err != nil {
		return req, err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	branchID := strings.TrimSpace(req.PersistBranchID)
	if sessionID == "" || branchID == "" {
		return req, nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return req, fmt.Errorf("pin persist turn: invalid session id: %w", err)
	}
	pgBranchID, err := dbpkg.ParseUUID(branchID)
	if err != nil {
		return req, fmt.Errorf("pin persist turn: invalid branch id: %w", err)
	}
	turnID, err := r.queries.CreateHistoryTurn(ctx, dbsqlc.CreateHistoryTurnParams{
		SessionID: pgSessionID,
		BranchID:  pgBranchID,
	})
	if err != nil {
		return req, fmt.Errorf("pin persist turn: create history turn: %w", err)
	}
	req.PersistTurnID = turnID.String()
	return req, nil
}

func (r *Resolver) cleanupEmptyPersistTurn(ctx context.Context, req conversation.ChatRequest) {
	if r == nil || r.queries == nil {
		return
	}
	sessionID := strings.TrimSpace(req.SessionID)
	branchID := strings.TrimSpace(req.PersistBranchID)
	turnID := strings.TrimSpace(req.PersistTurnID)
	if sessionID == "" || branchID == "" || turnID == "" {
		return
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return
	}
	pgBranchID, err := dbpkg.ParseUUID(branchID)
	if err != nil {
		return
	}
	pgTurnID, err := dbpkg.ParseUUID(turnID)
	if err != nil {
		return
	}
	if _, err := r.queries.CancelEmptyHistoryTurn(ctx, dbsqlc.CancelEmptyHistoryTurnParams{
		TurnID:    pgTurnID,
		SessionID: pgSessionID,
		BranchID:  pgBranchID,
	}); err != nil && r.logger != nil {
		r.logger.Warn("cleanup empty history turn failed",
			slog.String("turn_id", turnID),
			slog.Any("error", err),
		)
	}
}

func (r *Resolver) tryEnterIdleSessionTurn(ctx context.Context, botID, sessionID string) (func(), bool) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return nil, false
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	if r.sessionTurnRefs == nil {
		r.sessionTurnRefs = make(map[string]int)
	}
	if r.sessionTurnRefs[key] > 0 {
		r.sessionTurnMu.Unlock()
		return nil, false
	}
	r.sessionTurnRefs[key] = 1
	r.sessionTurnMu.Unlock()

	return r.makeSessionTurnReleaser(ctx, key, botID, sessionID), true
}

func (r *Resolver) makeSessionTurnReleaser(ctx context.Context, key, botID, sessionID string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			becameIdle := false

			r.sessionTurnMu.Lock()
			switch refs := r.sessionTurnRefs[key] - 1; {
			case refs > 0:
				r.sessionTurnRefs[key] = refs
			default:
				delete(r.sessionTurnRefs, key)
				becameIdle = true
			}
			r.sessionTurnMu.Unlock()

			if becameIdle {
				r.maybeTriggerDeferredBackgroundNotifications(ctx, botID, sessionID)
			}
		})
	}
}

func (r *Resolver) markDeferredBackgroundNotification(botID, sessionID string) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return
	}
	r.bgNotifDeferred.Store(sessionTurnKey(botID, sessionID), true)
}

func (r *Resolver) takeDeferredBackgroundNotification(botID, sessionID string) bool {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return false
	}
	_, loaded := r.bgNotifDeferred.LoadAndDelete(sessionTurnKey(botID, sessionID))
	return loaded
}

func (r *Resolver) maybeTriggerDeferredBackgroundNotifications(ctx context.Context, botID, sessionID string) {
	if !r.takeDeferredBackgroundNotification(botID, sessionID) {
		return
	}
	if r.bgManager == nil || !r.bgManager.HasNotifications(botID, sessionID) {
		return
	}

	r.logger.Info("background notification trigger queued after session became idle",
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
	)
	if ctx == nil {
		return
	}
	go r.TriggerBackgroundNotification(context.WithoutCancel(ctx), botID, sessionID)
}
