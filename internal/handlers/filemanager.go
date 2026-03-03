package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	fsops "github.com/memohai/memoh/internal/fs"
)

// ---------- request / response types ----------

type FSFileInfo = fsops.FileInfo
type FSListResponse = fsops.ListResult
type FSReadResponse = fsops.ReadResult
type FSUploadResponse = fsops.UploadResult

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
	fi, err := h.fsService.Stat(c.Request().Context(), botID, c.QueryParam("path"))
	if err != nil {
		return h.toFSHTTPError(err)
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
	resp, err := h.fsService.List(c.Request().Context(), botID, c.QueryParam("path"))
	if err != nil {
		return h.toFSHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
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
	resp, err := h.fsService.Read(c.Request().Context(), botID, c.QueryParam("path"))
	if err != nil {
		return h.toFSHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
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
	resp, err := h.fsService.Download(c.Request().Context(), botID, c.QueryParam("path"))
	if err != nil {
		return h.toFSHTTPError(err)
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, resp.FileName))
	if resp.FromHost {
		return c.File(resp.HostPath)
	}
	return c.Blob(http.StatusOK, resp.ContentType, resp.Data)
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
	if err := h.fsService.Write(botID, req.Path, req.Content); err != nil {
		return h.toFSHTTPError(err)
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
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer src.Close()
	resp, err := h.fsService.Upload(botID, destPath, src)
	if err != nil {
		return h.toFSHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
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
	if err := h.fsService.Mkdir(botID, req.Path); err != nil {
		return h.toFSHTTPError(err)
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
	if err := h.fsService.Delete(botID, req.Path, req.Recursive); err != nil {
		return h.toFSHTTPError(err)
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
	if err := h.fsService.Rename(botID, req.OldPath, req.NewPath); err != nil {
		return h.toFSHTTPError(err)
	}
	return c.JSON(http.StatusOK, fsOpResponse{OK: true})
}

func (h *ContainerdHandler) toFSHTTPError(err error) error {
	if fsErr, ok := fsops.AsError(err); ok {
		return echo.NewHTTPError(fsErr.Code, fsErr.Message)
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}
