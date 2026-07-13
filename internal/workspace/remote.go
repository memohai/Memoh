package workspace

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/db"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/userruntime"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	RemoteWorkspaceIDMetadataKey   = "x-memoh-workspace-id"
	RemoteWorkspacePathMetadataKey = "x-memoh-workspace-path-bin"

	RemoteBindingStatusOnline               = "online"
	RemoteBindingStatusOffline              = "offline"
	RemoteBindingStatusRevoked              = "revoked"
	RemoteBindingStatusOwnerMismatch        = "owner_mismatch"
	RemoteBindingStatusClientUpdateRequired = "client_update_required"
)

var (
	ErrRemoteWorkspaceNotBound         = errors.New("remote workspace is not bound")
	ErrWorkspaceNotServerManaged       = errors.New("workspace lifecycle is managed by a remote runtime")
	ErrRemoteRuntimeNotUsable          = errors.New("remote runtime not found, revoked, or owned by another user")
	ErrRemoteRuntimeOffline            = errors.New("remote runtime is offline")
	ErrRemoteRuntimeRevoked            = errors.New("remote runtime has been revoked")
	ErrRemoteRuntimeOwnerMismatch      = errors.New("remote runtime no longer belongs to the bot owner")
	ErrRemoteRuntimeClientUpdateNeeded = errors.New("remote runtime client must be updated")
	ErrInvalidRemoteWorkspacePath      = errors.New("invalid remote workspace path")
)

