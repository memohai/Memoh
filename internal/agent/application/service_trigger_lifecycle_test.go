package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	contextfrag "github.com/memohai/memoh/internal/agent/context/fragment"
	sessionruntime "github.com/memohai/memoh/internal/agent/runtime/session"
	"github.com/memohai/memoh/internal/heartbeat"
)

func triggerDirectHeartbeat(t *testing.T, service *Service) (heartbeat.TriggerResult, error) {
	t.Helper()
	return service.TriggerHeartbeat(context.Background(), lifecycleTestBotID, heartbeat.TriggerPayload{
		BotID:           lifecycleTestBotID,
		SessionID:       lifecycleTestSessionID,
		Interval:        30,
		LastHeartbeatAt: directLifecyclePrompt,
	}, "")
}

func TestTriggerHeartbeatResolveFailurePersistsAdmittedMinimalLifecycle(t *testing.T) {
	fixture := newDirectLifecycleFixture(t, directLifecycleModelSuccess)
	fixture.service.settingsService = nil

	_, err := triggerDirectHeartbeat(t, fixture.service)
	if err == nil {
		t.Fatal("TriggerHeartbeat() error = nil, want model-resolution failure")
	}
	creates := fixture.lifecycles.creates()
	if len(creates) != 1 {
		t.Fatalf("lifecycle creates = %d, want 1", len(creates))
	}
	row := creates[0]
	if pgUUIDString(row.RunID) != lifecycleTestRunID || row.Status != contextLifecycleStatusFailedProvider {
		t.Fatalf("lifecycle terminal = (run %q, status %q), want admitted run %q failed_provider", pgUUIDString(row.RunID), row.Status, lifecycleTestRunID)
	}
	var snapshot contextfrag.LifecycleSnapshot
	if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		t.Fatalf("decode minimal lifecycle: %v", err)
	}
	if snapshot.Version != 1 || snapshot.View != "" || snapshot.Counts != (contextfrag.ManifestCounts{}) ||
		snapshot.AssistantMessageID != "" {
		t.Fatalf("pre-context lifecycle is not minimal: %#v", snapshot)
	}
	if len(fixture.messages.persisted) != 0 {
		t.Fatalf("resolve failure persisted messages: %#v", fixture.messages.persisted)
	}
	if len(fixture.runtime.finishes) != 1 || fixture.runtime.finishes[0].status != sessionruntime.RunStatusErrored {
		t.Fatalf("runtime finishes = %#v, want one errored finish", fixture.runtime.finishes)
	}
}

func TestTriggerHeartbeatProviderFailurePersistsFailedProviderLifecycle(t *testing.T) {
	fixture := newDirectLifecycleFixture(t, directLifecycleModelFailure)

	_, err := triggerDirectHeartbeat(t, fixture.service)
	if err == nil {
		t.Fatal("TriggerHeartbeat() error = nil, want provider failure")
	}
	assertDirectLifecycle(t, fixture.lifecycles, lifecycleTestRunID, contextLifecycleStatusFailedProvider, "")
	if len(fixture.runtime.finishes) != 1 || fixture.runtime.finishes[0].status != sessionruntime.RunStatusErrored {
		t.Fatalf("runtime finishes = %#v, want one errored finish", fixture.runtime.finishes)
	}
	if fixture.lifecycles.creates()[0].ErrorCode.Valid {
		t.Fatalf("private provider diagnostic became stable error code: %#v", fixture.lifecycles.creates()[0].ErrorCode)
	}
}

func TestTriggerHeartbeatLifecycleStoreFailureDoesNotFailSuccessfulTrigger(t *testing.T) {
	fixture := newDirectLifecycleFixture(t, directLifecycleModelSuccess)
	fixture.lifecycles.mu.Lock()
	fixture.lifecycles.store.createErr = errors.New("context lifecycle store unavailable")
	fixture.lifecycles.mu.Unlock()

	result, err := triggerDirectHeartbeat(t, fixture.service)
	if err != nil {
		t.Fatalf("TriggerHeartbeat() error = %v, want nil despite lifecycle store failure", err)
	}
	if result.Status != "ok" {
		t.Fatalf("TriggerHeartbeat() status = %q, want ok", result.Status)
	}
	if got := fixture.service.contextLifecyclePersistenceErrors.Load(); got == 0 {
		t.Fatal("lifecycle persistence error count = 0, want at least one recorded failure")
	}
	if len(fixture.runtime.finishes) != 1 || fixture.runtime.finishes[0].status != sessionruntime.RunStatusCompleted {
		t.Fatalf("runtime finishes = %#v, want one completed finish", fixture.runtime.finishes)
	}
}
