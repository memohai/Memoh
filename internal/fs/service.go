package fs

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	memoryfmt "github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/mcp"
)

type Error struct {
	Code    int
	Message string
	Err     error
}

func (e *Error) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "fs operation failed"
}

func (e *Error) Unwrap() error { return e.Err }

func AsError(err error) (*Error, bool) {
	var fsErr *Error
	if errors.As(err, &fsErr) {
		return fsErr, true
	}
	return nil, false
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

type ListResult struct {
	Path    string     `json:"path"`
	Entries []FileInfo `json:"entries"`
}

type ReadResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
}

type DownloadResult struct {
	FileName    string
	ContentType string
	Data        []byte
	HostPath    string
	FromHost    bool
}

type UploadResult struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Service struct {
	exec              ctr.Service
	queries           *dbsqlc.Queries
	namespace         string
	ensureBotDataRoot func(botID string) (string, error)
}

func NewService(exec ctr.Service, queries *dbsqlc.Queries, namespace string, ensureBotDataRoot func(botID string) (string, error)) *Service {
	return &Service{
		exec:              exec,
		queries:           queries,
		namespace:         strings.TrimSpace(namespace),
		ensureBotDataRoot: ensureBotDataRoot,
	}
}

type pathContext struct {
	containerPath   string
	hostPath        string
	insideDataMount bool
}

func (s *Service) Stat(ctx context.Context, botID, rawPath string) (FileInfo, error) {
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/"
	}
	pc, err := s.resolvePath(botID, rawPath)
	if err != nil {
		return FileInfo{}, err
	}
	if pc.insideDataMount {
		info, osErr := os.Stat(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return FileInfo{}, notFound("not found", osErr)
			}
			return FileInfo{}, internal(osErr.Error(), osErr)
		}
		return osFileInfoToFS(pc.containerPath, info), nil
	}
	out, err := s.execRead(ctx, botID, []string{"stat", "-c", `%n|%s|%a|%Y|%F`, pc.containerPath})
	if err != nil {
		return FileInfo{}, internal(err.Error(), err)
	}
	fi, parseErr := parseStatLine(pc.containerPath, strings.TrimSpace(string(out)))
	if parseErr != nil {
		return FileInfo{}, internal(parseErr.Error(), parseErr)
	}
	return fi, nil
}

func (s *Service) List(ctx context.Context, botID, rawPath string) (ListResult, error) {
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/"
	}
	pc, err := s.resolvePath(botID, rawPath)
	if err != nil {
		return ListResult{}, err
	}
	if pc.insideDataMount {
		dirEntries, osErr := os.ReadDir(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return ListResult{}, notFound("directory not found", osErr)
			}
			return ListResult{}, internal(osErr.Error(), osErr)
		}
		entries := make([]FileInfo, 0, len(dirEntries))
		for _, de := range dirEntries {
			info, infoErr := de.Info()
			if infoErr != nil {
				continue
			}
			childPath := filepath.Join(pc.containerPath, de.Name())
			entries = append(entries, osFileInfoToFS(childPath, info))
		}
		return ListResult{Path: pc.containerPath, Entries: entries}, nil
	}

	out, err := s.execRead(ctx, botID, []string{"ls", "-1a", pc.containerPath})
	if err != nil {
		return ListResult{}, internal(err.Error(), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]FileInfo, 0, len(lines))
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		childPath := filepath.Join(pc.containerPath, name)
		statOut, statErr := s.execRead(ctx, botID, []string{"stat", "-c", `%n|%s|%a|%Y|%F`, childPath})
		if statErr != nil {
			entries = append(entries, FileInfo{Name: name, Path: childPath})
			continue
		}
		fi, parseErr := parseStatLine(childPath, strings.TrimSpace(string(statOut)))
		if parseErr != nil {
			entries = append(entries, FileInfo{Name: name, Path: childPath})
			continue
		}
		entries = append(entries, fi)
	}
	return ListResult{Path: pc.containerPath, Entries: entries}, nil
}

func (s *Service) Read(ctx context.Context, botID, rawPath string) (ReadResult, error) {
	result, err := s.ReadRaw(ctx, botID, rawPath)
	if err != nil {
		return ReadResult{}, err
	}
	result.Content = memoryfmt.RenderMemoryDayForDisplay(result.Path, result.Content)
	result.Size = int64(len(result.Content))
	return result, nil
}

func (s *Service) ReadRaw(ctx context.Context, botID, rawPath string) (ReadResult, error) {
	if strings.TrimSpace(rawPath) == "" {
		return ReadResult{}, badRequest("path is required", nil)
	}
	pc, err := s.resolvePath(botID, rawPath)
	if err != nil {
		return ReadResult{}, err
	}
	if pc.insideDataMount {
		data, osErr := os.ReadFile(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return ReadResult{}, notFound("file not found", osErr)
			}
			return ReadResult{}, internal(osErr.Error(), osErr)
		}
		return ReadResult{Path: pc.containerPath, Content: string(data), Size: int64(len(data))}, nil
	}
	out, err := s.execRead(ctx, botID, []string{"cat", pc.containerPath})
	if err != nil {
		return ReadResult{}, internal(err.Error(), err)
	}
	return ReadResult{Path: pc.containerPath, Content: string(out), Size: int64(len(out))}, nil
}

