package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type TurnRunMode string

const (
	TurnRunModeLegacy       TurnRunMode = "legacy"
	TurnRunModeNormal       TurnRunMode = "normal"
	TurnRunModeRewrite      TurnRunMode = "rewrite"
	TurnRunModeContinuation TurnRunMode = "continuation"
)

type TurnContextScopeKind string

const (
	ContextScopeEmpty       TurnContextScopeKind = "empty"
	ContextScopeTurnHead    TurnContextScopeKind = "turn_head"
	ContextScopeSessionHead TurnContextScopeKind = "session_head"
	ContextScopeBotHistory  TurnContextScopeKind = "bot_history"
)

type TurnContextScope struct {
	Kind   TurnContextScopeKind
	TurnID string
}

type VariantTransitionAction string

const (
	VariantTransitionNone            VariantTransitionAction = "none"
	VariantTransitionReplaceBaseHead VariantTransitionAction = "replace_base_head"
	VariantTransitionCreateSibling   VariantTransitionAction = "create_sibling"
)

type VariantTransition struct {
	Action         VariantTransitionAction
	SessionID      string
	BaseHeadTurnID string
}

type TurnRun struct {
	Mode          TurnRunMode
	PersistTurnID string
	Context       TurnContextScope
	Turn          TurnPersistencePlan
	Variant       VariantTransition
}

func (run TurnRun) ViewHeadTurnID() string {
	switch run.Context.Kind {
	case ContextScopeTurnHead:
		return strings.TrimSpace(run.Context.TurnID)
	case ContextScopeEmpty:
		return ""
	case ContextScopeSessionHead, ContextScopeBotHistory:
		return strings.TrimSpace(run.Variant.BaseHeadTurnID)
	default:
		return ""
	}
}

type TurnPersistencePlan struct {
	BotID          string
	OwnerSessionID string
	ParentTurnID   string
	// OriginKind, OriginTurnID and RequestGroupID are provenance metadata
	// persisted on the created turn row. OriginTurnID points at the turn a
	// retry/edit was anchored on; RequestGroupID is inherited from the retry
	// source so sibling turns that carry the same logical request share a
	// group (empty means the turn is its own group).
	OriginKind     conversation.TurnOriginKind
	OriginTurnID   string
	RequestGroupID string
}

func sessionTurnKey(botID, sessionID string) string {
	return strings.TrimSpace(botID) + ":" + strings.TrimSpace(sessionID)
}

func (r *Resolver) enterSessionTurn(_ context.Context, botID, sessionID string) func() {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return func() {}
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	lock := r.sessionTurnLockLocked(key)
	r.sessionTurnRefs[key]++
	r.sessionTurnMu.Unlock()

	lock.Lock()
	return r.makeSessionTurnReleaser(key, lock)
}

func (r *Resolver) tryEnterIdleSessionTurn(_ context.Context, botID, sessionID string) (func(), bool) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return func() {}, true
	}

	key := sessionTurnKey(botID, sessionID)
	r.sessionTurnMu.Lock()
	lock := r.sessionTurnLockLocked(key)
	if r.sessionTurnRefs[key] > 0 {
		r.sessionTurnMu.Unlock()
		return func() {}, false
	}
	r.sessionTurnRefs[key] = 1
	lock.Lock()
	r.sessionTurnMu.Unlock()
	return r.makeSessionTurnReleaser(key, lock), true
}

func (r *Resolver) sessionTurnLockLocked(key string) *sync.Mutex {
	if r.sessionTurnRefs == nil {
		r.sessionTurnRefs = make(map[string]int)
	}
	if r.sessionTurnLocks == nil {
		r.sessionTurnLocks = make(map[string]*sync.Mutex)
	}
	lock := r.sessionTurnLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		r.sessionTurnLocks[key] = lock
	}
	return lock
}

func (r *Resolver) makeSessionTurnReleaser(key string, lock *sync.Mutex) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if lock != nil {
				lock.Unlock()
			}
			r.sessionTurnMu.Lock()
			switch refs := r.sessionTurnRefs[key] - 1; {
			case refs > 0:
				r.sessionTurnRefs[key] = refs
			default:
				delete(r.sessionTurnRefs, key)
			}
			r.sessionTurnMu.Unlock()
		})
	}
}

