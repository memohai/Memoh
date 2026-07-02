package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SeedCatalog struct {
	Marker       string        `json:"marker"`
	BotIDs       []uuid.UUID   `json:"bot_ids"`
	Sessions     []SessionSeed `json:"sessions"`
	HotSessions  []int         `json:"hot_sessions"`
	ColdSessions []int         `json:"cold_sessions"`
	Estimate     SeedEstimate  `json:"estimate"`
}

type SessionSeed struct {
	BotID              uuid.UUID   `json:"bot_id"`
	OwnerUserID        uuid.UUID   `json:"owner_user_id"`
	RouteID            uuid.UUID   `json:"route_id"`
	SessionID          uuid.UUID   `json:"session_id"`
	DefaultHeadTurnID  uuid.UUID   `json:"default_head_turn_id"`
	HeadTurnIDs        []uuid.UUID `json:"head_turn_ids"`
	MidPathTurnID      uuid.UUID   `json:"mid_path_turn_id"`
	PageTurnIDs        []uuid.UUID `json:"page_turn_ids"`
	LatestMessageID    uuid.UUID   `json:"latest_message_id"`
	CursorMessageIDs   []uuid.UUID `json:"cursor_message_ids"`
	CursorCreatedAts   []time.Time `json:"cursor_created_ats"`
	ExternalMessageID  string      `json:"external_message_id"`
	ApprovalRequestID  uuid.UUID   `json:"approval_request_id"`
	ApprovalBaseReqID  uuid.UUID   `json:"approval_base_request_id"`
	ApprovalShortID    int32       `json:"approval_short_id"`
	ApprovalPromptID   string      `json:"approval_prompt_external_id"`
	UserInputRequestID uuid.UUID   `json:"user_input_request_id"`
	UserInputBaseReqID uuid.UUID   `json:"user_input_base_request_id"`
	UserInputShortID   int32       `json:"user_input_short_id"`
	UserInputPromptID  string      `json:"user_input_prompt_external_id"`
}

type sessionRequestSeq struct {
	approval  int32
	userInput int32
}

type turnSeed struct {
	id         uuid.UUID
	parentID   uuid.UUID
	createdAt  time.Time
	sessionIdx int
	seq        int
}

type assetSeedRow struct {
	id          uuid.UUID
	messageID   uuid.UUID
	role        string
	ordinal     int
	contentHash string
	name        string
	metadata    json.RawMessage
	createdAt   time.Time
}

type turnPointerUpdate struct {
	turnID      uuid.UUID
	requestID   uuid.UUID
	assistantID uuid.UUID
}

type pendingToolCallSeed struct {
	id    string
	name  string
	input json.RawMessage
}

type approvalInsertShape struct {
	columns             []string
	hasOperation        bool
	hasRequestedMessage bool
	hasPromptMessage    bool
	hasPromptExternal   bool
}

type userInputInsertShape struct {
	columns           []string
	hasPromptMessage  bool
	hasPromptExternal bool
}

