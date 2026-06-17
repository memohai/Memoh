package whatsapp

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/memohai/memoh/internal/channel"
)

func TestFinalizePairMovesStoreAndEnablesConfig(t *testing.T) {
	root := t.TempDir()
	store := &fakeWhatsAppConfigStore{}
	lifecycle := &fakeWhatsAppLifecycle{store: store}
	manager := &fakeWhatsAppStopper{}
	svc := NewService(nil, root, store, lifecycle, manager, nil)
	svc.now = func() time.Time { return time.Unix(456, 0).UTC() }
	writePendingWhatsAppStore(t, root, "login-1", "12345678900")

	cfg, err := svc.finalizePair(context.Background(), &pendingLogin{
		ID:    "login-1",
		BotID: "bot-1",
	})
	if err != nil {
		t.Fatalf("finalizePair: %v", err)
	}
	if cfg.Disabled {
		t.Fatal("final config was not enabled")
	}
	if store.current.Credentials["storeId"] != "cfg-1" {
		t.Fatalf("credentials = %#v", store.current.Credentials)
	}
	if store.current.ExternalIdentity != "12345678900@s.whatsapp.net" {
		t.Fatalf("external identity = %q", store.current.ExternalIdentity)
	}
	if got := store.current.SelfIdentity["phone"]; got != "12345678900" {
		t.Fatalf("phone = %#v", got)
	}
	if got := store.current.SelfIdentity["push_name"]; got != "Test User" {
		t.Fatalf("push name = %#v", got)
	}
	if got := store.current.SelfIdentity["business_name"]; got != "Test Biz" {
		t.Fatalf("business name = %#v", got)
	}
	if store.current.VerifiedAt != time.Unix(456, 0).UTC() {
		t.Fatalf("verified_at = %v", store.current.VerifiedAt)
	}
	if _, err := os.Stat(finalStorePaths(root, "cfg-1").DB); err != nil {
		t.Fatalf("final store missing: %v", err)
	}
	if _, err := os.Stat(pendingStorePaths(root, "login-1").Dir); !os.IsNotExist(err) {
		t.Fatalf("pending store still exists, stat err=%v", err)
	}
	if lifecycle.calls != 1 || lifecycle.disabled {
		t.Fatalf("lifecycle calls=%d disabled=%v", lifecycle.calls, lifecycle.disabled)
	}
	if len(manager.stopped) != 1 || manager.stopped[0] != "cfg-1" {
		t.Fatalf("manager stopped = %#v", manager.stopped)
	}
}

func TestFinalizePairKeepsSessionWhenEnableFails(t *testing.T) {
	root := t.TempDir()
	store := &fakeWhatsAppConfigStore{}
	lifecycle := &fakeWhatsAppLifecycle{
		store: store,
		err:   errors.New("enable failed"),
	}
	svc := NewService(nil, root, store, lifecycle, &fakeWhatsAppStopper{}, nil)
	writePendingWhatsAppStore(t, root, "login-1", "12345678900")

	_, err := svc.finalizePair(context.Background(), &pendingLogin{
		ID:    "login-1",
		BotID: "bot-1",
	})
	if err == nil {
		t.Fatal("finalizePair should return enable failure")
	}
	if !store.exists {
		t.Fatal("final config was rolled back after enable failure")
	}
	if !store.current.Disabled {
		t.Fatal("final config should remain disabled when enable fails")
	}
	if store.current.Credentials["storeId"] != "cfg-1" {
		t.Fatalf("credentials = %#v", store.current.Credentials)
	}
	if _, err := os.Stat(finalStorePaths(root, "cfg-1").DB); err != nil {
		t.Fatalf("final store missing after enable failure: %v", err)
	}
	if _, err := os.Stat(pendingStorePaths(root, "login-1").Dir); !os.IsNotExist(err) {
		t.Fatalf("pending store still exists after enable failure, stat err=%v", err)
	}
}