type turnStore interface {
	CreateHistoryTurn(context.Context, dbsqlc.CreateHistoryTurnParams) (dbsqlc.BotHistoryTurn, error)
	CreateSessionTurnHead(context.Context, dbsqlc.CreateSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	GetSessionByID(context.Context, pgtype.UUID) (dbsqlc.BotSession, error)
	GetSessionTurnHead(context.Context, dbsqlc.GetSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	GetVisibleUserMessageTurnForRewrite(context.Context, dbsqlc.GetVisibleUserMessageTurnForRewriteParams) (dbsqlc.GetVisibleUserMessageTurnForRewriteRow, error)
	ReplaceSessionTurnHead(context.Context, dbsqlc.ReplaceSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	UpdateSessionDefaultHeadTurnIfValid(context.Context, dbsqlc.UpdateSessionDefaultHeadTurnIfValidParams) (dbsqlc.BotSession, error)
}

type sessionHeadStore interface {
	GetSessionByID(context.Context, pgtype.UUID) (dbsqlc.BotSession, error)
	GetSessionTurnHead(context.Context, dbsqlc.GetSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
}

type continuationTurnStore interface {
	GetSessionByID(context.Context, pgtype.UUID) (dbsqlc.BotSession, error)
	GetHistoryTurnByID(context.Context, pgtype.UUID) (dbsqlc.BotHistoryTurn, error)
	GetSessionTurnHead(context.Context, dbsqlc.GetSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	ListHistoryTurnPathFromHead(context.Context, pgtype.UUID) ([]dbsqlc.BotHistoryTurn, error)
}

type turnTxRunner interface {
	RunInTx(ctx context.Context, fn func(dbstore.Queries) error) error
}

type resolvedBaseHead struct {
	HeadTurnID pgtype.UUID
}

func legacyTurnRun(req conversation.ChatRequest) TurnRun {
	scope := TurnContextScope{Kind: ContextScopeBotHistory}
	if strings.TrimSpace(req.SessionID) != "" {
		scope = TurnContextScope{Kind: ContextScopeSessionHead}
	}
	return TurnRun{
		Mode:    TurnRunModeLegacy,
		Context: scope,
	}
}

func continuationTurnRun(sessionID, persistTurnID string) TurnRun {
	sessionID = strings.TrimSpace(sessionID)
	persistTurnID = strings.TrimSpace(persistTurnID)
	if persistTurnID == "" {
		scope := TurnContextScope{Kind: ContextScopeBotHistory}
		if sessionID != "" {
			scope = TurnContextScope{Kind: ContextScopeSessionHead}
		}
		return TurnRun{
			Mode:    TurnRunModeContinuation,
			Context: scope,
		}
	}
	return TurnRun{
		Mode:          TurnRunModeContinuation,
		PersistTurnID: persistTurnID,
		Context: TurnContextScope{
			Kind:   ContextScopeTurnHead,
			TurnID: persistTurnID,
		},
	}
}

func (r *Resolver) validateContinuationTurnHead(ctx context.Context, sessionID, persistTurnID string) error {
	sessionID = strings.TrimSpace(sessionID)
	persistTurnID = strings.TrimSpace(persistTurnID)
	if sessionID == "" || persistTurnID == "" {
		return nil
	}
	if r == nil || r.queries == nil {
		return nil
	}
	store, ok := r.queries.(continuationTurnStore)
	if !ok {
		return nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: invalid session id: %w", err)
	}
	pgTurnID, err := dbpkg.ParseUUID(persistTurnID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: invalid persist turn id: %w", err)
	}
	sess, err := store.GetSessionByID(ctx, pgSessionID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: get session: %w", err)
	}
	turn, err := store.GetHistoryTurnByID(ctx, pgTurnID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("validate continuation turn: pending turn is no longer active for this session")
		}
		return fmt.Errorf("validate continuation turn: get history turn: %w", err)
	}
	if !turn.OwnerSessionID.Valid || turn.OwnerSessionID != pgSessionID || turn.BotID != sess.BotID {
		return errors.New("validate continuation turn: pending turn is no longer active for this session")
	}
	return nil
}

func (r *Resolver) validateBaseContinuationTurnHead(ctx context.Context, sessionID, persistTurnID, baseHeadTurnID, label string) error {
	persistTurnID = strings.TrimSpace(persistTurnID)
	baseHeadTurnID = strings.TrimSpace(baseHeadTurnID)
	if persistTurnID == "" {
		return nil
	}
	if err := r.validateContinuationTurnHead(ctx, sessionID, persistTurnID); err != nil {
		return err
	}
	if baseHeadTurnID == "" {
		return nil
	}
	if strings.TrimSpace(label) == "" {
		label = "continuation"
	}
	if r == nil || r.queries == nil {
		return nil
	}
	store, ok := r.queries.(continuationTurnStore)
	if !ok {
		return nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: invalid session id: %w", err)
	}
	pgBaseHeadTurnID, err := dbpkg.ParseUUID(baseHeadTurnID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: invalid base head turn id: %w", err)
	}
	pgPersistTurnID, err := dbpkg.ParseUUID(persistTurnID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: invalid persist turn id: %w", err)
	}
	if _, err := store.GetSessionTurnHead(ctx, dbsqlc.GetSessionTurnHeadParams{
		SessionID:  pgSessionID,
		HeadTurnID: pgBaseHeadTurnID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%s turn is no longer active for the requested conversation version", label)
		}
		return fmt.Errorf("validate continuation turn: get base head: %w", err)
	}
	path, err := store.ListHistoryTurnPathFromHead(ctx, pgBaseHeadTurnID)
	if err != nil {
		return fmt.Errorf("validate continuation turn: list base head path: %w", err)
	}
	for _, turn := range path {
		if turn.ID == pgPersistTurnID {
			return nil
		}
	}
	return fmt.Errorf("%s turn is no longer active for the requested conversation version", label)
}

func contextScopeFromParentTurn(parentTurnID pgtype.UUID) TurnContextScope {
	if !parentTurnID.Valid {
		return TurnContextScope{Kind: ContextScopeEmpty}
	}
	return TurnContextScope{
		Kind:   ContextScopeTurnHead,
		TurnID: parentTurnID.String(),
	}
}

func turnRunAllowsPipelineContext(run TurnRun) bool {
	switch run.Context.Kind {
	case ContextScopeSessionHead, ContextScopeBotHistory:
		return true
	default:
		return false
	}
}

func (r *Resolver) prepareTurnRun(ctx context.Context, req conversation.ChatRequest) (TurnRun, error) {
	return r.prepareTurnRunWithRewriteAnchor(ctx, req, nil)
}

func (r *Resolver) prepareTurnRunWithRewriteAnchor(ctx context.Context, req conversation.ChatRequest, rewriteAnchor *conversation.TurnAnchor) (TurnRun, error) {
	if r == nil || r.queries == nil {
		return legacyTurnRun(req), nil
	}
	botID := strings.TrimSpace(req.BotID)
	sessionID := strings.TrimSpace(req.SessionID)
	if botID == "" || sessionID == "" {
		return legacyTurnRun(req), nil
	}
	store, ok := r.queries.(turnStore)
	if !ok {
		return legacyTurnRun(req), nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return TurnRun{}, fmt.Errorf("prepare turn run: invalid session id: %w", err)
	}
	mode, parentTurnID, baseHead, origin, err := r.resolveTurnRunParent(ctx, store, pgSessionID, req, rewriteAnchor)
	if err != nil {
		return TurnRun{}, err
	}
	baseHeadID := ""
	if baseHead.HeadTurnID.Valid {
		baseHeadID = baseHead.HeadTurnID.String()
	}
	return TurnRun{
		Mode:    mode,
		Context: contextScopeFromParentTurn(parentTurnID),
		Turn: TurnPersistencePlan{
			BotID:          botID,
			OwnerSessionID: sessionID,
			ParentTurnID:   uuidString(parentTurnID),
			OriginKind:     origin.Kind,
			OriginTurnID:   origin.TurnID,
			RequestGroupID: origin.RequestGroupID,
		},
		Variant: VariantTransition{
			Action:         variantTransitionActionForTurnRun(mode, sessionID),
			SessionID:      sessionID,
			BaseHeadTurnID: baseHeadID,
		},
	}, nil
}

// turnOrigin captures provenance for the turn a run is about to create.
type turnOrigin struct {
	Kind           conversation.TurnOriginKind
	TurnID         string
	RequestGroupID string
}

func originFromAnchor(anchor conversation.TurnAnchor) turnOrigin {
	kind := anchor.OriginKind
	if kind == "" {
		kind = conversation.TurnOriginEdit
	}
	return turnOrigin{
		Kind:           kind,
		TurnID:         strings.TrimSpace(anchor.TurnID),
		RequestGroupID: strings.TrimSpace(anchor.RequestGroupID),
	}
}

func (*Resolver) resolveTurnRunParent(ctx context.Context, store turnStore, sessionID pgtype.UUID, req conversation.ChatRequest, rewriteAnchor *conversation.TurnAnchor) (TurnRunMode, pgtype.UUID, resolvedBaseHead, turnOrigin, error) {
	sess, err := store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, fmt.Errorf("prepare turn run: get session: %w", err)
	}
	variantsEnabled := sessionpkg.SupportsTurnVariants(sess.Type)

	if rewriteAnchor != nil {
		if !variantsEnabled {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, errors.New("turn variants are only supported for chat sessions")
		}
		if rewriteAnchor.Role != conversation.TurnAnchorRoleUser {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, errors.New("prepare turn run: rewrite anchor is not a user message")
		}
		if strings.TrimSpace(rewriteAnchor.BaseHeadTurnID) == "" {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, errors.New("prepare turn run: rewrite anchor base head turn id is required")
		}
		baseHead, err := resolveBaseSessionHead(ctx, store, sessionID, rewriteAnchor.BaseHeadTurnID)
		if err != nil {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, fmt.Errorf("prepare turn run: validate rewrite anchor base head: %w", err)
		}
		parentTurnID, err := parseOptionalUUID(rewriteAnchor.ParentTurnID)
		if err != nil {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, fmt.Errorf("prepare turn run: invalid rewrite anchor parent turn id: %w", err)
		}
		return TurnRunModeRewrite, parentTurnID, baseHead, originFromAnchor(*rewriteAnchor), nil
	}

	requestedBaseHeadTurnID := req.BaseHeadTurnID
	if !variantsEnabled {
		requestedBaseHeadTurnID = ""
	}
	baseHead, err := resolveBaseSessionHeadFromSession(ctx, store, sessionID, sess, requestedBaseHeadTurnID)
	if err != nil {
		return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, err
	}

	rewriteTargetID := strings.TrimSpace(req.RewriteTargetMessageID)
	if rewriteTargetID != "" {
		if !variantsEnabled {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, errors.New("turn variants are only supported for chat sessions")
		}
		anchor, err := resolveVisibleUserTurnAnchor(ctx, store, sessionID, baseHead.HeadTurnID, rewriteTargetID)
		if err != nil {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, err
		}
		parentTurnID, err := parseOptionalUUID(anchor.ParentTurnID)
		if err != nil {
			return "", pgtype.UUID{}, resolvedBaseHead{}, turnOrigin{}, fmt.Errorf("prepare turn run: invalid rewrite anchor parent turn id: %w", err)
		}
		return TurnRunModeRewrite, parentTurnID, baseHead, originFromAnchor(anchor), nil
	}

	return TurnRunModeNormal, baseHead.HeadTurnID, baseHead, turnOrigin{Kind: conversation.TurnOriginMessage}, nil
}

func resolveBaseSessionHead(ctx context.Context, store sessionHeadStore, sessionID pgtype.UUID, requestedHeadTurnID string) (resolvedBaseHead, error) {
	sess, err := store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return resolvedBaseHead{}, fmt.Errorf("prepare turn run: get session: %w", err)
	}
	return resolveBaseSessionHeadFromSession(ctx, store, sessionID, sess, requestedHeadTurnID)
}

func resolveBaseSessionHeadFromSession(ctx context.Context, store sessionHeadStore, sessionID pgtype.UUID, sess dbsqlc.BotSession, requestedHeadTurnID string) (resolvedBaseHead, error) {
	baseHeadID := sess.DefaultHeadTurnID
	if requestedHead := strings.TrimSpace(requestedHeadTurnID); requestedHead != "" {
		parsed, err := dbpkg.ParseUUID(requestedHead)
		if err != nil {
			return resolvedBaseHead{}, fmt.Errorf("prepare turn run: invalid base head turn id: %w", err)
		}
		if _, err := store.GetSessionTurnHead(ctx, dbsqlc.GetSessionTurnHeadParams{
			SessionID:  sessionID,
			HeadTurnID: parsed,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return resolvedBaseHead{}, errors.New("prepare turn run: base head is not valid for this session")
			}
			return resolvedBaseHead{}, fmt.Errorf("prepare turn run: validate base head: %w", err)
		}
		baseHeadID = parsed
	} else if baseHeadID.Valid {
		if _, err := store.GetSessionTurnHead(ctx, dbsqlc.GetSessionTurnHeadParams{
			SessionID:  sessionID,
			HeadTurnID: baseHeadID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return resolvedBaseHead{}, errors.New("prepare turn run: default head is not valid for this session")
			}
			return resolvedBaseHead{}, fmt.Errorf("prepare turn run: validate default head: %w", err)
		}
	}
	return resolvedBaseHead{HeadTurnID: baseHeadID}, nil
}

func resolveVisibleUserTurnAnchor(ctx context.Context, store turnStore, sessionID pgtype.UUID, baseHeadTurnID pgtype.UUID, messageID string) (conversation.TurnAnchor, error) {
	pgMessageID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		return conversation.TurnAnchor{}, fmt.Errorf("prepare turn run: invalid rewrite target message id: %w", err)
	}
	row, err := store.GetVisibleUserMessageTurnForRewrite(ctx, dbsqlc.GetVisibleUserMessageTurnForRewriteParams{
		MessageID:      pgMessageID,
		BaseHeadTurnID: baseHeadTurnID,
		SessionID:      sessionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return conversation.TurnAnchor{}, errors.New("prepare turn run: rewrite target is not a visible user message")
		}
		return conversation.TurnAnchor{}, fmt.Errorf("prepare turn run: resolve rewrite target: %w", err)
	}
	return conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleUser,
		MessageID:      row.MessageID.String(),
		TurnID:         row.TurnID.String(),
		ParentTurnID:   uuidString(row.ParentTurnID),
		BaseHeadTurnID: baseHeadTurnID.String(),
		OriginKind:     conversation.TurnOriginEdit,
	}, nil
}

