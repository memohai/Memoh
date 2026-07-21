package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type subagentConfigQueries struct {
	dbstore.Queries
	inTx          bool
	sessionCreate bool
	configCreate  bool
	failConfig    bool
}

func (q *subagentConfigQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.inTx = true
	err := fn(q)
	if err != nil {
		q.sessionCreate = false
		q.configCreate = false
	}
	return err
}

func (q *subagentConfigQueries) CreateSession(_ context.Context, arg sqlc.CreateSessionParams) (sqlc.BotSession, error) {
	q.sessionCreate = true
	now := pgtype.Timestamptz{Time: time.Unix(1, 0).UTC(), Valid: true}
	return sqlc.BotSession{
		ID:              mustSessionUUID("00000000-0000-0000-0000-000000000501"),
		BotID:           arg.BotID,
		Type:            arg.Type,
		SessionMode:     arg.SessionMode,
		RuntimeType:     arg.RuntimeType,
		RuntimeMetadata: arg.RuntimeMetadata,
		Title:           arg.Title,
		Metadata:        arg.Metadata,
		ParentSessionID: arg.ParentSessionID,
		CreatedByUserID: arg.CreatedByUserID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func (q *subagentConfigQueries) CreateSubagentConfig(_ context.Context, arg sqlc.CreateSubagentConfigParams) (sqlc.SubagentConfig, error) {
	if q.failConfig {
		return sqlc.SubagentConfig{}, errors.New("config insert failed")
	}
	q.configCreate = true
	now := pgtype.Timestamptz{Time: time.Unix(1, 0).UTC(), Valid: true}
	return sqlc.SubagentConfig{
		SessionID:      arg.SessionID,
		ModelUuid:      arg.ModelUuid,
		ModelID:        arg.ModelID,
		ProviderName:   arg.ProviderName,
		Forked:         arg.Forked,
		ParentMessages: arg.ParentMessages,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func mustSessionUUID(raw string) pgtype.UUID {
	id, err := dbpkg.ParseUUID(raw)
	if err != nil {
		panic(err)
	}
	return id
}

func TestCreateSubagentPersistsSessionAndConfigInOneTransaction(t *testing.T) {
	queries := &subagentConfigQueries{}
	service := NewService(nil, queries, nil)
	session, config, err := service.CreateSubagent(context.Background(), CreateSubagentInput{
		Session: CreateInput{
			BotID:           "00000000-0000-0000-0000-000000000502",
			ParentSessionID: "00000000-0000-0000-0000-000000000503",
			CreatedByUserID: "00000000-0000-0000-0000-000000000504",
			Title:           "worker",
			Metadata:        map[string]any{"agent_id": "worker"},
		},
		ModelUUID:      "00000000-0000-0000-0000-000000000505",
		ModelID:        "worker-model",
		ProviderName:   "provider-a",
		Forked:         true,
		ParentMessages: []byte(`[{"role":"user","content":[{"type":"text","text":"hello"}]}]`),
	})
	if err != nil {
		t.Fatalf("CreateSubagent: %v", err)
	}
	if !queries.inTx || !queries.sessionCreate || !queries.configCreate {
		t.Fatalf("expected transactional session+config creation: %+v", queries)
	}
	if session.Type != TypeSubagent || config.ModelID != "worker-model" || config.ProviderName != "provider-a" || !config.Forked {
		t.Fatalf("unexpected created values: session=%+v config=%+v", session, config)
	}
}

func TestCreateSubagentRollsBackSessionWhenConfigInsertFails(t *testing.T) {
	queries := &subagentConfigQueries{failConfig: true}
	service := NewService(nil, queries, nil)
	_, _, err := service.CreateSubagent(context.Background(), CreateSubagentInput{
		Session: CreateInput{
			BotID:           "00000000-0000-0000-0000-000000000502",
			ParentSessionID: "00000000-0000-0000-0000-000000000503",
		},
		ModelUUID:    "00000000-0000-0000-0000-000000000505",
		ModelID:      "worker-model",
		ProviderName: "provider-a",
	})
	if err == nil || !strings.Contains(err.Error(), "config insert failed") {
		t.Fatalf("expected config insert error, got %v", err)
	}
	if queries.sessionCreate || queries.configCreate {
		t.Fatalf("expected transaction rollback, got %+v", queries)
	}
}