func TestFinalizePairRestoresConfigWhenStoreMoveFails(t *testing.T) {
	previous := channel.ChannelConfig{
		ID:               "cfg-1",
		BotID:            "bot-1",
		ChannelType:      Type,
		Credentials:      map[string]any{"storeId": "cfg-1"},
		ExternalIdentity: "old@s.whatsapp.net",
		SelfIdentity:     map[string]any{"phone": "old"},
		Routing:          map[string]any{"mode": "private"},
		VerifiedAt:       time.Unix(123, 0).UTC(),
	}
	root := t.TempDir()
	store := &fakeWhatsAppConfigStore{current: previous, exists: true}
	svc := NewService(nil, root, store, nil, &fakeWhatsAppStopper{}, nil)
	writePendingWhatsAppStore(t, root, "login-1", "12345678900")

	oldReplace := replaceWhatsAppStore
	replaceWhatsAppStore = func(_ storePaths, _ storePaths, _ string) (*storeReplacement, error) {
		return nil, errors.New("move failed")
	}
	t.Cleanup(func() { replaceWhatsAppStore = oldReplace })

	_, err := svc.finalizePair(context.Background(), &pendingLogin{
		ID:    "login-1",
		BotID: "bot-1",
	})
	if err == nil {
		t.Fatal("finalizePair should return move failure")
	}
	if !store.exists {
		t.Fatal("previous config was deleted after move failure")
	}
	if store.current.ExternalIdentity != previous.ExternalIdentity {
		t.Fatalf("external identity after rollback = %q", store.current.ExternalIdentity)
	}
	if store.current.Disabled {
		t.Fatal("previous config was left disabled after rollback")
	}
	if got := store.current.Credentials["storeId"]; got != "cfg-1" {
		t.Fatalf("credentials after rollback = %#v", store.current.Credentials)
	}
	if _, err := os.Stat(pendingStorePaths(root, "login-1").DB); err != nil {
		t.Fatalf("pending store should remain after move failure: %v", err)
	}
	if _, err := os.Stat(finalStorePaths(root, "cfg-1").Dir); !os.IsNotExist(err) {
		t.Fatalf("final store should not exist after move failure, stat err=%v", err)
	}
}

func TestFinalizePairRollsBackPlaceholderConfigOnStoreFailure(t *testing.T) {
	previous := channel.ChannelConfig{
		ID:               "cfg-1",
		BotID:            "bot-1",
		ChannelType:      Type,
		Credentials:      map[string]any{"storeId": "cfg-1"},
		ExternalIdentity: "12345678900@s.whatsapp.net",
		SelfIdentity:     map[string]any{"phone": "12345678900"},
		Routing:          map[string]any{"mode": "private"},
		VerifiedAt:       time.Unix(123, 0).UTC(),
	}
	store := &fakeWhatsAppConfigStore{current: previous, exists: true}
	svc := NewService(nil, t.TempDir(), store, nil, nil, nil)
	svc.idSource = func() string { return "login-1" }

	_, err := svc.finalizePair(context.Background(), &pendingLogin{
		ID:    "missing-pending-store",
		BotID: "bot-1",
	})
	if err == nil {
		t.Fatal("finalizePair should fail when pending store is missing")
	}
	if !store.exists {
		t.Fatal("previous config was deleted")
	}
	if got := store.current.Credentials["storeId"]; got != "cfg-1" {
		t.Fatalf("credentials after rollback = %#v", store.current.Credentials)
	}
	if store.current.Disabled {
		t.Fatal("previous config was left disabled")
	}
	if store.current.ExternalIdentity != previous.ExternalIdentity {
		t.Fatalf("external identity after rollback = %q", store.current.ExternalIdentity)
	}
}

func TestFinalizePairDeletesPlaceholderConfigOnStoreFailureWithoutPreviousConfig(t *testing.T) {
	store := &fakeWhatsAppConfigStore{}
	svc := NewService(nil, t.TempDir(), store, nil, nil, nil)

	_, err := svc.finalizePair(context.Background(), &pendingLogin{
		ID:    "missing-pending-store",
		BotID: "bot-1",
	})
	if err == nil {
		t.Fatal("finalizePair should fail when pending store is missing")
	}
	if store.exists {
		t.Fatalf("placeholder config was not deleted: %+v", store.current)
	}
}