func parseOptionalUUID(id string) (pgtype.UUID, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return pgtype.UUID{}, nil
	}
	return dbpkg.ParseUUID(trimmed)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func variantTransitionActionForTurnRun(mode TurnRunMode, sessionID string) VariantTransitionAction {
	if strings.TrimSpace(sessionID) == "" {
		return VariantTransitionNone
	}
	switch mode {
	case TurnRunModeNormal:
		return VariantTransitionReplaceBaseHead
	case TurnRunModeRewrite:
		return VariantTransitionCreateSibling
	default:
		return VariantTransitionNone
	}
}

func normalizeVariantTransitionAction(action VariantTransitionAction) VariantTransitionAction {
	if action == "" {
		return VariantTransitionNone
	}
	return action
}

func (r *Resolver) ensurePersistTurn(ctx context.Context, run *TurnRun) (string, error) {
	return r.ensurePersistTurnWithQueries(ctx, nil, run)
}

func (r *Resolver) ensurePersistTurnWithQueries(ctx context.Context, queries dbstore.Queries, run *TurnRun) (string, error) {
	if run == nil {
		return "", nil
	}
	if turnID := strings.TrimSpace(run.PersistTurnID); turnID != "" {
		return turnID, nil
	}
	if r == nil {
		return "", nil
	}
	r.persistTurnMu.Lock()
	defer r.persistTurnMu.Unlock()
	if turnID := strings.TrimSpace(run.PersistTurnID); turnID != "" {
		return turnID, nil
	}
	plan := run.Turn
	if strings.TrimSpace(plan.BotID) == "" || strings.TrimSpace(plan.OwnerSessionID) == "" {
		return "", nil
	}
	if r.queries == nil && queries == nil {
		return "", nil
	}
	rawQueries := queries
	if rawQueries == nil {
		rawQueries = r.queries
	}
	store, ok := rawQueries.(turnStore)
	if !ok {
		return "", nil
	}
	pgBotID, err := dbpkg.ParseUUID(plan.BotID)
	if err != nil {
		return "", fmt.Errorf("ensure persist turn: invalid bot id: %w", err)
	}
	pgSessionID, err := dbpkg.ParseUUID(plan.OwnerSessionID)
	if err != nil {
		return "", fmt.Errorf("ensure persist turn: invalid session id: %w", err)
	}
	parentTurnID, err := parseOptionalUUID(plan.ParentTurnID)
	if err != nil {
		return "", fmt.Errorf("ensure persist turn: invalid parent turn id: %w", err)
	}
	originTurnID, err := parseOptionalUUID(plan.OriginTurnID)
	if err != nil {
		return "", fmt.Errorf("ensure persist turn: invalid origin turn id: %w", err)
	}
	requestGroupID, err := parseOptionalUUID(plan.RequestGroupID)
	if err != nil {
		return "", fmt.Errorf("ensure persist turn: invalid request group id: %w", err)
	}
	originKind := pgtype.Text{}
	if kind := strings.TrimSpace(string(plan.OriginKind)); kind != "" {
		originKind = pgtype.Text{String: kind, Valid: true}
	}
	turn, err := store.CreateHistoryTurn(ctx, dbsqlc.CreateHistoryTurnParams{
		BotID:          pgBotID,
		OwnerSessionID: pgSessionID,
		ParentTurnID:   parentTurnID,
		OriginKind:     originKind,
		OriginTurnID:   originTurnID,
		RequestGroupID: requestGroupID,
	})
	if err != nil {
		return "", fmt.Errorf("ensure persist turn: create history turn: %w", err)
	}
	run.PersistTurnID = turn.ID.String()
	return run.PersistTurnID, nil
}

