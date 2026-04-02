package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const mediaContainerRoot = "/data/media"

// ---------- request / response types ----------

type FSFileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

type FSListResponse struct {
	Path    string       `json:"path"`
	Entries []FSFileInfo `json:"entries"`
}

type FSReadResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
}

type FSUploadResponse struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// FSWriteRequest is the body for creating / overwriting a file.
type FSWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
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

// ---------- helpers ----------

// resolveContainerPath cleans and validates a container-relative path.
func resolveContainerPath(rawPath string) (string, error) {
	cleaned := filepath.Clean("/" + strings.TrimSpace(rawPath))
	if cleaned == "" {
		cleaned = "/"
	}
	if strings.HasPrefix(cleaned, "..") {
		return "", errors.New("invalid path")
	}
	return cleaned, nil
}

func isContainerMediaPath(containerPath string) bool {
	cleaned := filepath.Clean("/" + strings.TrimSpace(containerPath))
	return cleaned == mediaContainerRoot || strings.HasPrefix(cleaned, mediaContainerRoot+"/")
}

// getGRPCClient returns the gRPC client for the bot's container.
func (h *ContainerdHandler) getGRPCClient(ctx context.Context, botID string) (*bridge.Client, error) {
	return h.manager.MCPClient(ctx, botID)
}

// fsFileInfoFromEntry converts a gRPC FileEntry to FSFileInfo.
func fsFileInfoFromEntry(containerPath, name string, isDir bool, size int64, mode, modTime string) FSFileInfo {
	return FSFileInfo{
		Name:    name,
		Path:    filepath.Join(containerPath, name),
		Size:    size,
		Mode:    mode,
		ModTime: modTime,
		IsDir:   isDir,
	}
}

// fsHTTPError maps mcpclient domain errors to HTTP status codes.
func fsHTTPError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, bridge.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, bridge.ErrBadRequest):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, bridge.ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, bridge.ErrUnavailable):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
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
// @Router /bots/{bot_id}/container/fs [get].
func (h *ContainerdHandler) FSStat(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/"
	}

	containerPath, err := resolveContainerPath(rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	entry, err := client.Stat(ctx, containerPath)
	if err != nil {
		return fsHTTPError(err)
	}

	return c.JSON(http.StatusOK, FSFileInfo{
		Name:    filepath.Base(containerPath),
		Path:    containerPath,
		Size:    entry.GetSize(),
		Mode:    entry.GetMode(),
		ModTime: entry.GetModTime(),
		IsDir:   entry.GetIsDir(),
	})
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
// @Router /bots/{bot_id}/container/fs/list [get].
func (h *ContainerdHandler) FSList(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/"
	}

	containerPath, err := resolveContainerPath(rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	entries, err := client.ListDirAll(ctx, containerPath, false)
	if err != nil {
		return fsHTTPError(err)
	}

	fileInfos := make([]FSFileInfo, 0, len(entries))
	for _, e := range entries {
		if e.Path == containerPath {
			continue
		}
		fileInfos = append(fileInfos, fsFileInfoFromEntry(
			containerPath,
			filepath.Base(e.Path),
			e.IsDir,
			e.Size,
			e.Mode,
			e.ModTime,
		))
	}

	return c.JSON(http.StatusOK, FSListResponse{
		Path:    containerPath,
		Entries: fileInfos,
	})
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
// @Router /bots/{bot_id}/container/fs/read [get].
func (h *ContainerdHandler) FSRead(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	containerPath, err := resolveContainerPath(rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	rc, err := client.ReadRaw(ctx, containerPath)
	if err != nil {
		return fsHTTPError(err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}

	return c.JSON(http.StatusOK, FSReadResponse{
		Path:    containerPath,
		Content: string(data),
		Size:    int64(len(data)),
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
// @Router /bots/{bot_id}/container/fs/download [get].
func (h *ContainerdHandler) FSDownload(c echo.Context) error {
	rawPath := c.QueryParam("path")
	if strings.TrimSpace(rawPath) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	containerPath, err := resolveContainerPath(rawPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	requireAccess := h.requireBotAccess
	if isContainerMediaPath(containerPath) {
		requireAccess = h.requireBotAccessWithGuest
	}
	botID, err := requireAccess(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	rc, err := client.ReadRaw(ctx, containerPath)
	if err != nil {
		return fsHTTPError(err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}

	fileName := filepath.Base(containerPath)
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	return c.Blob(http.StatusOK, contentType, data)
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
// @Router /bots/{bot_id}/container/fs/write [post].
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

	containerPath, err := resolveContainerPath(req.Path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	if err := client.WriteFile(ctx, containerPath, []byte(req.Content)); err != nil {
		return fsHTTPError(err)
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
// @Router /bots/{bot_id}/container/fs/upload [post].
func (h *ContainerdHandler) FSUpload(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	destPath := strings.TrimSpace(c.FormValue("path"))
	if destPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path is required")
	}

	containerPath, err := resolveContainerPath(destPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer func() { _ = src.Close() }()

	written, err := client.WriteRaw(ctx, containerPath, src)
	if err != nil {
		return fsHTTPError(err)
	}

	return c.JSON(http.StatusOK, FSUploadResponse{
		Path: containerPath,
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
// @Router /bots/{bot_id}/container/fs/mkdir [post].
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

	containerPath, err := resolveContainerPath(req.Path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	if err := client.Mkdir(ctx, containerPath); err != nil {
		return fsHTTPError(err)
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
// @Router /bots/{bot_id}/container/fs/delete [post].
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

	containerPath, err := resolveContainerPath(req.Path)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if containerPath == "/" {
		return echo.NewHTTPError(http.StatusForbidden, "cannot delete root directory")
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	if err := client.DeleteFile(ctx, containerPath, req.Recursive); err != nil {
		return fsHTTPError(err)
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
// @Router /bots/{bot_id}/container/fs/rename [post].
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

	oldPath, err := resolveContainerPath(req.OldPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	newPath, err := resolveContainerPath(req.NewPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, fmt.Sprintf("container not reachable: %v", err))
	}

	if err := client.Rename(ctx, oldPath, newPath); err != nil {
		return fsHTTPError(err)
	}

	return c.JSON(http.StatusOK, fsOpResponse{OK: true})
}
