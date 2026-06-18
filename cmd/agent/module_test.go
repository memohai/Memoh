package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/fx"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	modelspkg "github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

func TestFXOptionsValidate(t *testing.T) {
	if err := fx.ValidateApp(options()); err != nil {
		t.Fatal(err)
	}
}

func TestACPToolProvidersIncludeAskUser(t *testing.T) {
	providers := acpToolProviders([]agenttools.ToolProvider{
		agenttools.NewAskUserProvider(slog.Default()),
		agenttools.NewSkillProvider(slog.Default()),
	})

	foundAskUser := false
	for _, provider := range providers {
		if _, ok := provider.(*agenttools.AskUserProvider); ok {
			foundAskUser = true
		}
	}
	if !foundAskUser {
		t.Fatal("ask_user should be exposed to ACP")
	}
	if len(providers) != 2 {
		t.Fatalf("filtered providers = %d, want 2", len(providers))
	}
}

type lazyLLMTestQueries struct {
	dbstore.Queries
	botID             string
	compactionModel   pgtype.UUID
	providerID        pgtype.UUID
	settingsLookups   int
	fallbackLookups   int
	configuredLookups int
}

func (q *lazyLLMTestQueries) GetSettingsByBotID(_ context.Context, id pgtype.UUID) (sqlc.GetSettingsByBotIDRow, error) {
	q.settingsLookups++
	if id.String() != q.botID {
		return sqlc.GetSettingsByBotIDRow{}, errors.New("unexpected bot id")
	}
	return sqlc.GetSettingsByBotIDRow{
		BotID:             id,
		CompactionModelID: q.compactionModel,
	}, nil
}

func (*lazyLLMTestQueries) ListModelsByModelID(_ context.Context, _ string) ([]sqlc.Model, error) {
	return nil, nil
}

func (q *lazyLLMTestQueries) GetModelByID(_ context.Context, id pgtype.UUID) (sqlc.Model, error) {
	q.configuredLookups++
	if id.String() != q.compactionModel.String() {
		return sqlc.Model{}, errors.New("unexpected model id")
	}
	return sqlc.Model{
		ID:         id,
		ModelID:    "compact-model",
		ProviderID: q.providerID,
		Type:       string(modelspkg.ModelTypeChat),
		Enable:     true,
	}, nil
}

func (q *lazyLLMTestQueries) ListEnabledModelsByType(context.Context, string) ([]sqlc.Model, error) {
	q.fallbackLookups++
	return nil, errors.New("fallback model lookup should not be used")
}

func (q *lazyLLMTestQueries) GetProviderByID(_ context.Context, id pgtype.UUID) (sqlc.Provider, error) {
	if id.String() != q.providerID.String() {
		return sqlc.Provider{}, errors.New("unexpected provider id")
	}
	return sqlc.Provider{
		ID:         id,
		Name:       "test-provider",
		ClientType: string(modelspkg.ClientTypeOpenAIResponses),
		Enable:     true,
		Config:     []byte(`{"base_url":"http://127.0.0.1","api_key":"test"}`),
	}, nil
}

func TestLazyLLMCompactResolvesModelWithRequestBotID(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &lazyLLMTestQueries{
		botID:           botID,
		compactionModel: mustTestUUID("22222222-2222-2222-2222-222222222222"),
		providerID:      mustTestUUID("33333333-3333-3333-3333-333333333333"),
	}
	client := &lazyLLMClient{
		modelsService:   modelspkg.NewService(slog.Default(), queries),
		settingsService: settings.NewService(slog.Default(), queries, nil, nil),
		queries:         queries,
		timeout:         time.Second,
		logger:          slog.Default(),
	}

	if _, err := client.Compact(context.Background(), memprovider.CompactRequest{BotID: botID}); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if queries.settingsLookups != 1 {
		t.Fatalf("settings lookups = %d, want 1", queries.settingsLookups)
	}
	if queries.configuredLookups == 0 {
		t.Fatal("configured bot model was not resolved")
	}
	if queries.fallbackLookups != 0 {
		t.Fatalf("fallback lookups = %d, want 0", queries.fallbackLookups)
	}
}

func mustTestUUID(s string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		panic(err)
	}
	return id
}