func (r *Resolver) applyVariantTransition(ctx context.Context, run *TurnRun, turnID string) error {
	if r == nil || r.queries == nil {
		return nil
	}
	if run == nil || normalizeVariantTransitionAction(run.Variant.Action) == VariantTransitionNone {
		return nil
	}
	if runner, ok := r.queries.(turnTxRunner); ok && runner != nil {
		return runner.RunInTx(ctx, func(q dbstore.Queries) error {
			return r.applyVariantTransitionWithQueries(ctx, q, run, turnID)
		})
	}
	return r.applyVariantTransitionWithQueries(ctx, r.queries, run, turnID)
}

func (*Resolver) applyVariantTransitionWithQueries(ctx context.Context, queries dbstore.Queries, run *TurnRun, turnID string) error {
	if run == nil {
		return nil
	}
	transition := run.Variant
	transition.Action = normalizeVariantTransitionAction(transition.Action)
	if transition.Action == VariantTransitionNone {
		return nil
	}
	sessionID := strings.TrimSpace(transition.SessionID)
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		turnID = strings.TrimSpace(run.PersistTurnID)
	}
	if sessionID == "" || turnID == "" {
		return nil
	}
	store, ok := queries.(turnStore)
	if !ok {
		return errors.New("apply variant transition: queries do not support turn updates")
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("apply variant transition: invalid session id: %w", err)
	}
	pgTurnID, err := dbpkg.ParseUUID(turnID)
	if err != nil {
		return fmt.Errorf("apply variant transition: invalid turn id: %w", err)
	}
	baseHeadText := strings.TrimSpace(transition.BaseHeadTurnID)
	switch transition.Action {
	case VariantTransitionReplaceBaseHead:
		if baseHeadText == "" {
			if _, err := store.CreateSessionTurnHead(ctx, dbsqlc.CreateSessionTurnHeadParams{
				SessionID:  pgSessionID,
				HeadTurnID: pgTurnID,
			}); err != nil {
				return fmt.Errorf("apply variant transition: create initial turn head: %w", err)
			}
		} else {
			pgBaseHeadTurnID, err := dbpkg.ParseUUID(baseHeadText)
			if err != nil {
				return fmt.Errorf("apply variant transition: invalid base head turn id: %w", err)
			}
			if _, err := store.ReplaceSessionTurnHead(ctx, dbsqlc.ReplaceSessionTurnHeadParams{
				TargetSessionID: pgSessionID,
				OldHeadTurnID:   pgBaseHeadTurnID,
				NewHeadTurnID:   pgTurnID,
			}); err != nil {
				return fmt.Errorf("apply variant transition: replace base head %q: %w", baseHeadText, err)
			}
		}
	case VariantTransitionCreateSibling:
		if _, err := store.CreateSessionTurnHead(ctx, dbsqlc.CreateSessionTurnHeadParams{
			SessionID:  pgSessionID,
			HeadTurnID: pgTurnID,
		}); err != nil {
			return fmt.Errorf("apply variant transition: create turn head: %w", err)
		}
	default:
		return fmt.Errorf("apply variant transition: unknown action %q", transition.Action)
	}
	if _, err := store.UpdateSessionDefaultHeadTurnIfValid(ctx, dbsqlc.UpdateSessionDefaultHeadTurnIfValidParams{
		ID:                pgSessionID,
		DefaultHeadTurnID: pgTurnID,
	}); err != nil {
		return fmt.Errorf("apply variant transition: update default head: %w", err)
	}
	return nil
}
