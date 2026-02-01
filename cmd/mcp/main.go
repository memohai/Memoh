package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	commitHash = "unknown"
	version    = "unknown"
)

func main() {
	if version == "unknown" {
		version = "v0.0.0-dev+" + commitHash
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "text"
	}
	logger.Init(logLevel, logFormat)

	server := gomcp.NewServer(
		&gomcp.Implementation{Name: "memoh-mcp", Version: version},
		nil,
	)
	mcp.RegisterTools(server)
	if err := server.Run(context.Background(), &gomcp.StdioTransport{}); err != nil {
		logger.Error("mcp server failed", slog.Any("error", err))
		os.Exit(1)
	}
}
