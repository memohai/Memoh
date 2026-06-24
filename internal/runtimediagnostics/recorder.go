package runtimediagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

const (
	defaultRetentionWindow  = 30 * 24 * time.Hour
	defaultPerBotEventLimit = 200
	recordEventTimeout      = 2 * time.Second
)

type EventInput struct {
	BotID     string
	Scope     string
	AgentID   string
	SessionID string
	RuntimeID string
	Phase     string
	Severity  string
	Code      string
	Message   string
	Metadata  map[string]any
}

type Recorder struct {
	queries         dbstore.Queries
	now             func() time.Time
	retentionWindow time.Duration
	perBotLimit     int32
}

func NewRecorder(queries dbstore.Queries) *Recorder {
	return &Recorder{
		queries:         queries,
		now:             time.Now,
		retentionWindow: defaultRetentionWindow,
		perBotLimit:     defaultPerBotEventLimit,
	}
}

func (r *Recorder) Record(ctx context.Context, input EventInput) error {
	if r == nil || r.queries == nil {
		return errors.New("runtime diagnostic recorder is not configured")
	}
	botID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return fmt.Errorf("invalid bot id: %w", err)
	}
	sessionID := dbpkg.ParseUUIDOrEmpty(input.SessionID)
	scope := normalizeEventScope(input.Scope)
	severity := normalizeEventSeverity(input.Severity)
	metadata, err := json.Marshal(normalizeMetadata(SanitizeEventMetadata(input.Metadata)))
	if err != nil {
		return fmt.Errorf("marshal runtime diagnostic metadata: %w", err)
	}
	if _, err := r.queries.CreateRuntimeDiagnosticEvent(ctx, dbsqlc.CreateRuntimeDiagnosticEventParams{
		BotID:     botID,
		Scope:     scope,
		AgentID:   strings.TrimSpace(input.AgentID),
		SessionID: sessionID,
		RuntimeID: strings.TrimSpace(input.RuntimeID),
		Phase:     fallback(strings.TrimSpace(input.Phase), "runtime"),
		Severity:  severity,
		Code:      fallback(strings.TrimSpace(input.Code), "runtime_diagnostic_event"),
		Message:   truncate(SanitizeEventMessage(strings.TrimSpace(input.Message)), 2048),
		Metadata:  metadata,
	}); err != nil {
		return err
	}

	_ = r.pruneRetention(ctx)
	_ = r.pruneBotLimit(ctx, botID)
	return nil
}

func (r *Recorder) Prune(ctx context.Context) error {
	if r == nil || r.queries == nil {
		return errors.New("runtime diagnostic recorder is not configured")
	}
	if err := r.pruneRetention(ctx); err != nil {
		return err
	}
	if r.perBotLimit <= 0 {
		return nil
	}
	botIDs, err := r.queries.ListRuntimeDiagnosticEventBotIDs(ctx)
	if err != nil {
		return err
	}
	for _, botID := range botIDs {
		if err := r.pruneBotLimit(ctx, botID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recorder) pruneRetention(ctx context.Context) error {
	if r == nil || r.queries == nil || r.retentionWindow <= 0 {
		return nil
	}
	before := pgtype.Timestamptz{Time: r.now().Add(-r.retentionWindow), Valid: true}
	return r.queries.DeleteRuntimeDiagnosticEventsBefore(ctx, before)
}

func (r *Recorder) pruneBotLimit(ctx context.Context, botID pgtype.UUID) error {
	if r == nil || r.queries == nil || r.perBotLimit <= 0 || !botID.Valid {
		return nil
	}
	return r.queries.PruneRuntimeDiagnosticEventsByBotLimit(ctx, dbsqlc.PruneRuntimeDiagnosticEventsByBotLimitParams{
		BotID:     botID,
		KeepCount: r.perBotLimit,
	})
}

func (r *Recorder) RecordRuntimeDiagnosticEvent(ctx context.Context, botID, scope, agentID, sessionID, runtimeID, phase, severity, code, message string, metadata map[string]any) { //nolint:contextcheck // diagnostic event persistence must outlive canceled runtime request contexts.
	if ctx == nil {
		ctx = context.Background()
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordEventTimeout)
	defer cancel()
	_ = r.Record(recordCtx, EventInput{
		BotID:     botID,
		Scope:     scope,
		AgentID:   agentID,
		SessionID: sessionID,
		RuntimeID: runtimeID,
		Phase:     phase,
		Severity:  severity,
		Code:      code,
		Message:   message,
		Metadata:  metadata,
	})
}

func (r *Recorder) ListRecent(ctx context.Context, botID string, limit int32) ([]RuntimeEventSummary, error) {
	if r == nil || r.queries == nil {
		return nil, nil
	}
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, fmt.Errorf("invalid bot id: %w", err)
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := r.queries.ListRuntimeDiagnosticEventsByBot(ctx, dbsqlc.ListRuntimeDiagnosticEventsByBotParams{
		BotID:      pgBotID,
		LimitCount: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RuntimeEventSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, runtimeEventFromRow(row))
	}
	return out, nil
}

func runtimeEventFromRow(row dbsqlc.RuntimeDiagnosticEvent) RuntimeEventSummary {
	var metadata map[string]any
	_ = json.Unmarshal(row.Metadata, &metadata)
	return RuntimeEventSummary{
		ID:        uuidToString(row.ID),
		BotID:     uuidToString(row.BotID),
		Scope:     row.Scope,
		AgentID:   row.AgentID,
		SessionID: uuidToString(row.SessionID),
		RuntimeID: row.RuntimeID,
		Phase:     row.Phase,
		Severity:  row.Severity,
		Code:      row.Code,
		Message:   row.Message,
		Metadata:  metadata,
		CreatedAt: dbpkg.TimeFromPg(row.CreatedAt),
	}
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func normalizeEventScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "workspace", "container", "display", "acp":
		return strings.ToLower(strings.TrimSpace(scope))
	default:
		return "acp"
	}
}

func normalizeEventSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "info", "warn", "error":
		return strings.ToLower(strings.TrimSpace(severity))
	default:
		return "error"
	}
}

func normalizeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return defaultValue
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
