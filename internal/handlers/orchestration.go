package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/orchestration"
)

type orchestrationAPI interface {
	StartRun(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error)
	GetRunSnapshot(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error)
	GetRunSnapshotAtSeq(context.Context, orchestration.ControlIdentity, string, uint64) (*orchestration.RunSnapshot, error)
	ListRunTasks(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error)
	CreateHumanCheckpoint(context.Context, orchestration.ControlIdentity, orchestration.CreateHumanCheckpointRequest) (*orchestration.CreateHumanCheckpointResult, error)
	ListRunCheckpoints(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error)
	ListRunArtifacts(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunArtifactsRequest) (*orchestration.ArtifactPage, error)
	ListRunEvents(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error)
	ResolveCheckpoint(context.Context, orchestration.ControlIdentity, string, orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error)
}

type OrchestrationHandler struct {
	service orchestrationAPI
	logger  *slog.Logger
}

func NewOrchestrationHandler(log *slog.Logger, service *orchestration.Service) *OrchestrationHandler {
	return &OrchestrationHandler{
		service: service,
		logger:  log.With(slog.String("handler", "orchestration")),
	}
}

func (h *OrchestrationHandler) Register(e *echo.Echo) {
	group := e.Group("/orchestration")
	group.POST("/runs", h.StartRun)
	group.GET("/runs/:run_id/snapshot", h.GetRunSnapshot)
	group.GET("/runs/:run_id/tasks", h.ListRunTasks)
	group.POST("/runs/:run_id/tasks/:task_id/checkpoints", h.CreateHumanCheckpoint)
	group.GET("/runs/:run_id/checkpoints", h.ListRunCheckpoints)
	group.GET("/runs/:run_id/artifacts", h.ListRunArtifacts)
	group.GET("/runs/:run_id/events", h.ListRunEvents)
	group.POST("/checkpoints/:checkpoint_id/resolve", h.ResolveCheckpoint)
}

// StartRun godoc
// @Summary Start an orchestration run
// @Description Create a new orchestration run under the authenticated user
// @Tags orchestration
// @Security BearerAuth
// @Param payload body orchestration.StartRunRequest true "Start run request"
// @Success 201 {object} orchestration.RunHandle
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs [post].
func (h *OrchestrationHandler) StartRun(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	var req orchestration.StartRunRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	handle, err := h.service.StartRun(c.Request().Context(), caller, req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusCreated, handle)
}

// GetRunSnapshot godoc
// @Summary Get orchestration run snapshot
// @Description Return the current run aggregate snapshot
// @Tags orchestration
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param as_of_seq query int false "Committed snapshot sequence"
// @Success 200 {object} orchestration.RunSnapshot
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs/{run_id}/snapshot [get].
func (h *OrchestrationHandler) GetRunSnapshot(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	asOfSeq, err := parseUint64Query(c, "as_of_seq")
	if err != nil {
		return err
	}
	runID := strings.TrimSpace(c.Param("run_id"))
	var snapshot *orchestration.RunSnapshot
	if asOfSeq == 0 {
		snapshot, err = h.service.GetRunSnapshot(c.Request().Context(), caller, runID)
	} else {
		snapshot, err = h.service.GetRunSnapshotAtSeq(c.Request().Context(), caller, runID, asOfSeq)
	}
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, snapshot)
}

// ListRunTasks godoc
// @Summary List orchestration tasks
// @Description List tasks for a run at a committed snapshot sequence
// @Tags orchestration
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param status query string false "Comma-separated task statuses"
// @Param after query string false "Opaque pagination cursor"
// @Param limit query int false "Page size"
// @Param as_of_seq query int false "Committed snapshot sequence"
// @Success 200 {object} orchestration.TaskPage
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs/{run_id}/tasks [get].
func (h *OrchestrationHandler) ListRunTasks(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	asOfSeq, err := parseUint64Query(c, "as_of_seq")
	if err != nil {
		return err
	}
	req := orchestration.ListRunTasksRequest{
		Status:  splitCSVQuery(c.QueryParam("status")),
		After:   strings.TrimSpace(c.QueryParam("after")),
		AsOfSeq: asOfSeq,
	}
	req.Limit, err = parsePositiveIntQuery(c, "limit")
	if err != nil {
		return err
	}
	page, err := h.service.ListRunTasks(c.Request().Context(), caller, strings.TrimSpace(c.Param("run_id")), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, page)
}

