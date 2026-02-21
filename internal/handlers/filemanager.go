package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/mcp"
)

// ---------- request / response types ----------

// FSFileInfo describes a file or directory entry.
type FSFileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

// FSListResponse is the response for a directory listing.
type FSListResponse struct {
	Path    string       `json:"path"`
	Entries []FSFileInfo `json:"entries"`
}

// FSReadResponse is the response when reading text content.
type FSReadResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
}

// FSWriteRequest is the body for creating / overwriting a file.
type FSWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FSUploadResponse is returned after a successful upload.
type FSUploadResponse struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// FSMkdirRequest is the body for creating a directory.
type FSMkdirRequest struct {
	Path string `json:"path"`
}

// FSDeleteRequest is the body for deleting a file or directory.
type FSDeleteRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

// FSRenameRequest is the body for renaming / moving an entry.
type FSRenameRequest struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
}

type fsOpResponse struct {
	OK bool `json:"ok"`
}

// ---------- path resolution ----------

// fsPathContext holds the resolved host path for a container-relative path,
// or indicates that exec-based fallback is required.
type fsPathContext struct {
	// containerPath is the cleaned absolute path inside the container.
	containerPath string
	// hostPath is set when the path lives under the data mount and can be
	// served directly from the host filesystem.
	hostPath string
	// insideDataMount is true when containerPath is within the data mount.
	insideDataMount bool
}

// resolveContainerPath maps a container-internal path to a host path when
// possible (i.e. within the data mount), otherwise returns a context that
// tells the caller to use exec-based fallback.
func (h *ContainerdHandler) resolveContainerPath(botID, rawPath string) (fsPathContext, error) {
	containerPath := filepath.Clean("/" + strings.TrimSpace(rawPath))
	if containerPath == "" {
		containerPath = "/"
	}

	dataMount := strings.TrimSpace(h.cfg.DataMount)
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}
	dataMount = filepath.Clean(dataMount)

	// Check whether the requested path falls under the data mount.
	if containerPath == dataMount || strings.HasPrefix(containerPath, dataMount+"/") {
		hostRoot, err := h.ensureBotDataRoot(botID)
		if err != nil {
			return fsPathContext{}, err
		}
		relPath := strings.TrimPrefix(containerPath, dataMount)
		if relPath == "" {
			relPath = "/"
		}
		hostPath := filepath.Join(hostRoot, filepath.FromSlash(relPath))

		// Prevent path traversal: resolved path must stay under hostRoot.
		hostPath = filepath.Clean(hostPath)
		if !strings.HasPrefix(hostPath, hostRoot) {
			return fsPathContext{}, fmt.Errorf("path traversal detected")
		}

		return fsPathContext{
			containerPath:   containerPath,
			hostPath:        hostPath,
			insideDataMount: true,
		}, nil
	}

	// Outside data mount – exec fallback only.
	return fsPathContext{
		containerPath:   containerPath,
		insideDataMount: false,
	}, nil
}

// resolveContainerID returns the containerd container ID for a given bot.
func (h *ContainerdHandler) resolveContainerIDForFS(botID string) string {
	if h.queries != nil {
		pgBotID, err := db.ParseUUID(botID)
		if err == nil {
			row, dbErr := h.queries.GetContainerByBotID(h.fsContext(), pgBotID)
			if dbErr == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID
			}
		}
	}
	return mcp.ContainerPrefix + botID
}

func (h *ContainerdHandler) fsContext() context.Context {
	ctx := context.Background()
	if strings.TrimSpace(h.namespace) != "" {
		ctx = namespaces.WithNamespace(ctx, h.namespace)
	}
	return ctx
}

// execRead runs a command inside the container and returns stdout as bytes.
func (h *ContainerdHandler) execRead(botID string, args []string) ([]byte, error) {
	containerID := h.resolveContainerIDForFS(botID)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, err := h.service.ExecTask(h.fsContext(), containerID, ctr.ExecTaskRequest{
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

// ---------- handlers ----------

// FSStat godoc
// @Summary Get file or directory info
// @Description Returns metadata about a file or directory at the given container path
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param path query string true "Container path"
// @Success 200 {object} FSFileInfo
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs [get]
func (h *ContainerdHandler) FSStat(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/"
	}

	pc, err := h.resolveContainerPath(botID, rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if pc.insideDataMount {
		info, osErr := os.Stat(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, osErr.Error())
		}
		return c.JSON(http.StatusOK, osFileInfoToFS(pc.containerPath, info))
	}

	// Exec fallback.
	out, err := h.execRead(botID, []string{"stat", "-c", `%n|%s|%a|%Y|%F`, pc.containerPath})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	fi, parseErr := parseStatLine(pc.containerPath, strings.TrimSpace(string(out)))
	if parseErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, parseErr.Error())
	}
	return c.JSON(http.StatusOK, fi)
}