type RemoteWorkspaceBinding struct {
	BotID         string    `json:"bot_id"`
	RuntimeID     string    `json:"runtime_id,omitempty"`
	RuntimeName   string    `json:"runtime_name,omitempty"`
	WorkspacePath string    `json:"workspace_path,omitempty"`
	Status        string    `json:"status"`
	Online        bool      `json:"online"`
	Hostname      string    `json:"hostname,omitempty"`
	OS            string    `json:"os,omitempty"`
	Arch          string    `json:"arch,omitempty"`
	Capabilities  []string  `json:"capabilities,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type BindRemoteWorkspaceRequest struct {
	RuntimeID     string `json:"runtime_id" validate:"required"`
	WorkspacePath string `json:"workspace_path,omitempty"`
}

// RemoteWorkspaceService resolves the persistent Bot-to-device binding while
// userruntime.Service owns only device credentials and live connections.
type RemoteWorkspaceService struct {
	store    dbstore.BotRemoteRuntimeBindingStore
	runtimes runtimeConnectionResolver
}

type runtimeConnectionResolver interface {
	Connection(runtimeID string) (*userruntime.Connection, bool)
}

func NewRemoteWorkspaceService(store dbstore.BotRemoteRuntimeBindingStore, runtimes *userruntime.Service) *RemoteWorkspaceService {
	return &RemoteWorkspaceService{store: store, runtimes: runtimes}
}

func (s *RemoteWorkspaceService) Bind(ctx context.Context, botID string, req BindRemoteWorkspaceRequest) (RemoteWorkspaceBinding, error) {
	if s == nil || s.store == nil {
		return RemoteWorkspaceBinding{}, errors.New("remote workspace service not configured")
	}
	botID, ok := canonicalWorkspaceUUID(botID)
	if !ok {
		return RemoteWorkspaceBinding{}, userruntime.ErrInvalidInput
	}
	runtimeID, ok := canonicalWorkspaceUUID(req.RuntimeID)
	if !ok {
		return RemoteWorkspaceBinding{}, userruntime.ErrInvalidInput
	}
	workspacePath, err := normalizeRemoteWorkspacePath(req.WorkspacePath, botID)
	if err != nil {
		return RemoteWorkspaceBinding{}, err
	}
	record, err := s.store.UpsertBotRemoteRuntimeBinding(ctx, dbstore.UpsertBotRemoteRuntimeBindingInput{
		BotID: botID, RuntimeID: runtimeID, WorkspacePath: workspacePath,
	})
	if errors.Is(err, db.ErrNotFound) {
		// The guarded INSERT ... SELECT matched no usable runtime row: the
		// caller supplied a bad runtime, not a missing binding.
		return RemoteWorkspaceBinding{}, ErrRemoteRuntimeNotUsable
	}
	if err != nil {
		return RemoteWorkspaceBinding{}, err
	}
	return s.binding(record), nil
}

func (s *RemoteWorkspaceService) Get(ctx context.Context, botID string) (RemoteWorkspaceBinding, error) {
	record, bound, err := s.record(ctx, botID)
	if err != nil {
		return RemoteWorkspaceBinding{}, err
	}
	if !bound {
		return RemoteWorkspaceBinding{}, ErrRemoteWorkspaceNotBound
	}
	return s.binding(record), nil
}

func (s *RemoteWorkspaceService) Unbind(ctx context.Context, botID string) error {
	if s == nil || s.store == nil {
		return errors.New("remote workspace service not configured")
	}
	botID, ok := canonicalWorkspaceUUID(botID)
	if !ok {
		return ErrRemoteWorkspaceNotBound
	}
	return s.store.DeleteBotRemoteRuntimeBinding(ctx, botID)
}

// ClientForBot returns whether a binding exists separately from the client
// error. Callers must never fall back to Local/Container when bound is true.
func (s *RemoteWorkspaceService) ClientForBot(ctx context.Context, botID string) (*bridge.Client, bool, error) {
	record, bound, err := s.record(ctx, botID)
	if err != nil || !bound {
		return nil, bound, err
	}
	if record.RuntimeUserID != record.BotOwnerUserID {
		return nil, true, ErrRemoteRuntimeOwnerMismatch
	}
	if record.RuntimeRevoked {
		return nil, true, ErrRemoteRuntimeRevoked
	}
	if s.runtimes == nil {
		return nil, true, ErrRemoteRuntimeOffline
	}
	connection, ok := s.runtimes.Connection(record.RuntimeID)
	if !ok || connection == nil || connection.Client == nil {
		return nil, true, ErrRemoteRuntimeOffline
	}
	if !supportsRemoteWorkspace(connection.Info.Capabilities) {
		return nil, true, ErrRemoteRuntimeClientUpdateNeeded
	}
	client := connection.Client.WithOutgoingMetadata(map[string]string{
		RemoteWorkspaceIDMetadataKey:   record.BotID,
		RemoteWorkspacePathMetadataKey: record.WorkspacePath,
	})
	if client == nil {
		return nil, true, ErrRemoteRuntimeOffline
	}
	return client, true, nil
}

func (s *RemoteWorkspaceService) WorkspaceInfo(ctx context.Context, botID string) (bridge.WorkspaceInfo, bool, error) {
	record, bound, err := s.record(ctx, botID)
	if err != nil || !bound {
		return bridge.WorkspaceInfo{}, bound, err
	}
	defaultWorkDir := "/data"
	runtimeOS := ""
	// Same gates as ClientForBot: a transferred or revoked binding must not
	// reveal the previous owner's workspace base or OS through prompt/tool
	// metadata, so keep the neutral defaults instead of consulting the live
	// connection.
	ownerMatches := record.RuntimeUserID == record.BotOwnerUserID
	if ownerMatches && !record.RuntimeRevoked && s.runtimes != nil {
		if connection, online := s.runtimes.Connection(record.RuntimeID); online && connection != nil {
			defaultWorkDir = remoteWorkspaceWorkDir(connection.Info, record.WorkspacePath)
			runtimeOS = connection.Info.OS
		}
	}
	return bridge.WorkspaceInfo{
		Backend:        bridge.WorkspaceBackendRemote,
		OS:             runtimeOS,
		DefaultWorkDir: defaultWorkDir,
	}, true, nil
}

func (s *RemoteWorkspaceService) EnsureReady(ctx context.Context, botID string) (bool, error) {
	client, bound, err := s.ClientForBot(ctx, botID)
	if err != nil || !bound {
		return bound, err
	}
	entry, err := client.Stat(ctx, "/")
	if err != nil {
		return true, fmt.Errorf("check remote workspace: %w", err)
	}
	if entry == nil || !entry.GetIsDir() {
		return true, errors.New("remote workspace root is not a directory")
	}
	return true, nil
}

func (s *RemoteWorkspaceService) record(ctx context.Context, botID string) (dbstore.BotRemoteRuntimeBindingRecord, bool, error) {
	if s == nil || s.store == nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, false, nil
	}
	botID, ok := canonicalWorkspaceUUID(botID)
	if !ok {
		return dbstore.BotRemoteRuntimeBindingRecord{}, false, nil
	}
	record, err := s.store.GetBotRemoteRuntimeBinding(ctx, botID)
	if errors.Is(err, db.ErrNotFound) {
		return dbstore.BotRemoteRuntimeBindingRecord{}, false, nil
	}
	if err != nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, false, err
	}
	return record, true, nil
}

func (s *RemoteWorkspaceService) binding(record dbstore.BotRemoteRuntimeBindingRecord) RemoteWorkspaceBinding {
	binding := RemoteWorkspaceBinding{
		BotID: record.BotID, RuntimeID: record.RuntimeID, RuntimeName: record.RuntimeName,
		WorkspacePath: record.WorkspacePath, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	switch {
	case record.RuntimeUserID != record.BotOwnerUserID:
		// A transferred Bot must not reveal the previous owner's device name,
		// runtime ID, or path. Keep only enough state for the new owner to see
		// that an invalid binding exists and remove it.
		binding.RuntimeID = ""
		binding.RuntimeName = ""
		binding.WorkspacePath = ""
		binding.Status = RemoteBindingStatusOwnerMismatch
	case record.RuntimeRevoked:
		binding.Status = RemoteBindingStatusRevoked
	default:
		if s.runtimes == nil {
			binding.Status = RemoteBindingStatusOffline
			break
		}
		connection, online := s.runtimes.Connection(record.RuntimeID)
		binding.Online = online
		if !online || connection == nil {
			binding.Status = RemoteBindingStatusOffline
			break
		}
		binding.Hostname = connection.Info.Hostname
		binding.OS = connection.Info.OS
		binding.Arch = connection.Info.Arch
		binding.Capabilities = append([]string(nil), connection.Info.Capabilities...)
		if !supportsRemoteWorkspace(connection.Info.Capabilities) {
			binding.Status = RemoteBindingStatusClientUpdateRequired
		} else {
			binding.Status = RemoteBindingStatusOnline
		}
	}
	return binding
}

func normalizeRemoteWorkspacePath(raw, botID string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = path.Join("bots", botID)
	}
	if len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || strings.Contains(value, `\`) {
		return "", ErrInvalidRemoteWorkspacePath
	}
	if value == "." {
		return value, nil
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.Contains(value, "//") {
		return "", ErrInvalidRemoteWorkspacePath
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", ErrInvalidRemoteWorkspacePath
		}
	}
	return value, nil
}

func remoteWorkspaceWorkDir(info userruntime.RuntimeInfo, workspacePath string) string {
	base := strings.TrimSpace(info.WorkspaceBase)
	if base == "" || workspacePath == "." {
		if base == "" {
			return "/data"
		}
		return base
	}
	if strings.EqualFold(info.OS, "win32") {
		return strings.TrimRight(base, `/\`) + `\` + strings.ReplaceAll(workspacePath, "/", `\`)
	}
	return strings.TrimRight(base, "/") + "/" + workspacePath
}

func canonicalWorkspaceUUID(value string) (string, bool) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", false
	}
	return id.String(), true
}

func supportsRemoteWorkspace(capabilities []string) bool {
	return slices.Contains(capabilities, userruntime.CapabilityFS) &&
		slices.Contains(capabilities, userruntime.CapabilityExec) &&
		slices.Contains(capabilities, userruntime.CapabilityWorkspaceScope)
}
