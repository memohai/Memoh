package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	ReadMediaToolName        = "read_media"
	toolReadMedia            = ReadMediaToolName
	defaultReadMediaRoot     = "/data"
	defaultReadMediaMaxBytes = 20 * 1024 * 1024
)

var readMediaSupportedMimeTypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

// ReadMediaToolResult is the public result returned to the model.
type ReadMediaToolResult struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path,omitempty"`
	Mime  string `json:"mime,omitempty"`
	Size  int    `json:"size,omitempty"`
	Error string `json:"error,omitempty"`
}

// ReadMediaToolOutput is the internal execution result used by the agent to
// inject the image into the next Twilight AI step while keeping the visible
// tool result lightweight.
type ReadMediaToolOutput struct {
	Public         ReadMediaToolResult
	ImageBase64    string
	ImageMediaType string
}

type readMediaToolOutput = ReadMediaToolOutput

type ReadMediaProvider struct {
	clients  bridge.Provider
	rootDir  string
	maxBytes int64
	logger   *slog.Logger
}

func NewReadMediaProvider(log *slog.Logger, clients bridge.Provider, rootDir string) *ReadMediaProvider {
	if log == nil {
		log = slog.Default()
	}
	root := strings.TrimSpace(rootDir)
	if root == "" {
		root = defaultReadMediaRoot
	}
	return &ReadMediaProvider{
		clients:  clients,
		rootDir:  path.Clean(root),
		maxBytes: defaultReadMediaMaxBytes,
		logger:   log.With(slog.String("tool", "read_media")),
	}
}

func (p *ReadMediaProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if p == nil || p.clients == nil || !session.SupportsImageInput {
		return nil, nil
	}
	root := p.rootDir
	if root == "" {
		root = defaultReadMediaRoot
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        toolReadMedia,
			Description: fmt.Sprintf("Load an image file from %s into model context so you can inspect it. Relative paths are resolved under %s.", root, root),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": fmt.Sprintf("Image file path under %s. Absolute paths must stay under %s; relative paths are resolved under %s.", root, root, root),
					},
				},
				"required": []string{"path"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execReadMedia(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

func (p *ReadMediaProvider) execReadMedia(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	client, err := p.getClient(ctx, session.BotID)
	if err != nil {
		return readMediaErrorResult(err.Error()), nil
	}

	resolvedPath, err := p.resolveImagePath(StringArg(args, "path"))
	if err != nil {
		return readMediaErrorResult(err.Error()), nil
	}

	reader, err := client.ReadRaw(ctx, resolvedPath)
	if err != nil {
		return readMediaErrorResult(err.Error()), nil
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(io.LimitReader(reader, p.maxBytes+1))
	if err != nil {
		return readMediaErrorResult("read_media failed to load image: " + err.Error()), nil
	}
	if int64(len(data)) > p.maxBytes {
		return readMediaErrorResult(fmt.Sprintf("read_media failed to load image: file exceeds %d bytes", p.maxBytes)), nil
	}

	mimeType, err := detectReadMediaMime(data)
	if err != nil {
		return readMediaErrorResult(err.Error()), nil
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return ReadMediaToolOutput{
		Public: ReadMediaToolResult{
			OK:   true,
			Path: resolvedPath,
			Mime: mimeType,
			Size: len(data),
		},
		ImageBase64:    encoded,
		ImageMediaType: mimeType,
	}, nil
}

func (p *ReadMediaProvider) getClient(ctx context.Context, botID string) (*bridge.Client, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}
	client, err := p.clients.MCPClient(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("container not reachable: %w", err)
	}
	return client, nil
}

func (p *ReadMediaProvider) resolveImagePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("path is required")
	}

	root := p.rootDir
	if root == "" {
		root = defaultReadMediaRoot
	}
	root = path.Clean(root)

	resolved := trimmed
	if !strings.HasPrefix(resolved, "/") {
		resolved = path.Join(root, resolved)
	}
	resolved = path.Clean(resolved)

	if resolved == root || !strings.HasPrefix(resolved, root+"/") {
		return "", fmt.Errorf("path must be under %s", root)
	}
	return resolved, nil
}

func readMediaErrorResult(message string) ReadMediaToolOutput {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "read_media failed"
	}
	return ReadMediaToolOutput{
		Public: ReadMediaToolResult{
			OK:    false,
			Error: msg,
		},
	}
}

func detectReadMediaMime(data []byte) (string, error) {
	sniffedMime := ""
	if len(data) > 0 {
		sniffedMime = strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	}

	switch {
	case sniffedMime == "":
		return "", errors.New("read_media only supports PNG, JPEG, GIF, or WebP image bytes")
	case isSupportedReadMediaMime(sniffedMime):
		return sniffedMime, nil
	default:
		return "", errors.New("read_media only supports PNG, JPEG, GIF, or WebP image bytes")
	}
}

func isSupportedReadMediaMime(mimeType string) bool {
	_, ok := readMediaSupportedMimeTypes[strings.ToLower(strings.TrimSpace(mimeType))]
	return ok
}
