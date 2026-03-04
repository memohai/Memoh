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

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/memohai/memoh/internal/logger"
	pb "github.com/memohai/memoh/internal/mcp/mcpcontainer"
)

const (
	defaultListenAddr = ":9090"
	templateDir       = "/opt/mcp-template"
)

// initDataDir ensures /data exists and seeds template files on first boot.
func initDataDir() {
	if err := os.MkdirAll(defaultWorkDir, 0o750); err != nil {
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
		dst := filepath.Join(defaultWorkDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templateDir, e.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, fs.FileMode(0o644)); err != nil {
			logger.Warn("failed to seed template", slog.String("file", e.Name()), slog.Any("error", err))
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initDataDir()

	addr := os.Getenv("MCP_LISTEN_ADDR")
	if addr == "" {
		addr = defaultListenAddr
	}

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		logger.Error("failed to listen", slog.String("addr", addr), slog.Any("error", err))
		return
	}

	srv := grpc.NewServer()
	pb.RegisterContainerServiceServer(srv, &containerServer{})
	reflection.Register(srv)

	go func() {
		<-ctx.Done()
		logger.FromContext(ctx).Info("shutting down gRPC server")
		srv.GracefulStop()
	}()

	logger.Info("mcp gRPC server listening", slog.String("addr", addr))
	if err := srv.Serve(lis); err != nil {
		logger.Error("gRPC server failed", slog.Any("error", err))
		return
	}
}
