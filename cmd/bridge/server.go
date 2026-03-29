package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const (
	readMaxLines     = 200
	readMaxBytes     = 5120
	readMaxLineLen   = 1000
	binaryProbeBytes = 8 * 1024
	rawChunkSize     = 64 * 1024
	defaultWorkDir   = "/data"
	defaultTimeout   = 30
)

type containerServer struct {
	pb.UnimplementedContainerServiceServer
}

func (*containerServer) ReadFile(_ context.Context, req *pb.ReadFileRequest) (*pb.ReadFileResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	path = resolvePath(path)

	f, err := os.Open(path) //nolint:gosec // G304: MCP container filesystem server; paths are resolved within the container's /data, SSRF is by design
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "open: %v", err)
	}
	defer func() { _ = f.Close() }()

	probe := make([]byte, binaryProbeBytes)
	n, _ := f.Read(probe)
	if bytes.IndexByte(probe[:n], 0) >= 0 {
		return &pb.ReadFileResponse{Binary: true}, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, status.Errorf(codes.Internal, "seek: %v", err)
	}

	lineOffset := req.GetLineOffset()
	if lineOffset < 1 {
		lineOffset = 1
	}
	nLines := req.GetNLines()
	if nLines < 1 || nLines > readMaxLines {
		nLines = readMaxLines
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentLine int32
	var totalLines int32
	var out strings.Builder
	var linesRead int32
	bytesWritten := 0

	for scanner.Scan() {
		currentLine++
		totalLines = currentLine
		if currentLine < lineOffset {
			continue
		}
		if linesRead >= nLines {
			continue // keep scanning to count total lines
		}

		line := scanner.Text()
		if utf8.RuneCountInString(line) > readMaxLineLen {
			line = truncateRunes(line, readMaxLineLen) + "..."
		}

		entry := line + "\n"
		if bytesWritten+len(entry) > readMaxBytes {
			break
		}
		out.WriteString(entry)
		bytesWritten += len(entry)
		linesRead++
	}

	// Drain remaining lines for total count.
	for scanner.Scan() {
		totalLines++
	}

	return &pb.ReadFileResponse{
		Content:    out.String(),
		TotalLines: totalLines,
	}, nil
}

func (*containerServer) WriteFile(_ context.Context, req *pb.WriteFileRequest) (*pb.WriteFileResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	path = resolvePath(path)

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir: %v", err)
	}
	if err := os.WriteFile(path, req.GetContent(), 0o600); err != nil {
		return nil, status.Errorf(codes.Internal, "write: %v", err)
	}
	return &pb.WriteFileResponse{}, nil
}

func (*containerServer) ListDir(_ context.Context, req *pb.ListDirRequest) (*pb.ListDirResponse, error) {
	dir := req.GetPath()
	if dir == "" {
		dir = "."
	}
	dir = resolvePath(dir)

	var entries []*pb.FileEntry

	if req.GetRecursive() {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}
			rel, _ := filepath.Rel(dir, p)
			if rel == "." {
				return nil
			}
			entry, _ := buildFileEntry(rel, p, d)
			if entry != nil {
				entries = append(entries, entry)
			}
			return nil
		})
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "walk: %v", err)
		}
	} else {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "readdir: %v", err)
		}
		for _, d := range dirEntries {
			entry, _ := buildFileEntry(d.Name(), filepath.Join(dir, d.Name()), d)
			if entry != nil {
				entries = append(entries, entry)
			}
		}
	}

	return &pb.ListDirResponse{Entries: entries}, nil
}

func (*containerServer) Exec(stream pb.ContainerService_ExecServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to receive exec config")
	}

	command := firstMsg.GetCommand()
	if command == "" {
		return status.Error(codes.InvalidArgument, "command is required")
	}

	if firstMsg.GetPty() {
		return execPTY(stream, firstMsg)
	}
	return execPipe(stream, firstMsg)
}

