// Package mcpclient provides a gRPC client for the MCP container service.
// Each bot container runs a gRPC server on port 9090 exposing file and exec
// operations. This client wraps the generated gRPC stubs with connection
// pooling and a simplified API for callers.
package mcpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/memohai/memoh/internal/config"
	pb "github.com/memohai/memoh/internal/mcp/mcpcontainer"
)

const connectingTimeout = 30 * time.Second

// Client wraps a gRPC connection to a single MCP container.
type Client struct {
	conn      *grpc.ClientConn
	svc       pb.ContainerServiceClient
	target    string
	createdAt time.Time
}

// NewClientFromConn wraps an existing gRPC connection into a Client.
// Intended for testing with in-process transports such as bufconn.
func NewClientFromConn(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:   conn,
		svc:    pb.NewContainerServiceClient(conn),
		target: conn.Target(),
	}
}

// Dial creates a new Client connected to the given container IP.
func Dial(_ context.Context, ip string) (*Client, error) {
	target := fmt.Sprintf("%s:%d", ip, config.MCPGRPCPort)
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", target, err)
	}
	return &Client{
		conn:      conn,
		svc:       pb.NewContainerServiceClient(conn),
		target:    target,
		createdAt: time.Now(),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) ReadFile(ctx context.Context, path string, lineOffset, nLines int32) (*pb.ReadFileResponse, error) {
	resp, err := c.svc.ReadFile(ctx, &pb.ReadFileRequest{
		Path:       path,
		LineOffset: lineOffset,
		NLines:     nLines,
	})
	return resp, mapError(err)
}

func (c *Client) WriteFile(ctx context.Context, path string, content []byte) error {
	_, err := c.svc.WriteFile(ctx, &pb.WriteFileRequest{
		Path:    path,
		Content: content,
	})
	return mapError(err)
}