// CreateHumanCheckpoint godoc
// @Summary Create a human checkpoint for a task
// @Description Create an open HITL checkpoint on a run task for the authenticated user
// @Tags orchestration
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param task_id path string true "Task ID"
// @Param payload body orchestration.CreateHumanCheckpointRequest true "Checkpoint definition"
// @Success 201 {object} orchestration.CreateHumanCheckpointResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs/{run_id}/tasks/{task_id}/checkpoints [post].
func (h *OrchestrationHandler) CreateHumanCheckpoint(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	runID := strings.TrimSpace(c.Param("run_id"))
	taskID := strings.TrimSpace(c.Param("task_id"))
	var req orchestration.CreateHumanCheckpointRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.RunID) != "" && strings.TrimSpace(req.RunID) != runID {
		return echo.NewHTTPError(http.StatusBadRequest, "body run_id must match path run_id")
	}
	if strings.TrimSpace(req.TaskID) != "" && strings.TrimSpace(req.TaskID) != taskID {
		return echo.NewHTTPError(http.StatusBadRequest, "body task_id must match path task_id")
	}
	req.RunID = runID
	req.TaskID = taskID
	result, err := h.service.CreateHumanCheckpoint(c.Request().Context(), caller, req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusCreated, result)
}

// ListRunCheckpoints godoc
// @Summary List human checkpoints
// @Description List checkpoints for a run at a committed snapshot sequence
// @Tags orchestration
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param status query string false "Comma-separated checkpoint statuses"
// @Param after query string false "Opaque pagination cursor"
// @Param limit query int false "Page size"
// @Param as_of_seq query int false "Committed snapshot sequence"
// @Success 200 {object} orchestration.HumanCheckpointPage
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs/{run_id}/checkpoints [get].
func (h *OrchestrationHandler) ListRunCheckpoints(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	asOfSeq, err := parseUint64Query(c, "as_of_seq")
	if err != nil {
		return err
	}
	req := orchestration.ListRunCheckpointsRequest{
		Status: splitCSVQuery(c.QueryParam("status")),
		After:  strings.TrimSpace(c.QueryParam("after")),
	}
	req.Limit, err = parsePositiveIntQuery(c, "limit")
	if err != nil {
		return err
	}
	req.AsOfSeq = asOfSeq
	page, err := h.service.ListRunCheckpoints(c.Request().Context(), caller, strings.TrimSpace(c.Param("run_id")), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, page)
}

// ListRunArtifacts godoc
// @Summary List orchestration artifacts
// @Description List artifact projections for a run at a committed snapshot sequence
// @Tags orchestration
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param task_id query string false "Filter by task ID"
// @Param kind query string false "Comma-separated artifact kinds"
// @Param after query string false "Opaque pagination cursor"
// @Param limit query int false "Page size"
// @Param as_of_seq query int false "Committed snapshot sequence"
// @Success 200 {object} orchestration.ArtifactPage
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs/{run_id}/artifacts [get].
func (h *OrchestrationHandler) ListRunArtifacts(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	asOfSeq, err := parseUint64Query(c, "as_of_seq")
	if err != nil {
		return err
	}
	req := orchestration.ListRunArtifactsRequest{
		TaskID: strings.TrimSpace(c.QueryParam("task_id")),
		Kind:   splitCSVQuery(c.QueryParam("kind")),
		After:  strings.TrimSpace(c.QueryParam("after")),
	}
	req.Limit, err = parsePositiveIntQuery(c, "limit")
	if err != nil {
		return err
	}
	req.AsOfSeq = asOfSeq
	page, err := h.service.ListRunArtifacts(c.Request().Context(), caller, strings.TrimSpace(c.Param("run_id")), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, page)
}