// FSList godoc
// @Summary List directory contents
// @Description Lists files and directories at the given container path
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param path query string true "Container directory path"
// @Success 200 {object} FSListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/list [get]
func (h *ContainerdHandler) FSList(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/"
	}

	pc, err := h.resolveContainerPath(botID, rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if pc.insideDataMount {
		dirEntries, osErr := os.ReadDir(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return echo.NewHTTPError(http.StatusNotFound, "directory not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, osErr.Error())
		}
		entries := make([]FSFileInfo, 0, len(dirEntries))
		for _, de := range dirEntries {
			info, infoErr := de.Info()
			if infoErr != nil {
				continue
			}
			childPath := filepath.Join(pc.containerPath, de.Name())
			entries = append(entries, osFileInfoToFS(childPath, info))
		}
		return c.JSON(http.StatusOK, FSListResponse{Path: pc.containerPath, Entries: entries})
	}

	// Exec fallback.
	out, err := h.execRead(botID, []string{"ls", "-1a", pc.containerPath})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]FSFileInfo, 0, len(lines))
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		childPath := filepath.Join(pc.containerPath, name)
		// Try to stat each entry for richer info.
		statOut, statErr := h.execRead(botID, []string{"stat", "-c", `%n|%s|%a|%Y|%F`, childPath})
		if statErr != nil {
			// Best-effort: return name only.
			entries = append(entries, FSFileInfo{
				Name: name,
				Path: childPath,
			})
			continue
		}
		fi, parseErr := parseStatLine(childPath, strings.TrimSpace(string(statOut)))
		if parseErr != nil {
			entries = append(entries, FSFileInfo{Name: name, Path: childPath})
			continue
		}
		entries = append(entries, fi)
	}
	return c.JSON(http.StatusOK, FSListResponse{Path: pc.containerPath, Entries: entries})
}