func (s *Service) Download(ctx context.Context, botID, rawPath string) (DownloadResult, error) {
	if strings.TrimSpace(rawPath) == "" {
		return DownloadResult{}, badRequest("path is required", nil)
	}
	pc, err := s.resolvePath(botID, rawPath)
	if err != nil {
		return DownloadResult{}, err
	}
	fileName := filepath.Base(pc.containerPath)
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if pc.insideDataMount {
		info, osErr := os.Stat(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return DownloadResult{}, notFound("file not found", osErr)
			}
			return DownloadResult{}, internal(osErr.Error(), osErr)
		}
		if info.IsDir() {
			return DownloadResult{}, badRequest("cannot download a directory", nil)
		}
		return DownloadResult{
			FileName:    fileName,
			ContentType: contentType,
			HostPath:    pc.hostPath,
			FromHost:    true,
		}, nil
	}
	out, err := s.execRead(ctx, botID, []string{"base64", pc.containerPath})
	if err != nil {
		return DownloadResult{}, internal(err.Error(), err)
	}
	decoded, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if decErr != nil {
		return DownloadResult{}, internal("failed to decode file content", decErr)
	}
	return DownloadResult{
		FileName:    fileName,
		ContentType: contentType,
		Data:        decoded,
	}, nil
}

func (s *Service) Write(botID, path, content string) error {
	if strings.TrimSpace(path) == "" {
		return badRequest("path is required", nil)
	}
	pc, err := s.resolvePath(botID, path)
	if err != nil {
		return err
	}
	if !pc.insideDataMount {
		return forbidden("write operations are only allowed within the data directory", nil)
	}
	if err := os.MkdirAll(filepath.Dir(pc.hostPath), 0o755); err != nil {
		return internal(err.Error(), err)
	}
	content = memoryfmt.NormalizeMemoryDayContent(pc.containerPath, content)
	if err := os.WriteFile(pc.hostPath, []byte(content), 0o644); err != nil {
		return internal(err.Error(), err)
	}
	return nil
}

func (s *Service) Upload(botID, destPath string, src io.Reader) (UploadResult, error) {
	if strings.TrimSpace(destPath) == "" {
		return UploadResult{}, badRequest("path is required", nil)
	}
	pc, err := s.resolvePath(botID, destPath)
	if err != nil {
		return UploadResult{}, err
	}
	if !pc.insideDataMount {
		return UploadResult{}, forbidden("upload operations are only allowed within the data directory", nil)
	}
	if err := os.MkdirAll(filepath.Dir(pc.hostPath), 0o755); err != nil {
		return UploadResult{}, internal(err.Error(), err)
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return UploadResult{}, internal(err.Error(), err)
	}
	data = []byte(memoryfmt.NormalizeMemoryDayContent(pc.containerPath, string(data)))
	if err := os.WriteFile(pc.hostPath, data, 0o644); err != nil {
		return UploadResult{}, internal(err.Error(), err)
	}
	return UploadResult{Path: pc.containerPath, Size: int64(len(data))}, nil
}

func (s *Service) Mkdir(botID, path string) error {
	if strings.TrimSpace(path) == "" {
		return badRequest("path is required", nil)
	}
	pc, err := s.resolvePath(botID, path)
	if err != nil {
		return err
	}
	if !pc.insideDataMount {
		return forbidden("mkdir operations are only allowed within the data directory", nil)
	}
	if err := os.MkdirAll(pc.hostPath, 0o755); err != nil {
		return internal(err.Error(), err)
	}
	return nil
}

func (s *Service) Delete(botID, path string, recursive bool) error {
	if strings.TrimSpace(path) == "" {
		return badRequest("path is required", nil)
	}
	pc, err := s.resolvePath(botID, path)
	if err != nil {
		return err
	}
	if !pc.insideDataMount {
		return forbidden("delete operations are only allowed within the data directory", nil)
	}
	if filepath.Clean(pc.containerPath) == filepath.Clean(config.DefaultDataMount) {
		return forbidden("cannot delete the data root directory", nil)
	}
	if _, statErr := os.Stat(pc.hostPath); os.IsNotExist(statErr) {
		return notFound("not found", statErr)
	}
	if recursive {
		if err := os.RemoveAll(pc.hostPath); err != nil {
			return internal(err.Error(), err)
		}
		return nil
	}
	if err := os.Remove(pc.hostPath); err != nil {
		return internal(err.Error(), err)
	}
	return nil
}

