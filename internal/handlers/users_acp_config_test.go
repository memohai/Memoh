package handlers

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

func TestPrepareACPWorkspaceConfigWritesCodexAPIKeyConfig(t *testing.T) {
	client, recorder := newUsersACPConfigBridgeClient(t)
	handler := &UsersHandler{
		acpWorkspace: &usersACPConfigWorkspace{
			backend: bridge.WorkspaceBackendContainer,
			client:  client,
		},
	}

	err := handler.prepareACPWorkspaceConfig(context.Background(), bots.Bot{
		ID: "bot-1",
		Metadata: map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{
						"enabled":    true,
						"setup_mode": "api_key",
						"managed": map[string]any{
							"api_key":  "sk-secret",
							"base_url": "https://proxy.example.com/v1",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("prepareACPWorkspaceConfig() error = %v", err)
	}

	writes := recorder.writes()
	if len(writes) != 2 {
		t.Fatalf("writes len = %d, want config.toml + auth.json: %#v", len(writes), writes)
	}
	configWrite, ok := findUsersACPConfigWrite(writes, acpclient.CodexManagedConfigDir+"/config.toml")
	if !ok {
		t.Fatalf("missing Codex config.toml write: %#v", writes)
	}
	content := string(configWrite.Content)
	for _, want := range []string{
		`model_provider = "OpenAI"`,
		`[model_providers.OpenAI]`,
		`base_url = "https://proxy.example.com/v1"`,
		`wire_api = "responses"`,
		`requires_openai_auth = false`,
		`supports_websockets = false`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("config missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "sk-secret") || strings.Contains(content, "api_key") {
		t.Fatalf("config leaked API key:\n%s", content)
	}
	authWrite, ok := findUsersACPConfigWrite(writes, acpclient.CodexManagedConfigDir+"/auth.json")
	if !ok {
		t.Fatalf("missing Codex auth.json write: %#v", writes)
	}
	auth := string(authWrite.Content)
	if !strings.Contains(auth, `"OPENAI_API_KEY": "sk-secret"`) {
		t.Fatalf("auth missing API key:\n%s", auth)
	}
}

func TestPrepareACPWorkspaceConfigSkipsCodexOAuthConfig(t *testing.T) {
	client, recorder := newUsersACPConfigBridgeClient(t)
	handler := &UsersHandler{
		acpWorkspace: &usersACPConfigWorkspace{
			backend: bridge.WorkspaceBackendContainer,
			client:  client,
		},
	}

	err := handler.prepareACPWorkspaceConfig(context.Background(), bots.Bot{
		ID: "bot-1",
		Metadata: map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{
						"enabled":    true,
						"setup_mode": "oauth",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("prepareACPWorkspaceConfig() error = %v", err)
	}
	if writes := recorder.writes(); len(writes) != 0 {
		t.Fatalf("OAuth setup should be written only by ACP Codex OAuth callback, got writes: %#v", writes)
	}
}

func TestPrepareACPWorkspaceConfigSkipsSelf(t *testing.T) {
	handler := &UsersHandler{acpWorkspace: &usersACPConfigWorkspace{backend: bridge.WorkspaceBackendLocal}}
	selfBot := bots.Bot{
		ID: "bot-1",
		Metadata: map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{
						"enabled":    true,
						"setup_mode": "self",
					},
				},
			},
		},
	}
	if err := handler.prepareACPWorkspaceConfig(context.Background(), selfBot); err != nil {
		t.Fatalf("self setup should be skipped: %v", err)
	}
}

func TestPrepareACPWorkspaceConfigWritesCodexAPIKeyConfigForLocalBYOK(t *testing.T) {
	client, recorder := newUsersACPConfigBridgeClient(t)
	handler := &UsersHandler{
		acpWorkspace: &usersACPConfigWorkspace{
			backend: bridge.WorkspaceBackendLocal,
			client:  client,
		},
	}

	err := handler.prepareACPWorkspaceConfig(context.Background(), bots.Bot{
		ID: "bot-1",
		Metadata: map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{
						"enabled":    true,
						"setup_mode": "api_key",
						"managed": map[string]any{
							"api_key": "sk-secret",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("prepareACPWorkspaceConfig() error = %v", err)
	}
	// Desktop BYOK: local api_key config is written to the bot-scoped .codex dir
	// (the bridge maps /data/.codex onto the workspace root on the local side).
	if writes := recorder.writes(); len(writes) != 2 {
		t.Fatalf("writes len = %d, want config.toml + auth.json: %#v", len(writes), writes)
	}
}

type usersACPConfigWorkspace struct {
	backend string
	client  *bridge.Client
}

func (w *usersACPConfigWorkspace) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return bridge.WorkspaceInfo{Backend: w.backend, DefaultWorkDir: "/data"}, nil
}

func (w *usersACPConfigWorkspace) MCPClient(context.Context, string) (*bridge.Client, error) {
	return w.client, nil
}

func (*usersACPConfigWorkspace) SetupBotContainerWithProgress(context.Context, string, workspace.ContainerSetupProgress) error {
	return nil
}

type usersACPConfigWrite struct {
	Path    string
	Content []byte
}

type usersACPConfigBridgeServer struct {
	pb.UnimplementedContainerServiceServer

	mu    sync.Mutex
	files []usersACPConfigWrite
}

func (s *usersACPConfigBridgeServer) WriteFile(_ context.Context, req *pb.WriteFileRequest) (*pb.WriteFileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, usersACPConfigWrite{
		Path:    req.GetPath(),
		Content: append([]byte(nil), req.GetContent()...),
	})
	return &pb.WriteFileResponse{}, nil
}

func (s *usersACPConfigBridgeServer) writes() []usersACPConfigWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]usersACPConfigWrite, len(s.files))
	copy(out, s.files)
	return out
}

func findUsersACPConfigWrite(writes []usersACPConfigWrite, path string) (usersACPConfigWrite, bool) {
	for _, write := range writes {
		if write.Path == path {
			return write, true
		}
	}
	return usersACPConfigWrite{}, false
}

func newUsersACPConfigBridgeClient(t *testing.T) (*bridge.Client, *usersACPConfigBridgeServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	recorder := &usersACPConfigBridgeServer{}
	pb.RegisterContainerServiceServer(server, recorder)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient("passthrough:///users-acp-config-test",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bridge.NewClientFromConn(conn), recorder
}