func TestCancelLoginDoesNotRemovePairedPendingStore(t *testing.T) {
	root := t.TempDir()
	paths := pendingStorePaths(root, "login-1")
	if err := ensureStoreDir(paths.Dir); err != nil {
		t.Fatalf("ensure pending store: %v", err)
	}
	if err := os.WriteFile(paths.DB, []byte("paired"), 0o600); err != nil {
		t.Fatalf("write pending store: %v", err)
	}
	svc := NewService(nil, root, nil, nil, nil, nil)
	svc.pending["login-1"] = &pendingLogin{
		ID:      "login-1",
		BotID:   "bot-1",
		Mode:    pendingModeQR,
		Status:  "success",
		paired:  true,
		cfgID:   "cfg-1",
		Timeout: time.Second,
	}

	resp, err := svc.CancelLogin(context.Background(), "bot-1", "login-1")
	if err != nil {
		t.Fatalf("cancel paired login: %v", err)
	}
	if resp.Status != "paired" {
		t.Fatalf("status = %q, want paired", resp.Status)
	}
	if _, err := os.Stat(paths.DB); err != nil {
		t.Fatalf("paired pending store was removed: %v", err)
	}
}

type fakeWhatsAppConfigStore struct {
	current channel.ChannelConfig
	exists  bool
}

func (s *fakeWhatsAppConfigStore) ResolveEffectiveConfig(_ context.Context, botID string, channelType channel.ChannelType) (channel.ChannelConfig, error) {
	if !s.exists {
		return channel.ChannelConfig{}, channel.ErrChannelConfigNotFound
	}
	out := s.current
	out.BotID = botID
	out.ChannelType = channelType
	if out.ID == "" {
		out.ID = "cfg-1"
	}
	return out, nil
}

func (s *fakeWhatsAppConfigStore) UpsertConfig(_ context.Context, botID string, channelType channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error) {
	if botID == "" {
		return channel.ChannelConfig{}, errors.New("bot id is required")
	}
	disabled := false
	if req.Disabled != nil {
		disabled = *req.Disabled
	}
	verifiedAt := time.Time{}
	if req.VerifiedAt != nil {
		verifiedAt = *req.VerifiedAt
	}
	cfg := channel.ChannelConfig{
		ID:               "cfg-1",
		BotID:            botID,
		ChannelType:      channelType,
		Credentials:      req.Credentials,
		ExternalIdentity: req.ExternalIdentity,
		SelfIdentity:     req.SelfIdentity,
		Routing:          req.Routing,
		Disabled:         disabled,
		VerifiedAt:       verifiedAt,
	}
	s.current = cfg
	s.exists = true
	return cfg, nil
}

func (s *fakeWhatsAppConfigStore) DeleteConfig(_ context.Context, _ string, _ channel.ChannelType) error {
	s.current = channel.ChannelConfig{}
	s.exists = false
	return nil
}

type fakeWhatsAppLifecycle struct {
	store    *fakeWhatsAppConfigStore
	err      error
	calls    int
	disabled bool
}

func (l *fakeWhatsAppLifecycle) SetBotChannelStatus(_ context.Context, _ string, _ channel.ChannelType, disabled bool) (channel.ChannelConfig, error) {
	l.calls++
	l.disabled = disabled
	if l.err != nil {
		return channel.ChannelConfig{}, l.err
	}
	if l.store == nil {
		return channel.ChannelConfig{Disabled: disabled}, nil
	}
	l.store.current.Disabled = disabled
	return l.store.current, nil
}

type fakeWhatsAppStopper struct {
	stopped []string
}

func (s *fakeWhatsAppStopper) Stop(_ context.Context, configID string) error {
	s.stopped = append(s.stopped, configID)
	return nil
}

func writePendingWhatsAppStore(t *testing.T, root, loginID, user string) {
	t.Helper()
	ctx := context.Background()
	paths := pendingStorePaths(root, loginID)
	container, client, err := openClientStore(ctx, paths, waLog.Noop)
	if err != nil {
		t.Fatalf("open pending store: %v", err)
	}
	defer func() { _ = container.Close() }()
	jid := types.NewJID(user, types.DefaultUserServer)
	client.Store.ID = &jid
	client.Store.Account = &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte("details"),
		AccountSignatureKey: make([]byte, 32),
		AccountSignature:    make([]byte, 64),
		DeviceSignature:     make([]byte, 64),
	}
	client.Store.PushName = "Test User"
	client.Store.BusinessName = "Test Biz"
	if err := client.Store.Save(ctx); err != nil {
		t.Fatalf("save pending store identity: %v", err)
	}
	secureStoreFiles(paths)
}
