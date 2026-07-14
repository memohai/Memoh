package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/registry"
	"github.com/memohai/memoh/internal/sessionruntime"
)

var coreRuntimePrefixSequence atomic.Uint64

var errSessionRuntimeStartup = errors.New("session runtime startup failed")

type failingSessionRuntimeBackend struct {
	sessionruntime.DistributedBackend
	closed atomic.Bool
}

func (*failingSessionRuntimeBackend) SubscribeCommands(context.Context, string) (sessionruntime.CommandSubscription, error) {
	return sessionruntime.CommandSubscription{}, errSessionRuntimeStartup
}

func (b *failingSessionRuntimeBackend) Close() error {
	b.closed.Store(true)
	return nil
}

func uniqueCoreRuntimePrefix(scope string) string {
	return fmt.Sprintf("memoh:test:runtime-lifecycle:%s:%d:%d:", scope, time.Now().UnixNano(), coreRuntimePrefixSequence.Add(1))
}

func TestSessionRuntimeLocalLifecycleOutlivesFXStartupContext(t *testing.T) {
	backend := sessionruntime.NewMemoryBackend()
	owner := sessionruntime.NewManager(backend, sessionruntime.Options{
		OwnerID:       "lifecycle-owner",
		StateTTL:      time.Hour,
		OwnerLeaseTTL: time.Second,
		CommandAckTTL: 100 * time.Millisecond,
	})
	hook := sessionRuntimeLifecycleHook(owner)
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	if err := hook.OnStart(startupCtx); err != nil {
		t.Fatalf("start owner manager: %v", err)
	}
	cancelStartup()
	defer func() { _ = hook.OnStop(context.Background()) }()

	injectCh := make(chan conversation.InjectMessage, 1)
	if err := owner.StartRun(context.Background(), "bot-lifecycle", "session-lifecycle", "stream-lifecycle", make(chan struct{}, 1), func() {}, injectCh); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	if _, err := owner.Steer(context.Background(), "bot-lifecycle", "session-lifecycle", "stream-lifecycle", "continue after startup"); err != nil {
		t.Fatalf("steer after startup: %v", err)
	}
	select {
	case injected := <-injectCh:
		if injected.Text != "continue after startup" {
			t.Fatalf("injected text = %q", injected.Text)
		}
		if injected.Applied != nil {
			injected.Applied()
		}
	case <-time.After(time.Second):
		t.Fatal("runtime command subscription stopped with FX startup context")
	}
}

func TestSessionRuntimeLifecycleClosesBackendWhenStartupFails(t *testing.T) {
	t.Parallel()
	backend := &failingSessionRuntimeBackend{}
	manager := sessionruntime.NewManager(backend, sessionruntime.Options{OwnerID: "failing-lifecycle-owner"})
	err := sessionRuntimeLifecycleHook(manager).OnStart(context.Background())
	if !errors.Is(err, errSessionRuntimeStartup) {
		t.Fatalf("startup error = %v, want %v", err, errSessionRuntimeStartup)
	}
	if !backend.closed.Load() {
		t.Fatal("backend was not closed after startup failure")
	}
}

func TestSessionRuntimeDistributedLifecycleOutlivesFXStartupContext(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_DISTRIBUTED_REQUIRED") == "1" {
			t.Fatal("distributed lifecycle contract is required, but neither MEMOH_TEST_REDIS_URL nor MEMOH_TEST_VALKEY_URL is set")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run distributed lifecycle contract")
	}
	prefix := uniqueCoreRuntimePrefix(strings.ReplaceAll(t.Name(), "/", ":"))
	ownerBackend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute})
	if err != nil {
		t.Fatalf("create owner backend: %v", err)
	}
	remoteBackend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute})
	if err != nil {
		_ = ownerBackend.Close()
		t.Fatalf("create remote backend: %v", err)
	}
	owner := sessionruntime.NewManager(ownerBackend, sessionruntime.Options{OwnerID: "lifecycle-owner", StateTTL: time.Hour, OwnerLeaseTTL: time.Second, CommandAckTTL: 100 * time.Millisecond})
	remote := sessionruntime.NewManager(remoteBackend, sessionruntime.Options{OwnerID: "lifecycle-remote", StateTTL: time.Hour, OwnerLeaseTTL: time.Second, CommandAckTTL: 100 * time.Millisecond})
	hook := sessionRuntimeLifecycleHook(owner)
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	if err := hook.OnStart(startupCtx); err != nil {
		t.Fatalf("start owner manager: %v", err)
	}
	cancelStartup()
	if err := remote.Start(context.Background()); err != nil {
		t.Fatalf("start remote manager: %v", err)
	}
	defer func() {
		_ = hook.OnStop(context.Background())
		_ = remote.Close()
	}()

	injectCh := make(chan conversation.InjectMessage, 1)
	if err := owner.StartRun(context.Background(), "bot-lifecycle", "session-lifecycle", "stream-lifecycle", make(chan struct{}, 1), func() {}, injectCh); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	if _, err := remote.Steer(context.Background(), "bot-lifecycle", "session-lifecycle", "stream-lifecycle", "continue after startup"); err != nil {
		t.Fatalf("remote steer: %v", err)
	}
	select {
	case injected := <-injectCh:
		if injected.Text != "continue after startup" {
			t.Fatalf("injected text = %q", injected.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime command subscription stopped with FX startup context")
	}
}

func TestRegistryProviderTemplatesKeepAllProviderFiles(t *testing.T) {
	defs := []registry.ProviderDefinition{
		{
			Name:       "DeepSeek",
			ClientType: "openai-completions",
		},
		{
			Name:       "OpenAI",
			ClientType: "openai-responses",
		},
		{
			Name:       "OpenAI Speech",
			ClientType: "openai-speech",
		},
		{
			Name:       "Google Transcription",
			ClientType: "google-transcription",
		},
	}

	got := registry.ProviderTemplateDefinitions(defs)
	if len(got) != len(defs) {
		t.Fatalf("definition count = %d, want %d", len(got), len(defs))
	}
	for i := range defs {
		if got[i].Name != defs[i].Name {
			t.Fatalf("definition %d = %#v, want %#v", i, got[i], defs[i])
		}
	}
}
