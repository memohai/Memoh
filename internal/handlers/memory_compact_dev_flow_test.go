package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/adapters/builtin"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type memoryCompactDevQueries struct {
	dbstore.Queries
	bot sqlc.GetBotByIDRow
}

func (q *memoryCompactDevQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (*memoryCompactDevQueries) GetContainerByBotID(context.Context, pgtype.UUID) (sqlc.Container, error) {
	return sqlc.Container{}, pgx.ErrNoRows
}

func TestChatCompactDevFlowCompactsAndArchivesFileMemory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	botID := "11111111-1111-1111-1111-111111111111"
	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	dataRoot, err := os.MkdirTemp("", "mc-*")
	if err != nil {
		t.Fatalf("create short temp data root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dataRoot) })
	startSkillsTestBridgeServer(t, dataRoot, botID)

	pool := bridge.NewPool(func(id string) string {
		return "unix://" + filepath.Join(dataRoot, "run", id, "bridge.sock")
	})
	t.Cleanup(pool.CloseAll)

	runtime := NewBuiltinMemoryRuntime(pool)
	provider := builtin.NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	llm := &fakeCompactLLM{facts: []string{
		"Ran likes tea, especially green and oolong tea.",
		"Ran works in Berlin and uses Vim.",
	}}
	provider.SetLLM(llm)

	filters := buildNamespaceFilters(sharedMemoryNamespace, botID, nil)
	seedMemories := []struct {
		memory   string
		metadata map[string]any
	}{
		{memory: "Pinned preference: Ran always wants calendar reminders.", metadata: map[string]any{"pinned": true}},
		{memory: "Ran likes green tea.", metadata: nil},
		{memory: "Ran likes oolong tea.", metadata: nil},
		{memory: "Ran works in Berlin.", metadata: nil},
		{memory: "Ran uses Vim for editing.", metadata: nil},
	}
	for _, seed := range seedMemories {
		if _, err := provider.Add(ctx, memprovider.AddRequest{
			Message:  seed.memory,
			BotID:    botID,
			Metadata: seed.metadata,
			Filters:  filters,
		}); err != nil {
			t.Fatalf("seed memory %q: %v", seed.memory, err)
		}
	}

	registry := memprovider.NewRegistry(slog.Default())
	registry.Register(defaultBuiltinProviderID, provider)

	botRow := testBotRow(botID, map[string]any{})
	botRow.OwnerUserID = testUUID(userID)
	botRow.Status = bots.BotStatusReady
	queries := &memoryCompactDevQueries{bot: botRow}
	handler := NewMemoryHandler(
		slog.Default(),
		bots.NewService(slog.Default(), queries),
		accounts.NewService(slog.Default(), testAdminAccountStore{role: "admin"}),
	)
	handler.SetMemoryRegistry(registry)

	body := bytes.NewBufferString(`{"ratio":0.5,"decay_days":14}`)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/bots/"+botID+"/memory/compact", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	echoCtx := testAuthContext(e, req, rec, userID)
	echoCtx.SetPath("/bots/:bot_id/memory/compact")
	echoCtx.SetParamNames("bot_id")
	echoCtx.SetParamValues(botID)

	if err := handler.ChatCompact(echoCtx); err != nil {
		t.Fatalf("ChatCompact returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("ChatCompact status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var result memprovider.CompactResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode compact response: %v", err)
	}
	if result.BeforeCount != 5 || result.AfterCount != 3 {
		t.Fatalf("compact counts before=%d after=%d, want before=5 after=3", result.BeforeCount, result.AfterCount)
	}
	if len(llm.reqs) != 1 {
		t.Fatalf("expected one LLM compact request, got %d", len(llm.reqs))
	}
	if got := len(llm.reqs[0].Memories); got != 4 {
		t.Fatalf("LLM received %d compactable memories, want 4", got)
	}
	if llm.reqs[0].TargetCount != 2 {
		t.Fatalf("LLM target_count = %d, want 2", llm.reqs[0].TargetCount)
	}
	if llm.reqs[0].DecayDays != 14 {
		t.Fatalf("LLM decay_days = %d, want 14", llm.reqs[0].DecayDays)
	}

	active, err := provider.GetAll(ctx, memprovider.GetAllRequest{BotID: botID, Filters: filters})
	if err != nil {
		t.Fatalf("GetAll after compact: %v", err)
	}
	activeText := joinMemoryText(active.Results)
	for _, want := range []string{
		"Pinned preference: Ran always wants calendar reminders.",
		"Ran likes tea, especially green and oolong tea.",
		"Ran works in Berlin and uses Vim.",
	} {
		if !strings.Contains(activeText, want) {
			t.Fatalf("active memories missing %q:\n%s", want, activeText)
		}
	}
	for _, oldFact := range []string{
		"Ran likes green tea.",
		"Ran likes oolong tea.",
		"Ran works in Berlin.",
		"Ran uses Vim for editing.",
	} {
		if strings.Contains(activeText, oldFact) {
			t.Fatalf("active memories still contain compacted source fact %q:\n%s", oldFact, activeText)
		}
	}

	archiveDir := filepath.Join(dataRoot, "data", "memory_archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read memory archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive file count = %d, want 1", len(entries))
	}
	archiveBytes, err := os.ReadFile(filepath.Join(archiveDir, entries[0].Name())) // #nosec G304 -- test reads an archive file created under its temporary data root.
	if err != nil {
		t.Fatalf("read archive file: %v", err)
	}
	archiveText := string(archiveBytes)
	for _, want := range []string{
		"Ran likes green tea.",
		"Ran likes oolong tea.",
		"Ran works in Berlin.",
		"Ran uses Vim for editing.",
		"superseded_by:",
	} {
		if !strings.Contains(archiveText, want) {
			t.Fatalf("archive missing %q:\n%s", want, archiveText)
		}
	}
	if strings.Contains(archiveText, "Pinned preference: Ran always wants calendar reminders.") {
		t.Fatalf("archive should not include pinned memory:\n%s", archiveText)
	}
}

func joinMemoryText(items []memprovider.MemoryItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Memory)
	}
	return strings.Join(parts, "\n")
}