func (s *Service) Rename(botID, oldPath, newPath string) error {
	if strings.TrimSpace(oldPath) == "" || strings.TrimSpace(newPath) == "" {
		return badRequest("oldPath and newPath are required", nil)
	}
	oldPC, err := s.resolvePath(botID, oldPath)
	if err != nil {
		return err
	}
	newPC, err := s.resolvePath(botID, newPath)
	if err != nil {
		return err
	}
	if !oldPC.insideDataMount || !newPC.insideDataMount {
		return forbidden("rename operations are only allowed within the data directory", nil)
	}
	if _, statErr := os.Stat(oldPC.hostPath); os.IsNotExist(statErr) {
		return notFound("source not found", statErr)
	}
	if err := os.MkdirAll(filepath.Dir(newPC.hostPath), 0o755); err != nil {
		return internal(err.Error(), err)
	}
	if err := os.Rename(oldPC.hostPath, newPC.hostPath); err != nil {
		return internal(err.Error(), err)
	}
	return nil
}

func (s *Service) resolvePath(botID, rawPath string) (pathContext, error) {
	containerPath := filepath.Clean("/" + strings.TrimSpace(rawPath))
	if containerPath == "" {
		containerPath = "/"
	}
	dataMount := filepath.Clean(config.DefaultDataMount)
	if containerPath == dataMount || strings.HasPrefix(containerPath, dataMount+"/") {
		if s.ensureBotDataRoot == nil {
			return pathContext{}, internal("bot data root resolver not configured", nil)
		}
		hostRoot, err := s.ensureBotDataRoot(botID)
		if err != nil {
			return pathContext{}, internal(err.Error(), err)
		}
		relPath := strings.TrimPrefix(containerPath, dataMount)
		if relPath == "" {
			relPath = "/"
		}
		hostPath := filepath.Clean(filepath.Join(hostRoot, filepath.FromSlash(relPath)))
		if !strings.HasPrefix(hostPath, hostRoot) {
			return pathContext{}, badRequest("path traversal detected", nil)
		}
		return pathContext{
			containerPath:   containerPath,
			hostPath:        hostPath,
			insideDataMount: true,
		}, nil
	}
	return pathContext{containerPath: containerPath}, nil
}

func (s *Service) resolveContainerID(ctx context.Context, botID string) string {
	if s.queries != nil {
		pgBotID, err := db.ParseUUID(botID)
		if err == nil {
			row, dbErr := s.queries.GetContainerByBotID(s.namespacedCtx(ctx), pgBotID)
			if dbErr == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID
			}
		}
	}
	return mcp.ContainerPrefix + botID
}

func (s *Service) namespacedCtx(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.namespace != "" {
		return namespaces.WithNamespace(ctx, s.namespace)
	}
	return ctx
}

func (s *Service) execRead(ctx context.Context, botID string, args []string) ([]byte, error) {
	containerID := s.resolveContainerID(ctx, botID)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := s.exec.ExecTask(s.namespacedCtx(ctx), containerID, ctr.ExecTaskRequest{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w", err)
	}
	if result.ExitCode != 0 {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return nil, fmt.Errorf("command failed: %s", errMsg)
	}
	return stdout.Bytes(), nil
}

func osFileInfoToFS(containerPath string, info os.FileInfo) FileInfo {
	return FileInfo{
		Name:    info.Name(),
		Path:    containerPath,
		Size:    info.Size(),
		Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
		IsDir:   info.IsDir(),
	}
}

func parseStatLine(containerPath, line string) (FileInfo, error) {
	parts := strings.SplitN(line, "|", 5)
	if len(parts) < 5 {
		return FileInfo{}, fmt.Errorf("unexpected stat output: %s", line)
	}
	var size int64
	fmt.Sscanf(parts[1], "%d", &size)
	mode := strings.TrimSpace(parts[2])
	var epoch int64
	fmt.Sscanf(parts[3], "%d", &epoch)
	modTime := time.Unix(epoch, 0).UTC().Format(time.RFC3339)
	fileType := strings.TrimSpace(parts[4])
	isDir := strings.Contains(fileType, "directory")
	name := filepath.Base(containerPath)
	if containerPath == "/" {
		name = "/"
	}
	return FileInfo{
		Name:    name,
		Path:    containerPath,
		Size:    size,
		Mode:    mode,
		ModTime: modTime,
		IsDir:   isDir,
	}, nil
}

func badRequest(msg string, err error) error {
	return &Error{Code: http.StatusBadRequest, Message: msg, Err: err}
}

func forbidden(msg string, err error) error {
	return &Error{Code: http.StatusForbidden, Message: msg, Err: err}
}

func notFound(msg string, err error) error {
	return &Error{Code: http.StatusNotFound, Message: msg, Err: err}
}

func internal(msg string, err error) error {
	return &Error{Code: http.StatusInternalServerError, Message: msg, Err: err}
}