func seedBenchmarkData(ctx context.Context, pool *pgxpool.Pool, cfg Config) (SeedCatalog, error) {
	if cfg.Seed.CleanupBefore {
		if err := cleanupBenchmarkData(ctx, pool, cfg.Seed.Marker); err != nil {
			return SeedCatalog{}, fmt.Errorf("cleanup before seed: %w", err)
		}
	}
	approvalShape, err := loadApprovalInsertShape(ctx, pool)
	if err != nil {
		return SeedCatalog{}, err
	}
	userInputShape, err := loadUserInputInsertShape(ctx, pool)
	if err != nil {
		return SeedCatalog{}, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return SeedCatalog{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	now := time.Now().UTC().Add(-24 * time.Hour)
	markerJSON := jsonText(map[string]any{
		"benchmark":        benchmarkName,
		"benchmark_marker": cfg.Seed.Marker,
	})

	userBatch := newCopyBatcher(ctx, tx, "users", []string{"id", "username", "email", "role", "display_name", "timezone", "metadata", "created_at", "updated_at"}, 5000)
	identityBatch := newCopyBatcher(ctx, tx, "channel_identities", []string{"id", "channel_type", "channel_subject_id", "display_name", "metadata", "created_at", "updated_at"}, 5000)
	botBatch := newCopyBatcher(ctx, tx, "bots", []string{"id", "owner_user_id", "name", "display_name", "timezone", "metadata", "created_at", "updated_at"}, 5000)
	routeBatch := newCopyBatcher(ctx, tx, "bot_channel_routes", []string{"id", "bot_id", "channel_type", "external_conversation_id", "external_thread_id", "conversation_type", "default_reply_target", "metadata", "created_at", "updated_at"}, 5000)
	sessionBatch := newCopyBatcher(ctx, tx, "bot_sessions", []string{"id", "bot_id", "route_id", "channel_type", "type", "title", "metadata", "created_by_user_id", "created_at", "updated_at"}, 5000)
	turnBatch := newCopyBatcher(ctx, tx, "bot_history_turns", []string{"id", "bot_id", "owner_session_id", "parent_turn_id", "created_at", "updated_at"}, 10000)
	headBatch := newCopyBatcher(ctx, tx, "bot_session_turn_heads", []string{"session_id", "head_turn_id", "bot_id", "created_at", "updated_at"}, 10000)
	messageBatch := newCopyBatcher(ctx, tx, "bot_history_messages", []string{"id", "bot_id", "session_id", "turn_id", "turn_message_seq", "sender_channel_identity_id", "sender_account_user_id", "source_message_id", "source_reply_to_message_id", "role", "content", "metadata", "usage", "display_text", "created_at"}, 10000)
	approvalBatch := newCopyBatcher(ctx, tx, "tool_approval_requests", approvalShape.columns, 5000)
	userInputBatch := newCopyBatcher(ctx, tx, "user_input_requests", userInputShape.columns, 5000)
	assetBatch := newCopyBatcher(ctx, tx, "bot_history_message_assets", []string{"id", "message_id", "role", "ordinal", "content_hash", "name", "metadata", "created_at"}, 5000)

	catalog := SeedCatalog{
		Marker:   cfg.Seed.Marker,
		BotIDs:   make([]uuid.UUID, 0, cfg.Seed.Bots),
		Sessions: make([]SessionSeed, 0, cfg.Seed.Bots*cfg.Seed.SessionsPerBot),
		Estimate: estimateSeed(cfg),
	}

	sessionGlobalIdx := 0
	messageGlobalIdx := 0
	turnGlobalIdx := 0
	approvalGlobalIdx := 0
	userInputGlobalIdx := 0
	for botIdx := 0; botIdx < cfg.Seed.Bots; botIdx++ {
		userID := uuid.New()
		identityID := uuid.New()
		botID := uuid.New()
		catalog.BotIDs = append(catalog.BotIDs, botID)

		if err := userBatch.add(userID, uniqueName("bench-user", cfg.Seed.Marker, botIdx), uniqueEmail(cfg.Seed.Marker, botIdx), "member", fmt.Sprintf("Bench User %d", botIdx), "UTC", markerJSON, now, now); err != nil {
			return SeedCatalog{}, err
		}
		if err := identityBatch.add(identityID, "local", uniqueName("bench-subject", cfg.Seed.Marker, botIdx), fmt.Sprintf("Bench Identity %d", botIdx), markerJSON, now, now); err != nil {
			return SeedCatalog{}, err
		}
		if err := botBatch.add(botID, userID, uniqueBotName(cfg.Seed.Marker, botIdx), fmt.Sprintf("Bench Bot %d", botIdx), "UTC", markerJSON, now, now); err != nil {
			return SeedCatalog{}, err
		}
		for _, batch := range []*copyBatcher{userBatch, identityBatch, botBatch} {
			if err := batch.flush(); err != nil {
				return SeedCatalog{}, err
			}
		}

		for sessIdx := 0; sessIdx < cfg.Seed.SessionsPerBot; sessIdx++ {
			sessionIdx := sessionGlobalIdx
			sessionGlobalIdx++
			routeID := uuid.New()
			sessionID := uuid.New()
			sessionCreated := now.Add(time.Duration(sessionIdx) * time.Millisecond)
			routeMetadata := jsonText(map[string]any{
				"benchmark":         benchmarkName,
				"benchmark_marker":  cfg.Seed.Marker,
				"conversation_name": fmt.Sprintf("Bench Conversation %d", sessionIdx),
			})
			sessionMetadata := jsonText(map[string]any{
				"benchmark":        benchmarkName,
				"benchmark_marker": cfg.Seed.Marker,
				"hot":              isHotSession(cfg.Seed, sessIdx),
			})
			if err := routeBatch.add(routeID, botID, "local", uniqueName("bench-conversation", cfg.Seed.Marker, sessionIdx), nil, "group", "", routeMetadata, sessionCreated, sessionCreated); err != nil {
				return SeedCatalog{}, err
			}
			if err := sessionBatch.add(sessionID, botID, routeID, "local", "chat", fmt.Sprintf("Bench Session %d", sessionIdx), sessionMetadata, userID, sessionCreated, sessionCreated); err != nil {
				return SeedCatalog{}, err
			}
			for _, batch := range []*copyBatcher{routeBatch, sessionBatch} {
				if err := batch.flush(); err != nil {
					return SeedCatalog{}, err
				}
			}

			sessionSeed := SessionSeed{
				BotID:       botID,
				OwnerUserID: userID,
				RouteID:     routeID,
				SessionID:   sessionID,
			}
			requestSeq := sessionRequestSeq{}
			turns := buildSessionTurns(cfg.Seed, sessionIdx, sessionCreated)
			requestByTurn := make(map[uuid.UUID]uuid.UUID, len(turns))
			assistantByTurn := make(map[uuid.UUID]uuid.UUID, len(turns))
			toolCallsByTurn := make(map[uuid.UUID][]pendingToolCallSeed)
			for turnIdx, turn := range turns {
				projectedTurnGlobalIdx := turnGlobalIdx + turnIdx + 1
				if cfg.Seed.ApprovalEveryNTurns > 0 && projectedTurnGlobalIdx%cfg.Seed.ApprovalEveryNTurns == 0 {
					toolCallID := uniqueName("tool-call", cfg.Seed.Marker, projectedTurnGlobalIdx)
					toolCallsByTurn[turn.id] = append(toolCallsByTurn[turn.id], pendingToolCallSeed{
						id:    toolCallID,
						name:  "write",
						input: jsonRaw(`{"path":"/tmp/bench.txt"}`),
					})
				}
				if cfg.Seed.UserInputEveryNTurns > 0 && projectedTurnGlobalIdx%cfg.Seed.UserInputEveryNTurns == 0 {
					toolCallID := uniqueName("ask-user", cfg.Seed.Marker, projectedTurnGlobalIdx)
					toolCallsByTurn[turn.id] = append(toolCallsByTurn[turn.id], pendingToolCallSeed{
						id:    toolCallID,
						name:  "ask_user",
						input: jsonRaw(`{"question":"bench?"}`),
					})
				}
			}
			for turnIdx, turn := range turns {
				var parent any
				if turn.parentID != uuid.Nil {
					parent = turn.parentID
				}
				if err := turnBatch.add(turn.id, botID, sessionID, parent, turn.createdAt, turn.createdAt); err != nil {
					return SeedCatalog{}, err
				}
				if turnIdx == cfg.Seed.TurnsPerSession-1 {
					sessionSeed.DefaultHeadTurnID = turn.id
				}
				// A mid-path (non-head) turn: the realistic input for the
				// head_resolve scenario, whose production caller only runs
				// after the cheap head-table lookup missed.
				if turnIdx == cfg.Seed.TurnsPerSession/2 {
					sessionSeed.MidPathTurnID = turn.id
				}
			}
			sessionSeed.PageTurnIDs = latestPageTurnIDs(cfg.Seed, turns)
			if err := turnBatch.flush(); err != nil {
				return SeedCatalog{}, err
			}

			heads := collectHeadTurns(cfg.Seed, turns)
			sessionSeed.HeadTurnIDs = heads
			for _, headID := range heads {
				if err := headBatch.add(sessionID, headID, botID, sessionCreated, sessionCreated); err != nil {
					return SeedCatalog{}, err
				}
			}
			if err := headBatch.flush(); err != nil {
				return SeedCatalog{}, err
			}
			if sessionSeed.DefaultHeadTurnID == uuid.Nil && len(heads) > 0 {
				sessionSeed.DefaultHeadTurnID = heads[0]
			}
			if _, err := tx.Exec(ctx, `UPDATE bot_sessions SET default_head_turn_id = $1, updated_at = created_at WHERE id = $2`, sessionSeed.DefaultHeadTurnID, sessionID); err != nil {
				return SeedCatalog{}, err
			}

			assetRows := make([]assetSeedRow, 0)
			cursorTargets := cursorTurnIndexes(len(turns))
			for turnIdx, turn := range turns {
				turnGlobalIdx++
				for seq := 1; seq <= cfg.Seed.MessagesPerTurn; seq++ {
					messageGlobalIdx++
					msgID := uuid.New()
					role := roleForSeq(seq)
					createdAt := turn.createdAt.Add(time.Duration(seq) * time.Microsecond)
					sourceMessageID := uniqueName("bench-msg", cfg.Seed.Marker, messageGlobalIdx)
					content := jsonText(map[string]any{
						"type":    "text",
						"content": fmt.Sprintf("bench %s session=%d turn=%d seq=%d", role, sessionIdx, turn.seq, seq),
					})
					display := fmt.Sprintf("bench %s %d/%d", role, turn.seq, seq)
					if role == "assistant" {
						content = assistantContentWithToolCalls(display, toolCallsByTurn[turn.id])
					}
					var senderIdentity any
					var senderUser any
					if role == "user" {
						senderIdentity = identityID
						senderUser = userID
					}
					if err := messageBatch.add(msgID, botID, sessionID, turn.id, int64(seq), senderIdentity, senderUser, sourceMessageID, nil, role, content, jsonRaw("{}"), jsonRaw("{}"), display, createdAt); err != nil {
						return SeedCatalog{}, err
					}
					if role == "user" && requestByTurn[turn.id] == uuid.Nil {
						requestByTurn[turn.id] = msgID
						if sessionSeed.ExternalMessageID == "" {
							sessionSeed.ExternalMessageID = sourceMessageID
						}
					}
					if role == "assistant" {
						assistantByTurn[turn.id] = msgID
					}
					sessionSeed.LatestMessageID = msgID
					if seq == 1 && cursorTargets[turnIdx] {
						sessionSeed.CursorMessageIDs = append(sessionSeed.CursorMessageIDs, msgID)
						sessionSeed.CursorCreatedAts = append(sessionSeed.CursorCreatedAts, createdAt)
					}
					if cfg.Seed.AssetEveryNMessages > 0 && role == "user" && messageGlobalIdx%cfg.Seed.AssetEveryNMessages == 0 {
						assetRows = append(assetRows, assetSeedRow{
							id:          uuid.New(),
							messageID:   msgID,
							role:        "attachment",
							ordinal:     0,
							contentHash: uniqueName("hash", cfg.Seed.Marker, messageGlobalIdx),
							name:        fmt.Sprintf("asset-%d.txt", messageGlobalIdx),
							metadata:    markerJSON,
							createdAt:   createdAt,
						})
					}
				}

				if cfg.Seed.ApprovalEveryNTurns > 0 && turnGlobalIdx%cfg.Seed.ApprovalEveryNTurns == 0 {
					approvalGlobalIdx++
					requestSeq.approval++
					shortID := requestSeq.approval
					status, decidedAt := requestStatus(cfg.Seed.PendingRatio, approvalGlobalIdx, "approved")
					requestID := uuid.New()
					promptExternalID := uniqueName("bench-approval-prompt", cfg.Seed.Marker, approvalGlobalIdx)
					if status == "pending" {
						sessionSeed.ApprovalRequestID = requestID
						if turnIdx < cfg.Seed.TurnsPerSession {
							sessionSeed.ApprovalBaseReqID = requestID
						}
						sessionSeed.ApprovalShortID = shortID
						sessionSeed.ApprovalPromptID = promptExternalID
					}
					values := approvalInsertValues(approvalShape, requestID, botID, sessionID, routeID, identityID, uniqueName("tool-call", cfg.Seed.Marker, turnGlobalIdx), "write", "write", jsonRaw(`{"path":"/tmp/bench.txt"}`), shortID, status, "", identityID, requestByTurn[turn.id], assistantByTurn[turn.id], turn.id, promptExternalID, "local", "", "group", turn.createdAt, timestampOrNil(decidedAt))
					if err := approvalBatch.add(values...); err != nil {
						return SeedCatalog{}, err
					}
				}
				if cfg.Seed.UserInputEveryNTurns > 0 && turnGlobalIdx%cfg.Seed.UserInputEveryNTurns == 0 {
					userInputGlobalIdx++
					requestSeq.userInput++
					shortID := requestSeq.userInput
					status, respondedAt := requestStatus(cfg.Seed.PendingRatio, userInputGlobalIdx, "submitted")
					requestID := uuid.New()
					promptExternalID := uniqueName("bench-user-input-prompt", cfg.Seed.Marker, userInputGlobalIdx)
					if status == "pending" {
						sessionSeed.UserInputRequestID = requestID
						if turnIdx < cfg.Seed.TurnsPerSession {
							sessionSeed.UserInputBaseReqID = requestID
						}
						sessionSeed.UserInputShortID = shortID
						sessionSeed.UserInputPromptID = promptExternalID
					}
					values := userInputInsertValues(userInputShape, requestID, botID, sessionID, routeID, identityID, uniqueName("ask-user", cfg.Seed.Marker, turnGlobalIdx), "ask_user", shortID, status, jsonRaw(`{"question":"bench?"}`), jsonRaw(`{"type":"text"}`), jsonRaw(`{}`), jsonRaw(`{}`), identityID, uuid.Nil, assistantByTurn[turn.id], uuid.Nil, uuid.Nil, turn.id, promptExternalID, "local", "", "group", nil, turn.createdAt, timestampOrNil(respondedAt), nil)
					if err := userInputBatch.add(values...); err != nil {
						return SeedCatalog{}, err
					}
				}
			}
			for _, batch := range []*copyBatcher{messageBatch, approvalBatch, userInputBatch} {
				if err := batch.flush(); err != nil {
					return SeedCatalog{}, err
				}
			}
			for _, row := range assetRows {
				if err := assetBatch.add(row.id, row.messageID, row.role, row.ordinal, row.contentHash, row.name, row.metadata, row.createdAt); err != nil {
					return SeedCatalog{}, err
				}
			}
			if err := assetBatch.flush(); err != nil {
				return SeedCatalog{}, err
			}
			if len(sessionSeed.CursorMessageIDs) == 0 {
				sessionSeed.CursorMessageIDs = append(sessionSeed.CursorMessageIDs, sessionSeed.LatestMessageID)
				sessionSeed.CursorCreatedAts = append(sessionSeed.CursorCreatedAts, sessionCreated)
			}
			catalog.Sessions = append(catalog.Sessions, sessionSeed)
			if isHotSession(cfg.Seed, sessIdx) {
				catalog.HotSessions = append(catalog.HotSessions, len(catalog.Sessions)-1)
			} else {
				catalog.ColdSessions = append(catalog.ColdSessions, len(catalog.Sessions)-1)
			}

			for _, turn := range turns {
				req := requestByTurn[turn.id]
				assistant := assistantByTurn[turn.id]
				if req == uuid.Nil && assistant == uuid.Nil {
					continue
				}
				update := turnPointerUpdate{
					turnID:      turn.id,
					requestID:   req,
					assistantID: assistant,
				}
				if _, err := tx.Exec(ctx, `UPDATE bot_history_turns SET request_message_id = $1, final_assistant_message_id = $2, updated_at = created_at WHERE id = $3`, nilUUID(update.requestID), nilUUID(update.assistantID), update.turnID); err != nil {
					return SeedCatalog{}, err
				}
			}
		}
	}

	for _, batch := range []*copyBatcher{
		userBatch,
		identityBatch,
		botBatch,
		routeBatch,
		sessionBatch,
		turnBatch,
		messageBatch,
		headBatch,
		approvalBatch,
		userInputBatch,
		assetBatch,
	} {
		if err := batch.flush(); err != nil {
			return SeedCatalog{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return SeedCatalog{}, err
	}
	if err := analyzeBenchmarkTables(ctx, pool); err != nil {
		return SeedCatalog{}, fmt.Errorf("analyze seeded tables: %w", err)
	}
	actual, err := actualSeedEstimate(ctx, pool, cfg.Seed.Marker)
	if err != nil {
		return SeedCatalog{}, fmt.Errorf("count seeded rows: %w", err)
	}
	catalog.Estimate = actual
	return catalog, nil
}

func assistantContentWithToolCalls(text string, calls []pendingToolCallSeed) json.RawMessage {
	parts := make([]map[string]any, 0, 1+len(calls))
	parts = append(parts, map[string]any{
		"type": "text",
		"text": text,
	})
	for _, call := range calls {
		if strings.TrimSpace(call.id) == "" {
			continue
		}
		input := any(nil)
		if len(call.input) > 0 {
			input = call.input
		}
		parts = append(parts, map[string]any{
			"type":       "tool-call",
			"toolCallId": call.id,
			"toolName":   call.name,
			"input":      input,
		})
	}
	return jsonText(map[string]any{
		"role":    "assistant",
		"content": parts,
	})
}

func loadSeedCatalog(ctx context.Context, pool *pgxpool.Pool, cfg Config) (SeedCatalog, error) {
	rows, err := pool.Query(ctx, `
	WITH benchmark_sessions AS (
	  SELECT s.*, b.owner_user_id
	  FROM bot_sessions s
	  JOIN bots b ON b.id = s.bot_id
	  WHERE b.metadata->>'benchmark_marker' = $1
	),
	ranked_messages AS (
	  SELECT
	    m.session_id,
	    m.id,
	    m.source_message_id,
	    m.created_at,
	    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at ASC, m.id ASC) AS rn,
	    COUNT(*) OVER (PARTITION BY m.session_id) AS cnt
	  FROM bot_history_messages m
	  JOIN benchmark_sessions s ON s.id = m.session_id
	)
	SELECT s.bot_id, s.owner_user_id, s.route_id, s.id, s.default_head_turn_id,
	       COALESCE(array_agg(h.head_turn_id ORDER BY h.created_at, h.head_turn_id) FILTER (WHERE h.head_turn_id IS NOT NULL), '{}') AS heads,
	       COALESCE((
	         SELECT m.id
	         FROM bot_history_messages m
	         WHERE m.session_id = s.id
	         ORDER BY m.created_at DESC, m.id DESC
	         LIMIT 1
	       ), '00000000-0000-0000-0000-000000000000'::uuid) AS latest_message_id,
	       COALESCE((
	         SELECT array_agg(rm.id ORDER BY rm.rn)
	         FROM ranked_messages rm
	         WHERE rm.session_id = s.id
	           AND rm.rn IN (1, GREATEST(rm.cnt / 2, 1), GREATEST(rm.cnt - 5, 1))
	       ), ARRAY[]::uuid[]) AS cursor_message_ids,
	       COALESCE((
	         SELECT array_agg(rm.created_at ORDER BY rm.rn)
	         FROM ranked_messages rm
	         WHERE rm.session_id = s.id
	           AND rm.rn IN (1, GREATEST(rm.cnt / 2, 1), GREATEST(rm.cnt - 5, 1))
	       ), ARRAY[]::timestamptz[]) AS cursor_created_ats,
	       COALESCE((
	         SELECT rm.source_message_id
	         FROM ranked_messages rm
	         WHERE rm.session_id = s.id
	           AND rm.source_message_id IS NOT NULL
	         ORDER BY rm.rn ASC
	         LIMIT 1
	       ), '') AS external_message_id,
	       COALESCE((
	         SELECT id FROM tool_approval_requests tar
	         WHERE tar.session_id = s.id AND tar.status = 'pending'
	         ORDER BY tar.created_at DESC, tar.short_id DESC LIMIT 1
	       ), '00000000-0000-0000-0000-000000000000'::uuid) AS approval_request_id,
	       COALESCE((
	         SELECT tar.id
	         FROM tool_approval_requests tar
	         JOIN bot_history_turns t ON t.id = tar.persist_turn_id
	         WHERE tar.session_id = s.id
	           AND tar.status = 'pending'
	           AND t.created_at <= (
	             SELECT created_at
	             FROM bot_history_turns head
	             WHERE head.id = s.default_head_turn_id
	           )
	         ORDER BY tar.created_at DESC, tar.short_id DESC LIMIT 1
	       ), '00000000-0000-0000-0000-000000000000'::uuid) AS approval_base_request_id,
	       COALESCE((
	         SELECT short_id FROM tool_approval_requests tar
	         WHERE tar.session_id = s.id AND tar.status = 'pending'
	         ORDER BY tar.created_at DESC, tar.short_id DESC LIMIT 1
	       ), 0) AS approval_short_id,
	       COALESCE((
	         SELECT prompt_external_message_id FROM tool_approval_requests tar
	         WHERE tar.session_id = s.id AND tar.status = 'pending'
	         ORDER BY tar.created_at DESC, tar.short_id DESC LIMIT 1
	       ), '') AS approval_prompt_external_id,
	       COALESCE((
	         SELECT id FROM user_input_requests uir
	         WHERE uir.session_id = s.id AND uir.status = 'pending'
	         ORDER BY uir.created_at DESC, uir.short_id DESC LIMIT 1
	       ), '00000000-0000-0000-0000-000000000000'::uuid) AS user_input_request_id,
	       COALESCE((
	         SELECT uir.id
	         FROM user_input_requests uir
	         JOIN bot_history_turns t ON t.id = uir.persist_turn_id
	         WHERE uir.session_id = s.id
	           AND uir.status = 'pending'
	           AND t.created_at <= (
	             SELECT created_at
	             FROM bot_history_turns head
	             WHERE head.id = s.default_head_turn_id
	           )
	         ORDER BY uir.created_at DESC, uir.short_id DESC LIMIT 1
	       ), '00000000-0000-0000-0000-000000000000'::uuid) AS user_input_base_request_id,
	       COALESCE((
	         SELECT short_id FROM user_input_requests uir
	         WHERE uir.session_id = s.id AND uir.status = 'pending'
	         ORDER BY uir.created_at DESC, uir.short_id DESC LIMIT 1
	       ), 0) AS user_input_short_id,
	       COALESCE((
	         SELECT prompt_external_message_id FROM user_input_requests uir
	         WHERE uir.session_id = s.id AND uir.status = 'pending'
	         ORDER BY uir.created_at DESC, uir.short_id DESC LIMIT 1
	       ), '') AS user_input_prompt_external_id,
	       COALESCE((
	         SELECT mt.id FROM bot_history_turns mt
	         WHERE mt.owner_session_id = s.id
	         ORDER BY mt.created_at ASC, mt.id ASC
	         OFFSET (SELECT COUNT(*) / 2 FROM bot_history_turns tc WHERE tc.owner_session_id = s.id)
	         LIMIT 1
	       ), '00000000-0000-0000-0000-000000000000'::uuid) AS mid_path_turn_id,
	       COALESCE((
	         WITH RECURSIVE default_path AS (
	           SELECT t.id, t.parent_turn_id, 1 AS depth
	           FROM bot_history_turns t
	           WHERE t.id = s.default_head_turn_id
	           UNION ALL
	           SELECT p.id, p.parent_turn_id, dp.depth + 1
	           FROM bot_history_turns p
	           JOIN default_path dp ON dp.parent_turn_id = p.id
	           WHERE dp.depth < 16
	         )
	         SELECT array_agg(dp.id) FROM default_path dp
	       ), ARRAY[]::uuid[]) AS page_turn_ids,
	       COALESCE((s.metadata->>'hot')::boolean, false) AS hot
	FROM benchmark_sessions s
	LEFT JOIN bot_session_turn_heads h ON h.session_id = s.id AND h.bot_id = s.bot_id
	GROUP BY s.id, s.bot_id, s.owner_user_id, s.route_id, s.default_head_turn_id, s.created_at, s.metadata
	ORDER BY s.created_at ASC, s.id ASC`, cfg.Seed.Marker)
	if err != nil {
		return SeedCatalog{}, err
	}
	defer rows.Close()
	catalog := SeedCatalog{Marker: cfg.Seed.Marker}
	botSeen := map[uuid.UUID]bool{}
	for rows.Next() {
		var s SessionSeed
		var hot bool
		if err := rows.Scan(&s.BotID, &s.OwnerUserID, &s.RouteID, &s.SessionID, &s.DefaultHeadTurnID, &s.HeadTurnIDs, &s.LatestMessageID, &s.CursorMessageIDs, &s.CursorCreatedAts, &s.ExternalMessageID, &s.ApprovalRequestID, &s.ApprovalBaseReqID, &s.ApprovalShortID, &s.ApprovalPromptID, &s.UserInputRequestID, &s.UserInputBaseReqID, &s.UserInputShortID, &s.UserInputPromptID, &s.MidPathTurnID, &s.PageTurnIDs, &hot); err != nil {
			return SeedCatalog{}, err
		}
		if !botSeen[s.BotID] {
			botSeen[s.BotID] = true
			catalog.BotIDs = append(catalog.BotIDs, s.BotID)
		}
		idx := len(catalog.Sessions)
		catalog.Sessions = append(catalog.Sessions, s)
		if hot {
			catalog.HotSessions = append(catalog.HotSessions, idx)
		} else {
			catalog.ColdSessions = append(catalog.ColdSessions, idx)
		}
	}
	if err := rows.Err(); err != nil {
		return SeedCatalog{}, err
	}
	if len(catalog.Sessions) == 0 {
		return SeedCatalog{}, fmt.Errorf("no benchmark sessions found for marker %q; run -mode seed or -mode seed-run first", cfg.Seed.Marker)
	}
	actual, err := actualSeedEstimate(ctx, pool, cfg.Seed.Marker)
	if err != nil {
		return SeedCatalog{}, fmt.Errorf("count benchmark rows: %w", err)
	}
	catalog.Estimate = actual
	return catalog, nil
}

func buildSessionTurns(seed SeedConfig, sessionIdx int, start time.Time) []turnSeed {
	branchHeads := branchHeadCount(seed)
	total := seed.TurnsPerSession + branchHeads*seed.BranchDepth
	turns := make([]turnSeed, 0, total)
	var parent uuid.UUID
	baseIDs := make([]uuid.UUID, 0, seed.TurnsPerSession)
	for i := 0; i < seed.TurnsPerSession; i++ {
		id := uuid.New()
		turns = append(turns, turnSeed{
			id:         id,
			parentID:   parent,
			createdAt:  start.Add(time.Duration(i) * time.Millisecond),
			sessionIdx: sessionIdx,
			seq:        i,
		})
		baseIDs = append(baseIDs, id)
		parent = id
	}
	if branchHeads == 0 || len(baseIDs) == 0 {
		return turns
	}
	baseStart := int(math.Max(0, float64(seed.TurnsPerSession-seed.BranchDepth-1)))
	for branch := 0; branch < branchHeads; branch++ {
		forkAt := baseStart - branch
		if forkAt < 0 {
			forkAt = 0
		}
		parent = baseIDs[forkAt]
		for depth := 0; depth < seed.BranchDepth; depth++ {
			id := uuid.New()
			seq := seed.TurnsPerSession + branch*seed.BranchDepth + depth
			turns = append(turns, turnSeed{
				id:         id,
				parentID:   parent,
				createdAt:  start.Add(time.Duration(seq) * time.Millisecond),
				sessionIdx: sessionIdx,
				seq:        seq,
			})
			parent = id
		}
	}
	return turns
}

// The number of default-path tail turns recorded per session for the
// turn_siblings scenario — a stand-in for the turn ids on one latest
// transcript page.
const variantPageTurnSeedCount = 16

// latestPageTurnIDs returns the tail of the base (default-head) path, the
// turn ids a latest transcript page would hand to ListSessionTurnSiblings.
func latestPageTurnIDs(seed SeedConfig, turns []turnSeed) []uuid.UUID {
	base := turns
	if len(base) > seed.TurnsPerSession {
		base = base[:seed.TurnsPerSession]
	}
	if len(base) > variantPageTurnSeedCount {
		base = base[len(base)-variantPageTurnSeedCount:]
	}
	ids := make([]uuid.UUID, 0, len(base))
	for _, turn := range base {
		ids = append(ids, turn.id)
	}
	return ids
}

func collectHeadTurns(seed SeedConfig, turns []turnSeed) []uuid.UUID {
	if len(turns) == 0 {
		return nil
	}
	heads := []uuid.UUID{turns[min(seed.TurnsPerSession, len(turns))-1].id}
	branchHeads := branchHeadCount(seed)
	for branch := 0; branch < branchHeads; branch++ {
		idx := seed.TurnsPerSession + (branch+1)*seed.BranchDepth - 1
		if idx >= 0 && idx < len(turns) {
			heads = append(heads, turns[idx].id)
		}
	}
	if len(heads) > seed.ActiveHeadsPerSess {
		heads = heads[:seed.ActiveHeadsPerSess]
	}
	return heads
}

func cursorTurnIndexes(turnCount int) map[int]bool {
	indexes := map[int]bool{}
	if turnCount <= 0 {
		return indexes
	}
	for _, idx := range []int{0, turnCount / 2, max(turnCount-3, 0)} {
		if idx >= 0 && idx < turnCount {
			indexes[idx] = true
		}
	}
	return indexes
}

func isHotSession(seed SeedConfig, sessionIndexWithinBot int) bool {
	hotCount := int(math.Ceil(float64(seed.SessionsPerBot) * seed.HotSessionRatio))
	if hotCount <= 0 {
		hotCount = 1
	}
	return sessionIndexWithinBot < hotCount
}

func roleForSeq(seq int) string {
	switch seq {
	case 1:
		return "user"
	case 2:
		return "assistant"
	default:
		return "tool"
	}
}

func requestStatus(pendingRatio float64, ordinal int, doneStatus string) (string, time.Time) {
	if pendingRatio >= 1 {
		return "pending", time.Time{}
	}
	if pendingRatio <= 0 {
		return doneStatus, time.Now().UTC()
	}
	period := int(math.Round(1 / pendingRatio))
	if period <= 1 || ordinal%period == 0 {
		return "pending", time.Time{}
	}
	return doneStatus, time.Now().UTC()
}

func jsonText(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}

func jsonRaw(raw string) json.RawMessage {
	return json.RawMessage(raw)
}

func nilUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func uniqueName(prefix, marker string, n int) string {
	cleanMarker := strings.ToLower(marker)
	cleanMarker = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, cleanMarker)
	cleanMarker = strings.Trim(cleanMarker, "-")
	if cleanMarker == "" {
		cleanMarker = "local"
	}
	return fmt.Sprintf("%s-%s-%d", prefix, cleanMarker, n)
}

func uniqueBotName(marker string, n int) string {
	name := uniqueName("bench-bot", marker, n)
	if len(name) > 63 {
		name = name[:63]
	}
	name = strings.TrimRight(name, "-")
	if len(name) < 2 {
		return fmt.Sprintf("bench-bot-%d", n)
	}
	return name
}

func uniqueEmail(marker string, n int) string {
	return fmt.Sprintf("%s@example.invalid", uniqueName("bench-user", marker, n))
}

func loadApprovalInsertShape(ctx context.Context, pool *pgxpool.Pool) (approvalInsertShape, error) {
	columns, err := tableColumns(ctx, pool, "tool_approval_requests")
	if err != nil {
		return approvalInsertShape{}, err
	}
	shape := approvalInsertShape{
		hasOperation:        columns["operation"],
		hasRequestedMessage: columns["requested_message_id"],
		hasPromptMessage:    columns["prompt_message_id"],
		hasPromptExternal:   columns["prompt_external_message_id"],
	}
	shape.columns = []string{
		"id",
		"bot_id",
		"session_id",
		"route_id",
		"channel_identity_id",
		"tool_call_id",
		"tool_name",
	}
	if shape.hasOperation {
		shape.columns = append(shape.columns, "operation")
	}
	shape.columns = append(shape.columns,
		"tool_input",
		"short_id",
		"status",
		"decision_reason",
		"requested_by_channel_identity_id",
	)
	if shape.hasRequestedMessage {
		shape.columns = append(shape.columns, "requested_message_id")
	}
	if shape.hasPromptMessage {
		shape.columns = append(shape.columns, "prompt_message_id")
	}
	shape.columns = append(shape.columns, "persist_turn_id")
	if shape.hasPromptExternal {
		shape.columns = append(shape.columns, "prompt_external_message_id")
	}
	shape.columns = append(shape.columns,
		"source_platform",
		"reply_target",
		"conversation_type",
		"created_at",
		"decided_at",
	)
	return shape, nil
}

func loadUserInputInsertShape(ctx context.Context, pool *pgxpool.Pool) (userInputInsertShape, error) {
	columns, err := tableColumns(ctx, pool, "user_input_requests")
	if err != nil {
		return userInputInsertShape{}, err
	}
	shape := userInputInsertShape{
		hasPromptMessage:  columns["prompt_message_id"],
		hasPromptExternal: columns["prompt_external_message_id"],
	}
	shape.columns = []string{
		"id",
		"bot_id",
		"session_id",
		"route_id",
		"channel_identity_id",
		"tool_call_id",
		"tool_name",
		"short_id",
		"status",
		"input_json",
		"ui_payload_json",
		"result_json",
		"provider_metadata",
		"requested_by_channel_identity_id",
		"responded_by_channel_identity_id",
		"assistant_message_id",
		"tool_result_message_id",
	}
	if shape.hasPromptMessage {
		shape.columns = append(shape.columns, "prompt_message_id")
	}
	shape.columns = append(shape.columns, "persist_turn_id")
	if shape.hasPromptExternal {
		shape.columns = append(shape.columns, "prompt_external_message_id")
	}
	shape.columns = append(shape.columns,
		"source_platform",
		"reply_target",
		"conversation_type",
		"expires_at",
		"created_at",
		"updated_at",
		"responded_at",
		"canceled_at",
	)
	return shape, nil
}

func tableColumns(ctx context.Context, pool *pgxpool.Pool, table string) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = $1`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s was not found in current schema", table)
	}
	return columns, nil
}

func approvalInsertValues(shape approvalInsertShape, id, botID, sessionID, routeID, identityID uuid.UUID, toolCallID, toolName, operation string, toolInput json.RawMessage, shortID int32, status, decisionReason string, requestedByID, requestedMessageID, promptMessageID, persistTurnID uuid.UUID, promptExternalID, sourcePlatform, replyTarget, conversationType string, createdAt time.Time, decidedAt any) []any {
	values := []any{
		id,
		botID,
		sessionID,
		routeID,
		identityID,
		toolCallID,
		toolName,
	}
	if shape.hasOperation {
		values = append(values, operation)
	}
	values = append(values,
		toolInput,
		shortID,
		status,
		decisionReason,
		requestedByID,
	)
	if shape.hasRequestedMessage {
		values = append(values, nilUUID(requestedMessageID))
	}
	if shape.hasPromptMessage {
		values = append(values, nilUUID(promptMessageID))
	}
	values = append(values, persistTurnID)
	if shape.hasPromptExternal {
		values = append(values, promptExternalID)
	}
	values = append(values,
		sourcePlatform,
		replyTarget,
		conversationType,
		createdAt,
		decidedAt,
	)
	return values
}

func userInputInsertValues(shape userInputInsertShape, id, botID, sessionID, routeID, identityID uuid.UUID, toolCallID, toolName string, shortID int32, status string, inputJSON, uiPayloadJSON, resultJSON, providerMetadata json.RawMessage, requestedByID, respondedByID, assistantMessageID, toolResultMessageID, promptMessageID, persistTurnID uuid.UUID, promptExternalID, sourcePlatform, replyTarget, conversationType string, expiresAt any, createdAt time.Time, respondedAt any, canceledAt any) []any {
	values := []any{
		id,
		botID,
		sessionID,
		routeID,
		identityID,
		toolCallID,
		toolName,
		shortID,
		status,
		inputJSON,
		uiPayloadJSON,
		resultJSON,
		providerMetadata,
		requestedByID,
		nilUUID(respondedByID),
		nilUUID(assistantMessageID),
		nilUUID(toolResultMessageID),
	}
	if shape.hasPromptMessage {
		values = append(values, nilUUID(promptMessageID))
	}
	values = append(values, persistTurnID)
	if shape.hasPromptExternal {
		values = append(values, promptExternalID)
	}
	values = append(values,
		sourcePlatform,
		replyTarget,
		conversationType,
		expiresAt,
		createdAt,
		createdAt,
		respondedAt,
		canceledAt,
	)
	return values
}

func actualSeedEstimate(ctx context.Context, pool *pgxpool.Pool, marker string) (SeedEstimate, error) {
	var estimate SeedEstimate
	err := pool.QueryRow(ctx, `
WITH bench_bots AS (
  SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1
)
SELECT
  (SELECT COUNT(*) FROM bench_bots),
  (SELECT COUNT(*) FROM bot_sessions WHERE bot_id IN (SELECT id FROM bench_bots)),
  (SELECT COUNT(*) FROM bot_history_turns WHERE bot_id IN (SELECT id FROM bench_bots)),
  (SELECT COUNT(*) FROM bot_history_messages WHERE bot_id IN (SELECT id FROM bench_bots)),
  (SELECT COUNT(*) FROM bot_session_turn_heads WHERE bot_id IN (SELECT id FROM bench_bots)),
  (SELECT COUNT(*) FROM tool_approval_requests WHERE bot_id IN (SELECT id FROM bench_bots)),
  (SELECT COUNT(*) FROM user_input_requests WHERE bot_id IN (SELECT id FROM bench_bots)),
  (SELECT COUNT(*)
   FROM bot_history_message_assets a
   JOIN bot_history_messages m ON m.id = a.message_id
   WHERE m.bot_id IN (SELECT id FROM bench_bots))`, marker).Scan(
		&estimate.Bots,
		&estimate.Sessions,
		&estimate.Turns,
		&estimate.Messages,
		&estimate.Heads,
		&estimate.Approvals,
		&estimate.UserInputs,
		&estimate.Assets,
	)
	if err != nil {
		return SeedEstimate{}, err
	}
	return estimate, nil
}

func benchmarkResidualRows(ctx context.Context, pool *pgxpool.Pool, marker string) (SeedEstimate, error) {
	return actualSeedEstimate(ctx, pool, marker)
}
