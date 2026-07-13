package workspace

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/userruntime"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const (
	remoteTestBotID     = "11111111-1111-4111-8111-111111111111"
	remoteTestRuntimeID = "22222222-2222-4222-8222-222222222222"
	remoteTestOwnerID   = "33333333-3333-4333-8333-333333333333"
)

type fakeRemoteBindingStore struct {
	record dbstore.BotRemoteRuntimeBindingRecord
	exists bool
}

func (s *fakeRemoteBindingStore) UpsertBotRemoteRuntimeBinding(_ context.Context, input dbstore.UpsertBotRemoteRuntimeBindingInput) (dbstore.BotRemoteRuntimeBindingRecord, error) {
	s.record.BotID = input.BotID
	s.record.RuntimeID = input.RuntimeID
	s.record.WorkspacePath = input.WorkspacePath
	if s.record.RuntimeName == "" {
		s.record.RuntimeName = "Office Mac"
	}
	if s.record.RuntimeUserID == "" {
		s.record.RuntimeUserID = remoteTestOwnerID
	}
	if s.record.BotOwnerUserID == "" {
		s.record.BotOwnerUserID = remoteTestOwnerID
	}
	s.exists = true
	return s.record, nil
}

func (s *fakeRemoteBindingStore) GetBotRemoteRuntimeBinding(context.Context, string) (dbstore.BotRemoteRuntimeBindingRecord, error) {
	if !s.exists {
		return dbstore.BotRemoteRuntimeBindingRecord{}, db.ErrNotFound
	}
	return s.record, nil
}

func (s *fakeRemoteBindingStore) DeleteBotRemoteRuntimeBinding(context.Context, string) error {
	if !s.exists {
		return db.ErrNotFound
	}
	s.exists = false
	return nil
}

type fakeRuntimeConnections map[string]*userruntime.Connection

func (f fakeRuntimeConnections) Connection(runtimeID string) (*userruntime.Connection, bool) {
	connection, ok := f[runtimeID]
	return connection, ok
}

type remoteScopeCaptureServer struct {
	pb.UnimplementedContainerServiceServer
	metadata chan metadata.MD
}

func (s *remoteScopeCaptureServer) Stat(ctx context.Context, _ *pb.StatRequest) (*pb.StatResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.metadata <- md
	return &pb.StatResponse{Entry: &pb.FileEntry{IsDir: true}}, nil
}

func TestRemoteWorkspaceBindingDefaultsToPerBotPath(t *testing.T) {
	store := &fakeRemoteBindingStore{}
	service := &RemoteWorkspaceService{store: store, runtimes: fakeRuntimeConnections{}}
	binding, err := service.Bind(context.Background(), remoteTestBotID, BindRemoteWorkspaceRequest{RuntimeID: remoteTestRuntimeID})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if binding.WorkspacePath != "bots/"+remoteTestBotID {
		t.Fatalf("workspace path = %q", binding.WorkspacePath)
	}
	if binding.Status != RemoteBindingStatusOffline || binding.Online {
		t.Fatalf("binding status = %q online=%v", binding.Status, binding.Online)
	}

	sharedPath := "projects/共享"
	binding, err = service.Bind(context.Background(), remoteTestBotID, BindRemoteWorkspaceRequest{
		RuntimeID: remoteTestRuntimeID, WorkspacePath: sharedPath,
	})
	if err != nil {
		t.Fatalf("Bind shared workspace: %v", err)
	}
	if binding.WorkspacePath != sharedPath {
		t.Fatalf("shared workspace path = %q, want %q", binding.WorkspacePath, sharedPath)
	}

	for _, invalid := range []string{"../escape", "/absolute", `windows\\path`, "a/../b", "a//b"} {
		if _, err := service.Bind(context.Background(), remoteTestBotID, BindRemoteWorkspaceRequest{
			RuntimeID: remoteTestRuntimeID, WorkspacePath: invalid,
		}); !errors.Is(err, ErrInvalidRemoteWorkspacePath) {
			t.Fatalf("Bind workspace_path %q error = %v", invalid, err)
		}
	}
}

