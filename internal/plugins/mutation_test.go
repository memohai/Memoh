package plugins

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/mcp"
)

const mutationTestBotID = "11111111-1111-4111-8111-111111111111"

type mutationTestQueries struct {
	dbstore.Queries
}

func TestWithBotMutationSerializesSameBot(t *testing.T) {
	service := NewService(nil, &mutationTestQueries{}, nil, nil, nil, BridgeProvider{})
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- service.WithBotMutation(context.Background(), mutationTestBotID, func(context.Context) error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- service.WithBotMutation(context.Background(), mutationTestBotID, func(context.Context) error {
			close(secondEntered)
			return nil
		})
	}()

	enteredEarly := false
	select {
	case <-secondEntered:
		enteredEarly = true
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first mutation: %v", err)
	}
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second mutation did not enter after the first mutation released the bot lock")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second mutation: %v", err)
	}
	if enteredEarly {
		t.Fatal("second mutation entered before the first mutation released the bot lock")
	}
}

func TestWithBotMutationReusesNestedScope(t *testing.T) {
	service := NewService(nil, &mutationTestQueries{}, nil, nil, nil, BridgeProvider{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := service.WithBotMutation(ctx, mutationTestBotID, func(scopedCtx context.Context) error {
		return service.WithBotMutation(scopedCtx, mutationTestBotID, func(context.Context) error {
			return nil
		})
	})
	if err != nil {
		t.Fatalf("nested mutation: %v", err)
	}
}

type mutationLockQueries struct {
	dbstore.Queries
	locked    dbstore.Queries
	lockCalls int
}

func (q *mutationLockQueries) WithBotMutationLock(
	_ context.Context,
	_ pgtype.UUID,
	fn func(dbstore.Queries) error,
) error {
	q.lockCalls++
	return fn(q.locked)
}

type mutationTransactionQueries struct {
	dbstore.Queries
	tx         dbstore.Queries
	txCalls    int
	txErr      error
	rolledBack bool
}

func (q *mutationTransactionQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.txCalls++
	q.txErr = fn(q.tx)
	q.rolledBack = q.txErr != nil
	return q.txErr
}

type failingPluginQueries struct {
	dbstore.Queries
	installationID pgtype.UUID
	mcpCreateCalls int
}

func (q *failingPluginQueries) CreateBotPluginInstallation(
	_ context.Context,
	arg sqlc.CreateBotPluginInstallationParams,
) (sqlc.BotPluginInstallation, error) {
	return sqlc.BotPluginInstallation{
		ID:         q.installationID,
		BotID:      arg.BotID,
		PluginID:   arg.PluginID,
		PluginName: arg.PluginName,
		Version:    arg.Version,
		Status:     arg.Status,
		Enabled:    arg.Enabled,
		Config:     arg.Config,
		Metadata:   arg.Metadata,
		Manifest:   arg.Manifest,
	}, nil
}

func (*failingPluginQueries) ListBotPluginResources(context.Context, pgtype.UUID) ([]sqlc.BotPluginResource, error) {
	return nil, nil
}

func (*failingPluginQueries) DeleteMCPConnectionsByPlugin(context.Context, sqlc.DeleteMCPConnectionsByPluginParams) error {
	return nil
}

func (*failingPluginQueries) DeleteBotPluginResources(context.Context, pgtype.UUID) error {
	return nil
}

func (q *failingPluginQueries) CreateManagedMCPConnection(
	context.Context,
	sqlc.CreateManagedMCPConnectionParams,
) (sqlc.McpConnection, error) {
	q.mcpCreateCalls++
	return sqlc.McpConnection{}, errors.New("injected MCP write failure")
}

func TestInstallRollsBackPluginAndMCPWritesTogether(t *testing.T) {
	botID := pgtype.UUID{Bytes: [16]byte{1, 1, 1, 1, 1, 1, 0x41, 1, 0x81, 1, 1, 1, 1, 1, 1, 1}, Valid: true}
	installationID := pgtype.UUID{Bytes: [16]byte{2, 2, 2, 2, 2, 2, 0x42, 2, 0x82, 2, 2, 2, 2, 2, 2, 2}, Valid: true}
	txQueries := &failingPluginQueries{installationID: installationID}
	lockedQueries := &mutationTransactionQueries{tx: txQueries}
	rootQueries := &mutationLockQueries{locked: lockedQueries}
	service := NewService(
		nil,
		rootQueries,
		mcp.NewConnectionService(nil, rootQueries),
		mcp.NewOAuthService(nil, rootQueries, ""),
		nil,
		BridgeProvider{},
	)

	_, err := service.Install(context.Background(), botID.String(), InstallRequest{Manifest: Manifest{
		ID:   "failing-plugin",
		Name: "Failing Plugin",
		MCPs: []MCPResource{{
			Key:       "api",
			Name:      "API",
			Transport: "http",
			URL:       "https://example.com/mcp",
		}},
	}})
	if err == nil || !errors.Is(err, lockedQueries.txErr) {
		t.Fatalf("Install() error = %v, transaction error = %v", err, lockedQueries.txErr)
	}
	if rootQueries.lockCalls != 1 {
		t.Fatalf("bot lock calls = %d, want 1", rootQueries.lockCalls)
	}
	if lockedQueries.txCalls != 1 {
		t.Fatalf("transaction calls = %d, want 1", lockedQueries.txCalls)
	}
	if !lockedQueries.rolledBack {
		t.Fatal("transaction did not roll back after the MCP write failed")
	}
	if txQueries.mcpCreateCalls != 1 {
		t.Fatalf("transaction-bound MCP creates = %d, want 1", txQueries.mcpCreateCalls)
	}
}

func TestWithBotMutationAllowsDifferentBotsInParallel(t *testing.T) {
	service := NewService(nil, &mutationTestQueries{}, nil, nil, nil, BridgeProvider{})
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = service.WithBotMutation(context.Background(), mutationTestBotID, func(context.Context) error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	otherBotID := "22222222-2222-4222-8222-222222222222"
	if err := service.WithBotMutation(context.Background(), otherBotID, func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("different bot mutation: %v", err)
	}
	close(releaseFirst)
	wg.Wait()
}