func execPTY(stream pb.ContainerService_ExecServer, firstMsg *pb.ExecInput) error {
	command := firstMsg.GetCommand()
	workDir := firstMsg.GetWorkDir()
	if workDir == "" {
		workDir = defaultWorkDir
	}

	var cmd *exec.Cmd
	if isBarePath(command) {
		cmd = exec.CommandContext(stream.Context(), command) //nolint:gosec // G204: intentional
	} else {
		cmd = exec.CommandContext(stream.Context(), "/bin/sh", "-c", command) //nolint:gosec // G204: intentional
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), firstMsg.GetEnv()...)
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	initialSize := &pty.Winsize{Rows: 24, Cols: 80}
	if r := firstMsg.GetResize(); r != nil && r.GetCols() > 0 && r.GetRows() > 0 {
		initialSize.Rows = uint16(r.GetRows()) //nolint:gosec // G115
		initialSize.Cols = uint16(r.GetCols()) //nolint:gosec // G115
	}

	ptmx, err := pty.StartWithSize(cmd, initialSize)
	if err != nil {
		return status.Errorf(codes.Internal, "pty start: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// stdin + resize from stream
	go func() {
		for {
			msg, recvErr := stream.Recv()
			if recvErr != nil {
				return
			}
			if r := msg.GetResize(); r != nil && r.GetCols() > 0 && r.GetRows() > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{
					Rows: uint16(r.GetRows()), //nolint:gosec // G115
					Cols: uint16(r.GetCols()), //nolint:gosec // G115
				})
			}
			if data := msg.GetStdinData(); len(data) > 0 {
				_, _ = ptmx.Write(data)
			}
		}
	}()

	// PTY output -> stream (single fd merges stdout+stderr)
	streamPipe(stream, ptmx, pb.ExecOutput_STDOUT)

	exitCode := int32(0)
	if waitErr := cmd.Wait(); waitErr != nil {
		exitErr := &exec.ExitError{}
		if errors.As(waitErr, &exitErr) {
			ec := exitErr.ExitCode()
			exitCode = int32(max(math.MinInt32, min(math.MaxInt32, ec))) //nolint:gosec // G115
		} else {
			exitCode = -1
		}
	}

	return stream.Send(&pb.ExecOutput{
		Stream:   pb.ExecOutput_EXIT,
		ExitCode: exitCode,
	})
}

func execPipe(stream pb.ContainerService_ExecServer, firstMsg *pb.ExecInput) error {
	command := firstMsg.GetCommand()
	workDir := firstMsg.GetWorkDir()
	if workDir == "" {
		workDir = defaultWorkDir
	}

	timeout := int(firstMsg.GetTimeoutSeconds())
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(stream.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command) //nolint:gosec // G204: MCP exec tool intentionally executes agent-issued shell commands inside the container
	cmd.Dir = workDir
	if len(firstMsg.GetEnv()) > 0 {
		cmd.Env = append(os.Environ(), firstMsg.GetEnv()...)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return status.Errorf(codes.Internal, "stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return status.Errorf(codes.Internal, "stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return status.Errorf(codes.Internal, "stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return status.Errorf(codes.Internal, "start: %v", err)
	}

	go func() {
		for {
			msg, recvErr := stream.Recv()
			if recvErr != nil {
				_ = stdinPipe.Close()
				return
			}
			if data := msg.GetStdinData(); len(data) > 0 {
				_, _ = stdinPipe.Write(data)
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		streamPipe(stream, stdoutPipe, pb.ExecOutput_STDOUT)
	}()
	streamPipe(stream, stderrPipe, pb.ExecOutput_STDERR)
	<-done

	exitCode := int32(0)
	if err := cmd.Wait(); err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			ec := exitErr.ExitCode()
			exitCode = int32(max(math.MinInt32, min(math.MaxInt32, ec))) //nolint:gosec // G115: value is clamped to int32 range above; Unix exit codes are 0-255
		} else {
			exitCode = -1
		}
	}

	return stream.Send(&pb.ExecOutput{
		Stream:   pb.ExecOutput_EXIT,
		ExitCode: exitCode,
	})
}

func (*containerServer) ReadRaw(req *pb.ReadRawRequest, stream pb.ContainerService_ReadRawServer) error {
	path := req.GetPath()
	if path == "" {
		return status.Error(codes.InvalidArgument, "path is required")
	}
	path = resolvePath(path)

	f, err := os.Open(path) //nolint:gosec // G304: MCP container filesystem server; path is resolved within the container
	if err != nil {
		return status.Errorf(codes.NotFound, "open: %v", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, rawChunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&pb.DataChunk{Data: buf[:n]}); sendErr != nil {
				return sendErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "read: %v", err)
		}
	}
	return nil
}

func (*containerServer) WriteRaw(stream pb.ContainerService_WriteRawServer) error {
	var f *os.File
	var written int64

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if f == nil {
			path := chunk.GetPath()
			if path == "" {
				return status.Error(codes.InvalidArgument, "first chunk must include path")
			}
			path = resolvePath(path)
			if mkErr := os.MkdirAll(filepath.Dir(path), 0o750); mkErr != nil {
				return status.Errorf(codes.Internal, "mkdir: %v", mkErr)
			}
			f, err = os.Create(path) //nolint:gosec // G304: MCP container filesystem server; path is resolved within the container
			if err != nil {
				return status.Errorf(codes.Internal, "create: %v", err)
			}
			defer func() { _ = f.Close() }()
		}

		if len(chunk.GetData()) > 0 {
			n, err := f.Write(chunk.GetData())
			written += int64(n)
			if err != nil {
				return status.Errorf(codes.Internal, "write: %v", err)
			}
		}
	}

	return stream.SendAndClose(&pb.WriteRawResponse{BytesWritten: written})
}