func TestRemoteWorkspaceClientCarriesPersistentBotScope(t *testing.T) {
	rootClient, captured := newRemoteScopeTestClient(t)
	store := &fakeRemoteBindingStore{
		exists: true,
		record: dbstore.BotRemoteRuntimeBindingRecord{
			BotID: remoteTestBotID, RuntimeID: remoteTestRuntimeID,
			WorkspacePath: "projects/共享", RuntimeName: "Office Mac",
			RuntimeUserID: remoteTestOwnerID, BotOwnerUserID: remoteTestOwnerID,
		},
	}
	service := &RemoteWorkspaceService{
		store: store,
		runtimes: fakeRuntimeConnections{remoteTestRuntimeID: {
			RuntimeID: remoteTestRuntimeID,
			Client:    rootClient,
			Info: userruntime.RuntimeInfo{
				Capabilities: []string{userruntime.CapabilityFS, userruntime.CapabilityExec, userruntime.CapabilityWorkspaceScope},
			},
		}},
	}

	bound, err := service.EnsureReady(context.Background(), remoteTestBotID)
	if err != nil || !bound {
		t.Fatalf("EnsureReady bound=%v err=%v", bound, err)
	}
	md := <-captured
	if got := md.Get(RemoteWorkspaceIDMetadataKey); len(got) != 1 || got[0] != remoteTestBotID {
		t.Fatalf("workspace id metadata = %v", got)
	}
	if got := md.Get(RemoteWorkspacePathMetadataKey); len(got) != 1 || got[0] != "projects/共享" {
		t.Fatalf("workspace path metadata = %v", got)
	}
}

func TestRemoteWorkspaceInfoMatchesNativeLocalWorkDirSemantics(t *testing.T) {
	for _, tc := range []struct {
		name          string
		os            string
		workspaceBase string
		wantWorkDir   string
	}{
		{name: "macOS", os: "darwin", workspaceBase: "/Users/alice/workspaces", wantWorkDir: "/Users/alice/workspaces/projects/demo"},
		{name: "Windows", os: "win32", workspaceBase: `C:\Users\alice\workspaces`, wantWorkDir: `C:\Users\alice\workspaces\projects\demo`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeRemoteBindingStore{exists: true, record: dbstore.BotRemoteRuntimeBindingRecord{
				BotID: remoteTestBotID, RuntimeID: remoteTestRuntimeID, WorkspacePath: "projects/demo",
				RuntimeUserID: remoteTestOwnerID, BotOwnerUserID: remoteTestOwnerID,
			}}
			service := &RemoteWorkspaceService{
				store: store,
				runtimes: fakeRuntimeConnections{remoteTestRuntimeID: {
					RuntimeID: remoteTestRuntimeID,
					Info:      userruntime.RuntimeInfo{WorkspaceBase: tc.workspaceBase, OS: tc.os},
				}},
			}
			info, bound, err := service.WorkspaceInfo(context.Background(), remoteTestBotID)
			if err != nil || !bound {
				t.Fatalf("WorkspaceInfo bound=%v err=%v", bound, err)
			}
			if info.Backend != bridge.WorkspaceBackendRemote || info.OS != tc.os || info.DefaultWorkDir != tc.wantWorkDir {
				t.Fatalf("WorkspaceInfo = %#v", info)
			}
		})
	}
}

func TestRemoteWorkspaceInfoHidesPreviousOwnerPathAfterTransferOrRevoke(t *testing.T) {
	for name, mutate := range map[string]func(*dbstore.BotRemoteRuntimeBindingRecord){
		"owner mismatch": func(record *dbstore.BotRemoteRuntimeBindingRecord) {
			record.BotOwnerUserID = "44444444-4444-4444-8444-444444444444"
		},
		"revoked": func(record *dbstore.BotRemoteRuntimeBindingRecord) { record.RuntimeRevoked = true },
	} {
		t.Run(name, func(t *testing.T) {
			record := dbstore.BotRemoteRuntimeBindingRecord{
				BotID: remoteTestBotID, RuntimeID: remoteTestRuntimeID, WorkspacePath: "projects/demo",
				RuntimeUserID: remoteTestOwnerID, BotOwnerUserID: remoteTestOwnerID,
			}
			mutate(&record)
			// The previous owner's runtime stays connected: its workspace base
			// and OS must still not leak into prompt/tool workspace info.
			service := &RemoteWorkspaceService{
				store: &fakeRemoteBindingStore{record: record, exists: true},
				runtimes: fakeRuntimeConnections{remoteTestRuntimeID: {
					RuntimeID: remoteTestRuntimeID,
					Info:      userruntime.RuntimeInfo{WorkspaceBase: "/Users/alice/workspaces", OS: "darwin"},
				}},
			}
			info, bound, err := service.WorkspaceInfo(context.Background(), remoteTestBotID)
			if err != nil || !bound {
				t.Fatalf("WorkspaceInfo bound=%v err=%v", bound, err)
			}
			if info.Backend != bridge.WorkspaceBackendRemote || info.OS != "" || info.DefaultWorkDir != "/data" {
				t.Fatalf("WorkspaceInfo leaked previous owner details: %#v", info)
			}
		})
	}
}

