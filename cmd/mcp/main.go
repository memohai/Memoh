// Package main is the entry point for the Memoh MCP stdio server.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/version"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// File tools (read/write/list/edit) are provided by the agent's MCP tool gateway, not this binary.
	server := gomcp.NewServer(
		&gomcp.Implementation{Name: "memoh-mcp", Version: version.GetInfo()},
		nil,
	)
	err := server.Run(ctx, &gomcp.StdioTransport{})
	if ctx.Err() != nil {
		return 0
	}
	if err == nil {
		logger.Warn("mcp server exited without error; waiting for shutdown signal")
		<-ctx.Done()
		return 0
	}
	if errors.Is(err, io.EOF) {
		logger.Warn("mcp stdio closed; waiting for shutdown signal")
		<-ctx.Done()
		return 0
	}
	logger.Error("mcp server failed", slog.Any("error", err))
	return 1
}