func (*containerServer) DeleteFile(_ context.Context, req *pb.DeleteFileRequest) (*pb.DeleteFileResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	path = resolvePath(path)

	var err error
	if req.GetRecursive() {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "delete: %v", err)
	}
	return &pb.DeleteFileResponse{}, nil
}

func (*containerServer) Stat(_ context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	path = resolvePath(path)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "not found")
		}
		return nil, status.Errorf(codes.Internal, "stat: %v", err)
	}
	return &pb.StatResponse{
		Entry: &pb.FileEntry{
			Path:    filepath.Base(path),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format(time.RFC3339),
		},
	}, nil
}

func (*containerServer) Mkdir(_ context.Context, req *pb.MkdirRequest) (*pb.MkdirResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	path = resolvePath(path)

	if err := os.MkdirAll(path, 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir: %v", err)
	}
	return &pb.MkdirResponse{}, nil
}

func (*containerServer) Rename(_ context.Context, req *pb.RenameRequest) (*pb.RenameResponse, error) {
	oldPath := req.GetOldPath()
	newPath := req.GetNewPath()
	if oldPath == "" || newPath == "" {
		return nil, status.Error(codes.InvalidArgument, "old_path and new_path are required")
	}
	oldPath = resolvePath(oldPath)
	newPath = resolvePath(newPath)

	if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir parent: %v", err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return nil, status.Errorf(codes.Internal, "rename: %v", err)
	}
	return &pb.RenameResponse{}, nil
}

// isBarePath returns true when the command string is a plain executable path
// (no spaces, shell operators, or arguments) so it can be exec'd directly
// without wrapping in "/bin/sh -c". This matters for interactive PTY shells
// where /bin/sh -c wrapping would strip readline support.
func isBarePath(cmd string) bool {
	if cmd == "" {
		return false
	}
	for _, c := range cmd {
		if c == ' ' || c == '\t' || c == '|' || c == '&' || c == ';' || c == '>' || c == '<' || c == '$' || c == '(' || c == ')' || c == '`' {
			return false
		}
	}
	return strings.HasPrefix(cmd, "/") || !strings.Contains(cmd, "/")
}

func streamPipe(stream pb.ContainerService_ExecServer, r io.Reader, st pb.ExecOutput_Stream) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_ = stream.Send(&pb.ExecOutput{
				Stream: st,
				Data:   buf[:n],
			})
		}
		if err != nil {
			break
		}
	}
}

func buildFileEntry(name, _ string, d fs.DirEntry) (*pb.FileEntry, error) {
	info, err := d.Info()
	if err != nil {
		return nil, err
	}
	return &pb.FileEntry{
		Path:    name,
		IsDir:   d.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Format(time.RFC3339),
	}, nil
}

func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(defaultWorkDir, path)
}

func truncateRunes(s string, maxRunes int) string {
	pos := 0
	count := 0
	for pos < len(s) && count < maxRunes {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
		count++
	}
	return s[:pos]
}
