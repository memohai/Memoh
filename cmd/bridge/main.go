package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/memohai/memoh/internal/logger"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
	"github.com/memohai/memoh/internal/workspace/bridgesvc"
)

const (
	defaultSocketPath = "/run/memoh/bridge.sock"
	templateDir       = "/opt/memoh/templates"
)

// initDataDir ensures /data exists and seeds template files on first boot.
func initDataDir() {
	if err := os.MkdirAll(bridgesvc.DefaultWorkDir, 0o750); err != nil {
		logger.Warn("failed to create data dir", slog.Any("error", err))
		return
	}

	entries, err := os.ReadDir(templateDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("failed to read template dir", slog.String("dir", templateDir), slog.Any("error", err))
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		dst := filepath.Join(bridgesvc.DefaultWorkDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templateDir, e.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, fs.FileMode(0o644)); err != nil { //nolint:gosec // G703: dst is built from filepath.Join(defaultWorkDir, e.Name()) where e comes from os.ReadDir
			logger.Warn("failed to seed template", slog.String("file", e.Name()), slog.Any("error", err))
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initDataDir()

	// Append toolkit to PATH so child processes (via /bin/sh -c) can find npx/uvx.
	// Container-native tools take priority since toolkit is appended at the end.
	_ = os.Setenv("PATH", os.Getenv("PATH")+":/opt/memoh/toolkit/bin")

	// PID 1 zombie reaping: when bridge runs as PID 1 inside a container,
	// orphaned child processes become zombies unless reaped.
	// On Linux 5.3+, Go's os/exec uses pidfd_open which avoids races between
	// this reaper and cmd.Wait(). Kernels below 5.3 may see rare ECHILD errors.
	go func() {
		var status syscall.WaitStatus
		for {
			if _, err := syscall.Wait4(-1, &status, 0, nil); err != nil {
				time.Sleep(time.Second)
			}
		}
	}()

	network := "unix"
	address := os.Getenv("BRIDGE_SOCKET_PATH")
	if tcpAddr := os.Getenv("BRIDGE_TCP_ADDR"); tcpAddr != "" {
		network = "tcp"
		address = tcpAddr
	}
	if address == "" {
		address = defaultSocketPath
	}
	if network == "unix" {
		// Clean up residual socket from a previous run.
		_ = os.Remove(filepath.Clean(address)) //nolint:gosec // G703: address is from BRIDGE_SOCKET_PATH env or a compiled-in default, not end-user input
	}

	lis, err := (&net.ListenConfig{}).Listen(ctx, network, address)
	if err != nil {
		logger.Error("failed to listen", slog.String("network", network), slog.String("address", address), slog.Any("error", err))
		return
	}

	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.MaxSendMsgSize(16*1024*1024),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 10 * time.Second,
			Time:                  60 * time.Second,
			Timeout:               15 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	pb.RegisterContainerServiceServer(srv, bridgesvc.New(bridgesvc.Options{
		DefaultWorkDir:    bridgesvc.DefaultWorkDir,
		DataMount:         bridgesvc.DefaultWorkDir,
		AllowHostAbsolute: true,
	}))
	reflection.Register(srv)

	go func() {
		<-ctx.Done()
		logger.FromContext(ctx).Info("shutting down gRPC server")
		srv.GracefulStop()
	}()

	logger.Info("bridge gRPC server listening", slog.String("network", network), slog.String("address", address))
	if err := srv.Serve(lis); err != nil {
		logger.Error("gRPC server failed", slog.Any("error", err))
		return
	}
}
