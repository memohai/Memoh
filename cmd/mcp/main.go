package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/version"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	server := gomcp.NewServer(
		&gomcp.Implementation{Name: "memoh-mcp", Version: version.GetInfo()},
		nil,
	)
	mcp.RegisterTools(server)
	if err := server.Run(context.Background(), &gomcp.StdioTransport{}); err != nil {
		logger.Error("mcp server failed", slog.Any("error", err))
		os.Exit(1)
	}
}