func (c *Client) ListDir(ctx context.Context, path string, recursive bool) ([]*pb.FileEntry, error) {
	resp, err := c.svc.ListDir(ctx, &pb.ListDirRequest{
		Path:      path,
		Recursive: recursive,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return resp.GetEntries(), nil
}

func (c *Client) Stat(ctx context.Context, path string) (*pb.FileEntry, error) {
	resp, err := c.svc.Stat(ctx, &pb.StatRequest{Path: path})
	if err != nil {
		return nil, mapError(err)
	}
	return resp.GetEntry(), nil
}

func (c *Client) Mkdir(ctx context.Context, path string) error {
	_, err := c.svc.Mkdir(ctx, &pb.MkdirRequest{Path: path})
	return mapError(err)
}

func (c *Client) Rename(ctx context.Context, oldPath, newPath string) error {
	_, err := c.svc.Rename(ctx, &pb.RenameRequest{OldPath: oldPath, NewPath: newPath})
	return mapError(err)
}

// ExecResult holds the output of a non-streaming exec call.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int32
}

// Exec runs a command and collects all output. For streaming, use ExecStream.
func (c *Client) Exec(ctx context.Context, command, workDir string, timeout int32) (*ExecResult, error) {
	return c.ExecWithStdin(ctx, command, workDir, timeout, nil)
}

// ExecWithStdin runs a command with optional stdin data.
func (c *Client) ExecWithStdin(ctx context.Context, command, workDir string, timeout int32, stdinData []byte) (*ExecResult, error) {
	stream, err := c.svc.Exec(ctx)
	if err != nil {
		return nil, mapError(err)
	}

	// Send config message first
	err = stream.Send(&pb.ExecInput{
		Command:        command,
		WorkDir:        workDir,
		TimeoutSeconds: timeout,
		StdinData:      stdinData,
	})
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	var exitCode int32

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch msg.GetStream() {
		case pb.ExecOutput_STDOUT:
			stdout.Write(msg.GetData())
		case pb.ExecOutput_STDERR:
			stderr.Write(msg.GetData())
		case pb.ExecOutput_EXIT:
			exitCode = msg.GetExitCode()
		}
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// ExecStream returns a bidirectional stream for interactive exec.
// Caller can send stdin data and receive stdout/stderr in real-time.
func (c *Client) ExecStream(ctx context.Context, command, workDir string, timeout int32) (*ExecStream, error) {
	stream, err := c.svc.Exec(ctx)
	if err != nil {
		return nil, mapError(err)
	}

	// Send config message first
	err = stream.Send(&pb.ExecInput{
		Command:        command,
		WorkDir:        workDir,
		TimeoutSeconds: timeout,
	})
	if err != nil {
		return nil, err
	}

	return &ExecStream{stream: stream}, nil
}

// ExecStream wraps a bidirectional exec stream.
type ExecStream struct {
	stream pb.ContainerService_ExecClient
}

// SendStdin sends data to the process stdin.
func (s *ExecStream) SendStdin(data []byte) error {
	return s.stream.Send(&pb.ExecInput{
		StdinData: data,
	})
}

// Recv receives output from the process.
func (s *ExecStream) Recv() (*pb.ExecOutput, error) {
	return s.stream.Recv()
}

// Resize sends a terminal resize event to the running process.
func (s *ExecStream) Resize(cols, rows uint32) error {
	return s.stream.Send(&pb.ExecInput{
		Resize: &pb.TerminalResize{Cols: cols, Rows: rows},
	})
}

// Close closes the stream.
func (s *ExecStream) Close() error {
	return s.stream.CloseSend()
}

// ExecStreamPTY opens a bidirectional PTY exec stream.
// The command runs inside a pseudo-terminal with the given initial size.
func (c *Client) ExecStreamPTY(ctx context.Context, command, workDir string, cols, rows uint32) (*ExecStream, error) {
	stream, err := c.svc.Exec(ctx)
	if err != nil {
		return nil, mapError(err)
	}

	err = stream.Send(&pb.ExecInput{
		Command: command,
		WorkDir: workDir,
		Pty:     true,
		Resize:  &pb.TerminalResize{Cols: cols, Rows: rows},
	})
	if err != nil {
		return nil, err
	}

	return &ExecStream{stream: stream}, nil
}

// ReadRaw streams raw file bytes. Caller must consume the returned reader.
func (c *Client) ReadRaw(ctx context.Context, path string) (io.ReadCloser, error) {
	stream, err := c.svc.ReadRaw(ctx, &pb.ReadRawRequest{Path: path})
	if err != nil {
		return nil, mapError(err)
	}
	return newStreamReader(stream)
}

// WriteRaw writes raw bytes to a file in the container.
func (c *Client) WriteRaw(ctx context.Context, path string, r io.Reader) (int64, error) {
	stream, err := c.svc.WriteRaw(ctx)
	if err != nil {
		return 0, mapError(err)
	}

	buf := make([]byte, 64*1024)
	first := true
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			chunk := &pb.WriteRawChunk{Data: buf[:n]}
			if first {
				chunk.Path = path
				first = false
			}
			if sendErr := stream.Send(chunk); sendErr != nil {
				return 0, sendErr
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return 0, err
	}
	return resp.GetBytesWritten(), nil
}

func (c *Client) DeleteFile(ctx context.Context, path string, recursive bool) error {
	_, err := c.svc.DeleteFile(ctx, &pb.DeleteFileRequest{
		Path:      path,
		Recursive: recursive,
	})
	return mapError(err)
}

// streamReader adapts a gRPC server stream into an io.ReadCloser.
type streamReader struct {
	stream pb.ContainerService_ReadRawClient
	buf    []byte
	off    int
}

func newStreamReader(stream pb.ContainerService_ReadRawClient) (io.ReadCloser, error) {
	first, err := stream.Recv()
	switch {
	case errors.Is(err, io.EOF):
		return io.NopCloser(bytes.NewReader(nil)), nil
	case err != nil:
		return nil, mapError(err)
	default:
		return &streamReader{stream: stream, buf: first.GetData()}, nil
	}
}

func (r *streamReader) fill() error {
	for r.off >= len(r.buf) {
		msg, err := r.stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			return mapError(err)
		}
		r.buf = msg.GetData()
		r.off = 0
	}
	return nil
}

func (r *streamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := r.fill(); err != nil {
		return 0, err
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

func (*streamReader) Close() error {
	return nil
}

// Provider resolves a gRPC client for a given bot container.
type Provider interface {
	MCPClient(ctx context.Context, botID string) (*Client, error)
}

// Pool manages cached gRPC clients keyed by bot ID.
type Pool struct {
	mu      sync.RWMutex
	clients map[string]*Client
	ipFunc  func(botID string) string
}

// NewPool creates a client pool. ipFunc maps bot ID to container IP.
func NewPool(ipFunc func(string) string) *Pool {
	return &Pool{
		clients: make(map[string]*Client),
		ipFunc:  ipFunc,
	}
}

// MCPClient implements Provider. Alias for Get.
func (p *Pool) MCPClient(ctx context.Context, botID string) (*Client, error) {
	return p.Get(ctx, botID)
}

// Get returns a cached client or dials a new one.
// Stale connections (Shutdown / TransientFailure / stuck Connecting) are evicted automatically.
func (p *Pool) Get(ctx context.Context, botID string) (*Client, error) {
	p.mu.RLock()
	if c, ok := p.clients[botID]; ok {
		state := c.conn.GetState()
		stale := state == connectivity.Shutdown || state == connectivity.TransientFailure ||
			(state == connectivity.Connecting && time.Since(c.createdAt) > connectingTimeout)
		if !stale {
			p.mu.RUnlock()
			return c, nil
		}
		p.mu.RUnlock()
		p.Remove(botID)
	} else {
		p.mu.RUnlock()
	}

	ip := p.ipFunc(botID)
	if ip == "" {
		return nil, fmt.Errorf("no IP for bot %s", botID)
	}

	c, err := Dial(ctx, ip)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if existing, ok := p.clients[botID]; ok {
		p.mu.Unlock()
		_ = c.Close()
		return existing, nil
	}
	p.clients[botID] = c
	p.mu.Unlock()
	return c, nil
}

// Remove closes and removes the client for a bot.
func (p *Pool) Remove(botID string) {
	p.mu.Lock()
	if c, ok := p.clients[botID]; ok {
		_ = c.Close()
		delete(p.clients, botID)
	}
	p.mu.Unlock()
}

// CloseAll closes all cached clients.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	for id, c := range p.clients {
		_ = c.Close()
		delete(p.clients, id)
	}
	p.mu.Unlock()
}
