package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/memohai/memoh/internal/agent/turn"
	turntransport "github.com/memohai/memoh/internal/agent/turn/grpctransport"
	"github.com/memohai/memoh/internal/agent/turn/turnpb"
	"github.com/memohai/memoh/internal/audio"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/email"
	"github.com/memohai/memoh/internal/handlers"
	intrpc "github.com/memohai/memoh/internal/rpc"
	"github.com/memohai/memoh/internal/rpc/channelruntime"
	runtimeRpc "github.com/memohai/memoh/internal/rpc/runtime"
	"github.com/memohai/memoh/internal/rpc/runtimepb"
	"github.com/memohai/memoh/internal/rpc/serverruntime"
	"github.com/memohai/memoh/internal/webhooktunnel"
)

type serverRPC struct {
	server *grpc.Server
	addr   string
}

func provideChannelRPCConn(lc fx.Lifecycle, cfg config.Config) (*grpc.ClientConn, error) {
	conn, err := intrpc.Dial(cfg.InternalRPC.ChannelTarget, cfg.InternalRPC.SharedSecret)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return conn.Close() }})
	return conn, nil
}

func provideRuntimeRPCClient(conn *grpc.ClientConn) *runtimeRpc.Client {
	return runtimeRpc.NewClient(conn)
}

func provideChannelRuntimeClient(client *runtimeRpc.Client) *channelruntime.Client {
	return channelruntime.NewClient(client)
}

func provideChannelRuntime(client *channelruntime.Client, manager *channel.Manager) channel.Runtime {
	return &localFirstChannelRuntime{local: manager, remote: client}
}
func provideEmailRuntime(client *channelruntime.Client) email.Runtime { return client }

// channelSendRuntime is the slice of *channel.Manager the local route needs.
type channelSendRuntime interface {
	Send(context.Context, string, channel.ChannelType, channel.SendRequest) error
	React(context.Context, string, channel.ChannelType, channel.ReactRequest) error
}

// localFirstChannelRuntime routes local channel types (web/cli) to this
// process's manager and everything else over the internal RPC. The Web SSE
// stream subscribes to THIS process's RouteHub; the channel process has its
// own hub with no subscribers, so delivering web sends remotely would drop
// them silently (send_message/speak/schedule notifications to the Web
// surface would vanish).
type localFirstChannelRuntime struct {
	local  channelSendRuntime
	remote channel.Runtime
}

func isLocalChannelType(typ channel.ChannelType) bool {
	switch typ {
	case local.WebType, local.CLIType:
		return true
	default:
		return false
	}
}

func (r *localFirstChannelRuntime) Send(ctx context.Context, botID string, typ channel.ChannelType, req channel.SendRequest) error {
	if isLocalChannelType(typ) {
		return r.local.Send(ctx, botID, typ, req)
	}
	return r.remote.Send(ctx, botID, typ, req)
}

func (r *localFirstChannelRuntime) React(ctx context.Context, botID string, typ channel.ChannelType, req channel.ReactRequest) error {
	if isLocalChannelType(typ) {
		return r.local.React(ctx, botID, typ, req)
	}
	return r.remote.React(ctx, botID, typ, req)
}

func (r *localFirstChannelRuntime) UpsertBotChannelConfig(ctx context.Context, botID string, typ channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error) {
	return r.remote.UpsertBotChannelConfig(ctx, botID, typ, req)
}

func (r *localFirstChannelRuntime) SetBotChannelStatus(ctx context.Context, botID string, typ channel.ChannelType, disabled bool) (channel.ChannelConfig, error) {
	return r.remote.SetBotChannelStatus(ctx, botID, typ, disabled)
}

func (r *localFirstChannelRuntime) DeleteBotChannelConfig(ctx context.Context, botID string, typ channel.ChannelType) error {
	return r.remote.DeleteBotChannelConfig(ctx, botID, typ)
}

func (r *localFirstChannelRuntime) SetWebhookEndpoint(ctx context.Context, botID string, typ channel.ChannelType, req channel.SetWebhookEndpointRequest) (channel.SetWebhookEndpointResponse, error) {
	return r.remote.SetWebhookEndpoint(ctx, botID, typ, req)
}

func (r *localFirstChannelRuntime) ConnectionStatusesByBot(botID string) []channel.ConnectionStatus {
	return r.remote.ConnectionStatusesByBot(botID)
}
func provideWebhookTunnelStatus(client *channelruntime.Client) interface{ Status() webhooktunnel.Status } {
	return client
}

// provideLocalWebhookTunnelStatus is the embedded-mode counterpart: the
// tunnel manager runs in this process.
func provideLocalWebhookTunnelStatus(manager *webhooktunnel.Manager) interface{ Status() webhooktunnel.Status } {
	return manager
}

func provideServerRPC(log *slog.Logger, cfg config.Config, turnService turn.Service, commandHandler *command.Handler, skillHandler *handlers.ContainerdHandler, audioService *audio.Service) (*serverRPC, error) {
	if err := cfg.ValidateServerRuntime(); err != nil {
		return nil, err
	}
	server := intrpc.NewServer(cfg.InternalRPC.SharedSecret)
	turnpb.RegisterTurnServiceServer(server, turntransport.NewServer(log, turnService))
	runtimepb.RegisterRuntimeServiceServer(server, runtimeRpc.NewServer(log, serverruntime.Handlers(commandHandler, skillHandler, audioService)))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	return &serverRPC{server: server, addr: cfg.Server.RPCListenAddr}, nil
}

func startServerRPC(lc fx.Lifecycle, log *slog.Logger, rpcServer *serverRPC, shutdowner fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", rpcServer.addr)
			if err != nil {
				return fmt.Errorf("listen server rpc: %w", err)
			}
			go func() {
				log.Info("server rpc listening", slog.String("addr", rpcServer.addr))
				if err := rpcServer.server.Serve(lis); err != nil {
					log.Error("server rpc failed", slog.Any("error", err))
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error { rpcServer.server.GracefulStop(); return nil },
	})
}