func TestRemoteWorkspaceBoundFailuresNeverLookUnbound(t *testing.T) {
	base := dbstore.BotRemoteRuntimeBindingRecord{
		BotID: remoteTestBotID, RuntimeID: remoteTestRuntimeID, WorkspacePath: ".",
		RuntimeUserID: remoteTestOwnerID, BotOwnerUserID: remoteTestOwnerID,
	}
	for name, tc := range map[string]struct {
		mutate func(*dbstore.BotRemoteRuntimeBindingRecord)
		want   error
	}{
		"offline": {mutate: func(*dbstore.BotRemoteRuntimeBindingRecord) {}, want: ErrRemoteRuntimeOffline},
		"revoked": {mutate: func(record *dbstore.BotRemoteRuntimeBindingRecord) { record.RuntimeRevoked = true }, want: ErrRemoteRuntimeRevoked},
		"owner mismatch": {mutate: func(record *dbstore.BotRemoteRuntimeBindingRecord) {
			record.RuntimeUserID = "44444444-4444-4444-8444-444444444444"
		}, want: ErrRemoteRuntimeOwnerMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			tc.mutate(&record)
			service := &RemoteWorkspaceService{
				store:    &fakeRemoteBindingStore{record: record, exists: true},
				runtimes: fakeRuntimeConnections{},
			}
			_, bound, err := service.ClientForBot(context.Background(), remoteTestBotID)
			if !bound || !errors.Is(err, tc.want) {
				t.Fatalf("ClientForBot bound=%v err=%v, want %v", bound, err, tc.want)
			}
		})
	}
}

func TestManagerDoesNotFallBackWhenBoundRuntimeIsOffline(t *testing.T) {
	service := &RemoteWorkspaceService{
		store: &fakeRemoteBindingStore{
			exists: true,
			record: dbstore.BotRemoteRuntimeBindingRecord{
				BotID: remoteTestBotID, RuntimeID: remoteTestRuntimeID, WorkspacePath: ".",
				RuntimeUserID: remoteTestOwnerID, BotOwnerUserID: remoteTestOwnerID,
			},
		},
		runtimes: fakeRuntimeConnections{},
	}
	manager := NewManager(slog.Default(), nil, nil, config.WorkspaceConfig{}, "", nil)
	manager.SetRemoteWorkspaceService(service)

	if _, err := manager.MCPClient(context.Background(), remoteTestBotID); !errors.Is(err, ErrRemoteRuntimeOffline) {
		t.Fatalf("MCPClient error = %v, want offline", err)
	}
	info, err := manager.WorkspaceInfo(context.Background(), remoteTestBotID)
	if err != nil {
		t.Fatalf("WorkspaceInfo: %v", err)
	}
	if info.Backend != bridge.WorkspaceBackendRemote || info.DefaultWorkDir != "/data" {
		t.Fatalf("WorkspaceInfo = %#v", info)
	}
}

func newRemoteScopeTestClient(t *testing.T) (*bridge.Client, <-chan metadata.MD) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	captured := make(chan metadata.MD, 1)
	server := grpc.NewServer()
	pb.RegisterContainerServiceServer(server, &remoteScopeCaptureServer{metadata: captured})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///remote-scope-test",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bridge.NewClientFromConn(conn), captured
}