// ListRunEvents godoc
// @Summary List committed orchestration events
// @Description List committed run events after a sequence cursor
// @Tags orchestration
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param after_seq query int false "Return events with seq greater than this value"
// @Param limit query int false "Maximum number of events"
// @Param until_seq query int false "Upper committed sequence bound for stable replay"
// @Success 200 {object} orchestration.RunEventPage
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/runs/{run_id}/events [get].
func (h *OrchestrationHandler) ListRunEvents(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	afterSeq, err := parseUint64Query(c, "after_seq")
	if err != nil {
		return err
	}
	untilSeq, err := parseUint64Query(c, "until_seq")
	if err != nil {
		return err
	}
	req := orchestration.ListRunEventsRequest{
		AfterSeq: afterSeq,
		UntilSeq: untilSeq,
	}
	req.Limit, err = parsePositiveIntQuery(c, "limit")
	if err != nil {
		return err
	}
	page, err := h.service.ListRunEvents(c.Request().Context(), caller, strings.TrimSpace(c.Param("run_id")), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, page)
}

// ResolveCheckpoint godoc
// @Summary Resolve a human checkpoint
// @Description Resolve an open checkpoint using a committed idempotent resolution
// @Tags orchestration
// @Security BearerAuth
// @Param checkpoint_id path string true "Checkpoint ID"
// @Param payload body orchestration.CheckpointResolution true "Checkpoint resolution"
// @Success 200 {object} orchestration.ResolveCheckpointResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /orchestration/checkpoints/{checkpoint_id}/resolve [post].
func (h *OrchestrationHandler) ResolveCheckpoint(c echo.Context) error {
	caller, err := controlIdentity(c)
	if err != nil {
		return err
	}
	var req orchestration.CheckpointResolution
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	result, err := h.service.ResolveCheckpoint(c.Request().Context(), caller, strings.TrimSpace(c.Param("checkpoint_id")), req)
	if err != nil {
		return h.httpError(err)
	}
	return c.JSON(http.StatusOK, result)
}

func controlIdentity(c echo.Context) (orchestration.ControlIdentity, error) {
	if strings.TrimSpace(c.QueryParam("token")) != "" {
		return orchestration.ControlIdentity{}, echo.NewHTTPError(http.StatusUnauthorized, "authorization header required")
	}
	tokenType, err := auth.TokenTypeFromContext(c)
	if err != nil {
		return orchestration.ControlIdentity{}, err
	}
	if tokenType != "" {
		return orchestration.ControlIdentity{}, echo.NewHTTPError(http.StatusUnauthorized, "user token required")
	}
	tenantID, err := auth.TenantIDFromContext(c)
	if err != nil {
		return orchestration.ControlIdentity{}, err
	}
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return orchestration.ControlIdentity{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	userID = strings.TrimSpace(userID)
	if tenantID == "" {
		return orchestration.ControlIdentity{}, echo.NewHTTPError(http.StatusUnauthorized, "tenant id missing")
	}
	if userID == "" {
		return orchestration.ControlIdentity{}, echo.NewHTTPError(http.StatusUnauthorized, "user id missing")
	}
	return orchestration.ControlIdentity{
		TenantID: tenantID,
		Subject:  userID,
	}, nil
}

func (h *OrchestrationHandler) httpError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, orchestration.ErrInvalidControlIdentity),
		errors.Is(err, orchestration.ErrInvalidArgument),
		errors.Is(err, orchestration.ErrInvalidCursor),
		errors.Is(err, orchestration.ErrInvalidCheckpointResolution):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, orchestration.ErrAccessDenied):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, orchestration.ErrRunNotFound),
		errors.Is(err, orchestration.ErrTaskNotFound),
		errors.Is(err, orchestration.ErrCheckpointNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, orchestration.ErrIdempotencyConflict):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, orchestration.ErrIdempotencyIncomplete):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, orchestration.ErrRunImmutable),
		errors.Is(err, orchestration.ErrTaskImmutable),
		errors.Is(err, orchestration.ErrTaskCheckpointUnsupported),
		errors.Is(err, orchestration.ErrTaskAlreadyWaitingHuman),
		errors.Is(err, orchestration.ErrRunBarrierAlreadyOpen),
		errors.Is(err, orchestration.ErrRunBarrierUnsupported),
		errors.Is(err, orchestration.ErrCheckpointNotOpen):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	default:
		h.logger.Error("orchestration handler error", slog.String("error", err.Error()))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal orchestration error")
	}
}

func splitCSVQuery(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func parseUint64Query(c echo.Context, key string) (uint64, error) {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, key+" must be an unsigned integer")
	}
	return value, nil
}

func parsePositiveIntQuery(c echo.Context, key string) (int, error) {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, key+" must be a positive integer")
	}
	return value, nil
}