// FSRead godoc
// @Summary Read file content as text
// @Description Reads the content of a file and returns it as a JSON string
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param path query string true "Container file path"
// @Success 200 {object} FSReadResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/read [get]
func (h *ContainerdHandler) FSRead(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	pc, err := h.resolveContainerPath(botID, rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if pc.insideDataMount {
		data, osErr := os.ReadFile(pc.hostPath)
		if osErr != nil {
			if os.IsNotExist(osErr) {
				return echo.NewHTTPError(http.StatusNotFound, "file not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, osErr.Error())
		}
		return c.JSON(http.StatusOK, FSReadResponse{
			Path:    pc.containerPath,
			Content: string(data),
			Size:    int64(len(data)),
		})
	}

	// Exec fallback.
	out, err := h.execRead(botID, []string{"cat", pc.containerPath})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, FSReadResponse{
		Path:    pc.containerPath,
		Content: string(out),
		Size:    int64(len(out)),
	})
}

// FSDownload godoc
// @Summary Download a file as binary stream
// @Description Downloads a file from the container with appropriate Content-Type
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param path query string true "Container file path"
// @Produce octet-stream
// @Success 200 {file} binary
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/download [get]
func (h *ContainerdHandler) FSDownload(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	pc, err := h.resolveContainerPath(botID, rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
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
				return echo.NewHTTPError(http.StatusNotFound, "file not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, osErr.Error())
		}
		if info.IsDir() {
			return echo.NewHTTPError(http.StatusBadRequest, "cannot download a directory")
		}
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
		return c.File(pc.hostPath)
	}

	// Exec fallback: base64 encode inside container, decode on host.
	out, err := h.execRead(botID, []string{"base64", pc.containerPath})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	decoded, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if decErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to decode file content")
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	return c.Blob(http.StatusOK, contentType, decoded)
}

// FSWrite godoc
// @Summary Write text content to a file
// @Description Creates or overwrites a file with the provided text content
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body FSWriteRequest true "Write request"
// @Success 200 {object} fsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/write [post]
func (h *ContainerdHandler) FSWrite(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req FSWriteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Path) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	pc, err := h.resolveContainerPath(botID, req.Path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !pc.insideDataMount {
		return echo.NewHTTPError(http.StatusForbidden, "write operations are only allowed within the data directory")
	}

	dir := filepath.Dir(pc.hostPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := os.WriteFile(pc.hostPath, []byte(req.Content), 0o644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, fsOpResponse{OK: true})
}

// FSUpload godoc
// @Summary Upload a file via multipart form
// @Description Uploads a binary file to the given container path
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param path formData string true "Destination container path"
// @Param file formData file true "File to upload"
// @Accept multipart/form-data
// @Success 200 {object} FSUploadResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/upload [post]
func (h *ContainerdHandler) FSUpload(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	destPath := strings.TrimSpace(c.FormValue("path"))
	if destPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}

	pc, err := h.resolveContainerPath(botID, destPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !pc.insideDataMount {
		return echo.NewHTTPError(http.StatusForbidden, "upload operations are only allowed within the data directory")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer src.Close()

	dir := filepath.Dir(pc.hostPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	dst, err := os.Create(pc.hostPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, FSUploadResponse{
		Path: pc.containerPath,
		Size: written,
	})
}

// FSMkdir godoc
// @Summary Create a directory
// @Description Creates a directory (and parents) at the given container path
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body FSMkdirRequest true "Mkdir request"
// @Success 200 {object} fsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/mkdir [post]
func (h *ContainerdHandler) FSMkdir(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req FSMkdirRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Path) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	pc, err := h.resolveContainerPath(botID, req.Path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !pc.insideDataMount {
		return echo.NewHTTPError(http.StatusForbidden, "mkdir operations are only allowed within the data directory")
	}

	if err := os.MkdirAll(pc.hostPath, 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, fsOpResponse{OK: true})
}

// FSDelete godoc
// @Summary Delete a file or directory
// @Description Deletes a file or directory at the given container path
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body FSDeleteRequest true "Delete request"
// @Success 200 {object} fsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/delete [post]
func (h *ContainerdHandler) FSDelete(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req FSDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Path) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	pc, err := h.resolveContainerPath(botID, req.Path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !pc.insideDataMount {
		return echo.NewHTTPError(http.StatusForbidden, "delete operations are only allowed within the data directory")
	}

	// Prevent deleting the data mount root itself.
	dataMount := strings.TrimSpace(h.cfg.DataMount)
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}
	if filepath.Clean(pc.containerPath) == filepath.Clean(dataMount) {
		return echo.NewHTTPError(http.StatusForbidden, "cannot delete the data root directory")
	}

	if _, statErr := os.Stat(pc.hostPath); os.IsNotExist(statErr) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	if req.Recursive {
		if err := os.RemoveAll(pc.hostPath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	} else {
		if err := os.Remove(pc.hostPath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	return c.JSON(http.StatusOK, fsOpResponse{OK: true})
}

// FSRename godoc
// @Summary Rename or move a file/directory
// @Description Renames or moves a file/directory from oldPath to newPath
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body FSRenameRequest true "Rename request"
// @Success 200 {object} fsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs/rename [post]
func (h *ContainerdHandler) FSRename(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req FSRenameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.OldPath) == "" || strings.TrimSpace(req.NewPath) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "oldPath and newPath are required")
	}

	oldPC, err := h.resolveContainerPath(botID, req.OldPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	newPC, err := h.resolveContainerPath(botID, req.NewPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !oldPC.insideDataMount || !newPC.insideDataMount {
		return echo.NewHTTPError(http.StatusForbidden, "rename operations are only allowed within the data directory")
	}

	if _, statErr := os.Stat(oldPC.hostPath); os.IsNotExist(statErr) {
		return echo.NewHTTPError(http.StatusNotFound, "source not found")
	}

	// Ensure the parent of the destination exists.
	if err := os.MkdirAll(filepath.Dir(newPC.hostPath), 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := os.Rename(oldPC.hostPath, newPC.hostPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, fsOpResponse{OK: true})
}

// ---------- helpers ----------

func osFileInfoToFS(containerPath string, info os.FileInfo) FSFileInfo {
	return FSFileInfo{
		Name:    info.Name(),
		Path:    containerPath,
		Size:    info.Size(),
		Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
		IsDir:   info.IsDir(),
	}
}

// parseStatLine parses output from: stat -c '%n|%s|%a|%Y|%F' /path
func parseStatLine(containerPath, line string) (FSFileInfo, error) {
	parts := strings.SplitN(line, "|", 5)
	if len(parts) < 5 {
		return FSFileInfo{}, fmt.Errorf("unexpected stat output: %s", line)
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

	return FSFileInfo{
		Name:    name,
		Path:    containerPath,
		Size:    size,
		Mode:    mode,
		ModTime: modTime,
		IsDir:   isDir,
	}, nil
}

