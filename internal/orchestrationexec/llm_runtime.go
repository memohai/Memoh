package orchestrationexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	"github.com/memohai/memoh/internal/orchestration"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/settings"
	tzutil "github.com/memohai/memoh/internal/timezone"
)

const (
	LLMWorkerExecutorID   = "llm.workerd"
	LLMVerifierExecutorID = "llm.verifyd"
)

type Runtime struct {
	logger          *slog.Logger
	queries         *sqlc.Queries
	settingsService *settings.Service
	modelsService   *models.Service
	agent           *agentpkg.Agent
	httpClient      *http.Client
	clockLocation   *time.Location
}

type actionLedgerSubject struct {
	runID          pgtype.UUID
	taskID         pgtype.UUID
	attemptID      pgtype.UUID
	verificationID pgtype.UUID
}

type actionLedgerObserver struct {
	queries *sqlc.Queries
	subject actionLedgerSubject
}

func NewRuntime(
	log *slog.Logger,
	queries *sqlc.Queries,
	settingsService *settings.Service,
	modelsService *models.Service,
	agent *agentpkg.Agent,
	clockLocation *time.Location,
) *Runtime {
	if log == nil {
		log = slog.Default()
	}
	if clockLocation == nil {
		clockLocation = time.UTC
	}
	return &Runtime{
		logger:          log.With(slog.String("component", "orchestration_llm_runtime")),
		queries:         queries,
		settingsService: settingsService,
		modelsService:   modelsService,
		agent:           agent,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
			},
		},
		clockLocation: clockLocation,
	}
}

func newActionLedgerObserver(queries *sqlc.Queries, subject actionLedgerSubject) agentpkg.ToolCallObserver {
	if queries == nil {
		return nil
	}
	return &actionLedgerObserver{
		queries: queries,
		subject: subject,
	}
}

func (o *actionLedgerObserver) OnToolCallStart(ctx context.Context, observation agentpkg.ToolCallObservation) error {
	if o == nil || o.queries == nil {
		return nil
	}
	recordID := uuid.New()
	inputPayload, err := marshalJSONValue(observation.Input)
	if err != nil {
		return fmt.Errorf("marshal action input payload: %w", err)
	}
	if o.subject.attemptID.Valid {
		_, err = o.queries.CreateOrchestrationAttemptActionRecord(ctx, sqlc.CreateOrchestrationAttemptActionRecordParams{
			ID:           pgUUIDFromUUID(recordID),
			RunID:        o.subject.runID,
			TaskID:       o.subject.taskID,
			AttemptID:    o.subject.attemptID,
			ActionKind:   "tool_call",
			Status:       "running",
			ToolName:     strings.TrimSpace(observation.ToolName),
			ToolCallID:   strings.TrimSpace(observation.ToolCallID),
			InputPayload: inputPayload,
		})
		if err != nil {
			return fmt.Errorf("create attempt action record: %w", err)
		}
		return nil
	}
	_, err = o.queries.CreateOrchestrationVerificationActionRecord(ctx, sqlc.CreateOrchestrationVerificationActionRecordParams{
		ID:             pgUUIDFromUUID(recordID),
		RunID:          o.subject.runID,
		TaskID:         o.subject.taskID,
		VerificationID: o.subject.verificationID,
		ActionKind:     "tool_call",
		Status:         "running",
		ToolName:       strings.TrimSpace(observation.ToolName),
		ToolCallID:     strings.TrimSpace(observation.ToolCallID),
		InputPayload:   inputPayload,
	})
	if err != nil {
		return fmt.Errorf("create verification action record: %w", err)
	}
	return nil
}

func (o *actionLedgerObserver) OnToolCallFinish(ctx context.Context, observation agentpkg.ToolCallObservation) error {
	if o == nil || o.queries == nil {
		return nil
	}
	status := "completed"
	if observation.Err != nil {
		status = "failed"
	}
	outputPayload, err := marshalJSONValue(observation.Result)
	if err != nil {
		return fmt.Errorf("marshal action output payload: %w", err)
	}
	errorPayload, err := marshalJSONValue(actionErrorPayload(observation.Err))
	if err != nil {
		return fmt.Errorf("marshal action error payload: %w", err)
	}
	summary := summarizeActionObservation(observation)
	if o.subject.attemptID.Valid {
		_, err = o.queries.CompleteOrchestrationAttemptActionRecord(ctx, sqlc.CompleteOrchestrationAttemptActionRecordParams{
			AttemptID:     o.subject.attemptID,
			ToolCallID:    strings.TrimSpace(observation.ToolCallID),
			Status:        status,
			OutputPayload: outputPayload,
			ErrorPayload:  errorPayload,
			Summary:       summary,
		})
		if err != nil {
			return fmt.Errorf("complete attempt action record: %w", err)
		}
		return nil
	}
	_, err = o.queries.CompleteOrchestrationVerificationActionRecord(ctx, sqlc.CompleteOrchestrationVerificationActionRecordParams{
		VerificationID: o.subject.verificationID,
		ToolCallID:     strings.TrimSpace(observation.ToolCallID),
		Status:         status,
		OutputPayload:  outputPayload,
		ErrorPayload:   errorPayload,
		Summary:        summary,
	})
	if err != nil {
		return fmt.Errorf("complete verification action record: %w", err)
	}
	return nil
}

func marshalJSONValue(value any) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func actionErrorPayload(err error) any {
	if err == nil {
		return nil
	}
	return map[string]any{
		"message": err.Error(),
	}
}

func summarizeActionObservation(observation agentpkg.ToolCallObservation) string {
	if observation.Err != nil {
		return truncateSummary(observation.Err.Error(), 240)
	}
	if text := summarizeActionValue(observation.Result); text != "" {
		return truncateSummary(text, 240)
	}
	return strings.TrimSpace(observation.ToolName)
}

func summarizeActionValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if message := strings.TrimSpace(stringValue(typed["message"])); message != "" {
			return message
		}
		if text := strings.TrimSpace(stringValue(typed["text"])); text != "" {
			return text
		}
	case []any:
		if len(typed) == 0 {
			return ""
		}
		return summarizeActionValue(typed[0])
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (r *Runtime) generateWithThinkingTrace(ctx context.Context, cfg agentpkg.RunConfig, recordThinking func(string), recordOutput func(string)) (*agentpkg.GenerateResult, error) {
	var text strings.Builder
	var textChunk strings.Builder
	var reasoning strings.Builder
	var usage *sdk.Usage
	var streamErr error

	flushReasoning := func() {
		delta := strings.TrimSpace(reasoning.String())
		reasoning.Reset()
		if delta == "" || recordThinking == nil {
			return
		}
		recordThinking(delta)
	}
	flushText := func() {
		delta := strings.TrimSpace(textChunk.String())
		textChunk.Reset()
		if delta == "" || recordOutput == nil {
			return
		}
		recordOutput(delta)
	}

	for event := range r.agent.Stream(ctx, cfg) {
		switch event.Type {
		case agentpkg.EventTextDelta:
			text.WriteString(event.Delta)
			textChunk.WriteString(event.Delta)
			if textChunk.Len() >= 600 {
				flushText()
			}
		case agentpkg.EventTextEnd:
			flushText()
		case agentpkg.EventReasoningDelta:
			reasoning.WriteString(event.Delta)
			if reasoning.Len() >= 600 {
				flushReasoning()
			}
		case agentpkg.EventReasoningEnd:
			flushReasoning()
		case agentpkg.EventError:
			if strings.TrimSpace(event.Error) != "" {
				streamErr = errors.New(event.Error)
			}
		case agentpkg.EventAgentAbort:
			if streamErr == nil {
				streamErr = errors.New("agent stream aborted")
			}
		case agentpkg.EventAgentEnd:
			flushText()
			flushReasoning()
			streamErr = nil
			if len(event.Usage) > 0 {
				var parsed sdk.Usage
				if err := json.Unmarshal(event.Usage, &parsed); err == nil {
					usage = &parsed
				}
			}
		}
	}
	flushText()
	flushReasoning()
	if streamErr != nil {
		return nil, streamErr
	}
	return &agentpkg.GenerateResult{
		Text:  text.String(),
		Usage: usage,
	}, nil
}

func (r *Runtime) recordAttemptThinking(ctx context.Context, execCtx attemptExecutionContext, attemptID pgtype.UUID, role, delta string) {
	r.recordAttemptAgentTrace(ctx, execCtx, attemptID, "agent.thinking", role, "reasoning_delta", delta)
}

func (r *Runtime) recordAttemptAgentOutput(ctx context.Context, execCtx attemptExecutionContext, attemptID pgtype.UUID, role, delta string) {
	r.recordAttemptAgentTrace(ctx, execCtx, attemptID, "agent.output", role, "text_delta", delta)
}

func (r *Runtime) recordAttemptAgentTrace(ctx context.Context, execCtx attemptExecutionContext, attemptID pgtype.UUID, toolName, role, eventName, delta string) {
	if r == nil || r.queries == nil || !attemptID.Valid {
		return
	}
	toolCallID, payload := agentTraceRecordPayload(role, eventName, delta)
	if strings.TrimSpace(toolCallID) == "" {
		return
	}
	_, err := r.queries.CreateOrchestrationAttemptActionRecord(ctx, sqlc.CreateOrchestrationAttemptActionRecordParams{
		ID:           pgUUIDFromUUID(uuid.New()),
		RunID:        execCtx.Run.ID,
		TaskID:       execCtx.Task.ID,
		AttemptID:    attemptID,
		ActionKind:   "tool_call",
		Status:       "running",
		ToolName:     toolName,
		ToolCallID:   toolCallID,
		InputPayload: []byte(`{"event":"agent_stream"}`),
	})
	if err != nil {
		r.logger.Warn("create attempt agent trace record failed", slog.Any("error", err))
		return
	}
	_, err = r.queries.CompleteOrchestrationAttemptActionRecord(ctx, sqlc.CompleteOrchestrationAttemptActionRecordParams{
		Status:        "completed",
		OutputPayload: payload,
		ErrorPayload:  []byte("null"),
		Summary:       truncateSummary(delta, 240),
		AttemptID:     attemptID,
		ToolCallID:    toolCallID,
	})
	if err != nil {
		r.logger.Warn("complete attempt agent trace record failed", slog.Any("error", err))
	}
}

func (r *Runtime) recordVerificationThinking(ctx context.Context, execCtx verificationExecutionContext, verificationID pgtype.UUID, role, delta string) {
	r.recordVerificationAgentTrace(ctx, execCtx, verificationID, "agent.thinking", role, "reasoning_delta", delta)
}

func (r *Runtime) recordVerificationAgentOutput(ctx context.Context, execCtx verificationExecutionContext, verificationID pgtype.UUID, role, delta string) {
	r.recordVerificationAgentTrace(ctx, execCtx, verificationID, "agent.output", role, "text_delta", delta)
}

func (r *Runtime) recordVerificationAgentTrace(ctx context.Context, execCtx verificationExecutionContext, verificationID pgtype.UUID, toolName, role, eventName, delta string) {
	if r == nil || r.queries == nil || !verificationID.Valid {
		return
	}
	toolCallID, payload := agentTraceRecordPayload(role, eventName, delta)
	if strings.TrimSpace(toolCallID) == "" {
		return
	}
	_, err := r.queries.CreateOrchestrationVerificationActionRecord(ctx, sqlc.CreateOrchestrationVerificationActionRecordParams{
		ID:             pgUUIDFromUUID(uuid.New()),
		RunID:          execCtx.Run.ID,
		TaskID:         execCtx.Task.ID,
		VerificationID: verificationID,
		ActionKind:     "tool_call",
		Status:         "running",
		ToolName:       toolName,
		ToolCallID:     toolCallID,
		InputPayload:   []byte(`{"event":"agent_stream"}`),
	})
	if err != nil {
		r.logger.Warn("create verification agent trace record failed", slog.Any("error", err))
		return
	}
	_, err = r.queries.CompleteOrchestrationVerificationActionRecord(ctx, sqlc.CompleteOrchestrationVerificationActionRecordParams{
		Status:         "completed",
		OutputPayload:  payload,
		ErrorPayload:   []byte("null"),
		Summary:        truncateSummary(delta, 240),
		VerificationID: verificationID,
		ToolCallID:     toolCallID,
	})
	if err != nil {
		r.logger.Warn("complete verification agent trace record failed", slog.Any("error", err))
	}
}

func agentTraceRecordPayload(role, eventName, delta string) (string, []byte) {
	delta = strings.TrimSpace(delta)
	if delta == "" {
		return "", nil
	}
	payload, err := marshalJSONValue(map[string]any{
		"event": strings.TrimSpace(eventName),
		"role":  strings.TrimSpace(role),
		"delta": delta,
	})
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(eventName) + "-" + uuid.NewString(), payload
}

func truncateSummary(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}

func pgUUIDFromUUID(value uuid.UUID) pgtype.UUID {
	var bytes [16]byte
	copy(bytes[:], value[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}
}

func parseUUIDOrZero(value string) pgtype.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgUUIDFromUUID(parsed)
}

func (r *Runtime) ExecuteAttempt(ctx context.Context, attempt orchestration.TaskAttempt) orchestration.AttemptCompletion {
	completion := orchestration.AttemptCompletion{
		AttemptID:          attempt.ID,
		ClaimToken:         attempt.ClaimToken,
		Status:             orchestration.TaskAttemptStatusFailed,
		Summary:            "orchestration worker failed",
		StructuredOutput:   map[string]any{},
		CompletionMetadata: map[string]any{"executor": LLMWorkerExecutorID},
	}
	execCtx, err := r.loadAttemptExecutionContext(ctx, attempt)
	if err != nil {
		return failAttemptCompletion(completion, "attempt_context_load_failed", err)
	}
	cfg, model, provider, err := r.buildBotRunConfig(ctx, execCtx.BotID, execCtx.Run.OwnerSubject)
	if err != nil {
		return failAttemptCompletion(completion, "worker_model_resolution_failed", err)
	}
	cfg.System = workerSystemPrompt
	cfg.Messages = []sdk.Message{sdk.UserMessage(buildWorkerPrompt(execCtx))}
	cfg.ResponseFormat = &sdk.ResponseFormat{Type: sdk.ResponseFormatJSONObject}
	cfg.ToolCallObserver = newActionLedgerObserver(r.queries, actionLedgerSubject{
		runID:     execCtx.Run.ID,
		taskID:    execCtx.Task.ID,
		attemptID: parseUUIDOrZero(attempt.ID),
	})

	result, err := r.generateWithThinkingTrace(ctx, cfg, func(delta string) {
		r.recordAttemptThinking(ctx, execCtx, parseUUIDOrZero(attempt.ID), "worker", delta)
	}, func(delta string) {
		r.recordAttemptAgentOutput(ctx, execCtx, parseUUIDOrZero(attempt.ID), "worker", delta)
	})
	if err != nil {
		return failAttemptCompletion(completion, "worker_generate_failed", err)
	}
	payload, err := decodeJSONObjectText(result.Text)
	if err != nil {
		return failAttemptCompletion(completion, "worker_response_invalid", err)
	}
	parsed, err := decodeAttemptCompletionPayload(attempt, execCtx.Task, payload)
	if err != nil {
		return failAttemptCompletion(completion, "worker_response_invalid", err)
	}
	if parsed.CompletionMetadata == nil {
		parsed.CompletionMetadata = map[string]any{}
	}
	parsed.CompletionMetadata["executor"] = LLMWorkerExecutorID
	parsed.CompletionMetadata["model_id"] = model.ModelID
	parsed.CompletionMetadata["provider"] = provider.ClientType
	return parsed
}

func (r *Runtime) ExecuteVerification(ctx context.Context, verification orchestration.TaskVerification) orchestration.VerificationCompletion {
	completion := orchestration.VerificationCompletion{
		VerificationID: verification.ID,
		ClaimToken:     verification.ClaimToken,
		Status:         orchestration.TaskVerificationStatusFailed,
		Verdict:        orchestration.VerificationVerdictRejected,
		Summary:        "orchestration verification failed",
		FailureClass:   "verification_failed",
		TerminalReason: "orchestration verification failed",
	}
	execCtx, err := r.loadVerificationExecutionContext(ctx, verification)
	if err != nil {
		return failVerificationCompletion(completion, "verification_context_load_failed", err)
	}
	cfg, _, _, err := r.buildBotRunConfig(ctx, execCtx.BotID, execCtx.Run.OwnerSubject)
	if err != nil {
		return failVerificationCompletion(completion, "verifier_model_resolution_failed", err)
	}
	cfg.System = verifierSystemPrompt
	cfg.Messages = []sdk.Message{sdk.UserMessage(buildVerifierPrompt(execCtx))}
	cfg.ResponseFormat = &sdk.ResponseFormat{Type: sdk.ResponseFormatJSONObject}
	cfg.ToolCallObserver = newActionLedgerObserver(r.queries, actionLedgerSubject{
		runID:          execCtx.Run.ID,
		taskID:         execCtx.Task.ID,
		verificationID: parseUUIDOrZero(verification.ID),
	})

	result, err := r.generateWithThinkingTrace(ctx, cfg, func(delta string) {
		r.recordVerificationThinking(ctx, execCtx, parseUUIDOrZero(verification.ID), "verifier", delta)
	}, func(delta string) {
		r.recordVerificationAgentOutput(ctx, execCtx, parseUUIDOrZero(verification.ID), "verifier", delta)
	})
	if err != nil {
		return failVerificationCompletion(completion, "verifier_generate_failed", err)
	}
	payload, err := decodeJSONObjectText(result.Text)
	if err != nil {
		return failVerificationCompletion(completion, "verifier_response_invalid", err)
	}
	parsed, err := decodeVerificationCompletionPayload(verification, execCtx.Task, execCtx.Result, payload)
	if err != nil {
		return failVerificationCompletion(completion, "verifier_response_invalid", err)
	}
	return parsed
}

type attemptExecutionContext struct {
	BotID         string
	Run           sqlc.OrchestrationRun
	Task          sqlc.OrchestrationTask
	Attempt       orchestration.TaskAttempt
	TaskInputs    map[string]any
	InputManifest map[string]any
	Predecessors  []map[string]any
}

func (r *Runtime) loadAttemptExecutionContext(ctx context.Context, attempt orchestration.TaskAttempt) (attemptExecutionContext, error) {
	attemptTaskID, err := db.ParseUUID(attempt.TaskID)
	if err != nil {
		return attemptExecutionContext{}, fmt.Errorf("invalid task id: %w", err)
	}
	runID, err := db.ParseUUID(attempt.RunID)
	if err != nil {
		return attemptExecutionContext{}, fmt.Errorf("invalid run id: %w", err)
	}
	taskRow, err := r.queries.GetOrchestrationTaskByID(ctx, attemptTaskID)
	if err != nil {
		return attemptExecutionContext{}, fmt.Errorf("load task: %w", err)
	}
	runRow, err := r.queries.GetOrchestrationRunByID(ctx, runID)
	if err != nil {
		return attemptExecutionContext{}, fmt.Errorf("load run: %w", err)
	}
	sourceMetadata := decodeJSONObject(runRow.SourceMetadata)
	botID := strings.TrimSpace(stringValue(sourceMetadata["bot_id"]))
	if botID == "" {
		return attemptExecutionContext{}, errors.New("run source metadata is missing bot_id")
	}

	inputManifest := map[string]any{}
	if attempt.InputManifestID != "" {
		manifestID, manifestErr := db.ParseUUID(attempt.InputManifestID)
		if manifestErr == nil {
			if manifestRow, getErr := r.queries.GetOrchestrationInputManifestByID(ctx, manifestID); getErr == nil {
				inputManifest = map[string]any{
					"id":                            manifestRow.ID.String(),
					"captured_task_inputs":          decodeJSONObject(manifestRow.CapturedTaskInputs),
					"captured_artifact_versions":    decodeJSONArrayObjects(manifestRow.CapturedArtifactVersions),
					"captured_blackboard_revisions": decodeJSONArrayObjects(manifestRow.CapturedBlackboardRevisions),
					"projection_hash":               strings.TrimSpace(manifestRow.ProjectionHash),
				}
			}
		}
	}

	predecessors, err := r.loadPredecessorContexts(ctx, taskRow)
	if err != nil {
		return attemptExecutionContext{}, err
	}
	return attemptExecutionContext{
		BotID:         botID,
		Run:           runRow,
		Task:          taskRow,
		Attempt:       attempt,
		TaskInputs:    decodeJSONObject(taskRow.Inputs),
		InputManifest: inputManifest,
		Predecessors:  predecessors,
	}, nil
}

type verificationExecutionContext struct {
	BotID              string
	Run                sqlc.OrchestrationRun
	Task               sqlc.OrchestrationTask
	Result             sqlc.OrchestrationTaskResult
	Verification       orchestration.TaskVerification
	VerificationPolicy map[string]any
	ResultArtifacts    []map[string]any
}

func (r *Runtime) loadVerificationExecutionContext(ctx context.Context, verification orchestration.TaskVerification) (verificationExecutionContext, error) {
	taskID, err := db.ParseUUID(verification.TaskID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("invalid task id: %w", err)
	}
	runID, err := db.ParseUUID(verification.RunID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("invalid run id: %w", err)
	}
	resultID, err := db.ParseUUID(verification.ResultID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("invalid result id: %w", err)
	}
	taskRow, err := r.queries.GetOrchestrationTaskByID(ctx, taskID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("load task: %w", err)
	}
	runRow, err := r.queries.GetOrchestrationRunByID(ctx, runID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("load run: %w", err)
	}
	resultRow, err := r.queries.GetOrchestrationTaskResultByID(ctx, resultID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("load task result: %w", err)
	}
	sourceMetadata := decodeJSONObject(runRow.SourceMetadata)
	botID := strings.TrimSpace(stringValue(sourceMetadata["bot_id"]))
	if botID == "" {
		return verificationExecutionContext{}, errors.New("run source metadata is missing bot_id")
	}
	artifacts, err := r.queries.ListOrchestrationArtifactsByTask(ctx, taskRow.ID)
	if err != nil {
		return verificationExecutionContext{}, fmt.Errorf("load task artifacts: %w", err)
	}
	return verificationExecutionContext{
		BotID:              botID,
		Run:                runRow,
		Task:               taskRow,
		Result:             resultRow,
		Verification:       verification,
		VerificationPolicy: decodeJSONObject(taskRow.VerificationPolicy),
		ResultArtifacts:    encodeArtifactsForAttempt(artifacts, resultRow.AttemptID),
	}, nil
}

func (r *Runtime) loadPredecessorContexts(ctx context.Context, taskRow sqlc.OrchestrationTask) ([]map[string]any, error) {
	dependencies, err := r.queries.ListActiveOrchestrationTaskDependenciesBySuccessor(ctx, taskRow.ID)
	if err != nil {
		return nil, fmt.Errorf("load predecessor dependencies: %w", err)
	}
	if len(dependencies) == 0 {
		return nil, nil
	}
	tasksByRun, err := r.queries.ListCurrentOrchestrationTasksByRun(ctx, taskRow.RunID)
	if err != nil {
		return nil, fmt.Errorf("load run tasks: %w", err)
	}
	resultsByRun, err := r.queries.ListCurrentOrchestrationTaskResultsByRun(ctx, taskRow.RunID)
	if err != nil {
		return nil, fmt.Errorf("load run results: %w", err)
	}
	artifactsByRun, err := r.queries.ListCurrentOrchestrationArtifactsByRun(ctx, taskRow.RunID)
	if err != nil {
		return nil, fmt.Errorf("load run artifacts: %w", err)
	}

	tasksByID := make(map[string]sqlc.OrchestrationTask, len(tasksByRun))
	for _, candidate := range tasksByRun {
		tasksByID[candidate.ID.String()] = candidate
	}
	resultsByTaskID := make(map[string]sqlc.OrchestrationTaskResult, len(resultsByRun))
	for _, candidate := range resultsByRun {
		resultsByTaskID[candidate.TaskID.String()] = candidate
	}
	artifactsByTaskID := make(map[string][]sqlc.OrchestrationArtifact)
	for _, artifact := range artifactsByRun {
		key := artifact.TaskID.String()
		artifactsByTaskID[key] = append(artifactsByTaskID[key], artifact)
	}

	predecessors := make([]map[string]any, 0, len(dependencies))
	for _, dependency := range dependencies {
		task, ok := tasksByID[dependency.PredecessorTaskID.String()]
		if !ok {
			continue
		}
		item := map[string]any{
			"task_id":          task.ID.String(),
			"goal":             strings.TrimSpace(task.Goal),
			"status":           strings.TrimSpace(task.Status),
			"worker_profile":   strings.TrimSpace(task.WorkerProfile),
			"inputs":           decodeJSONObject(task.Inputs),
			"blackboard_scope": strings.TrimSpace(task.BlackboardScope),
		}
		if result, ok := resultsByTaskID[task.ID.String()]; ok {
			item["result"] = map[string]any{
				"result_id":         result.ID.String(),
				"attempt_id":        pgUUIDString(result.AttemptID),
				"status":            strings.TrimSpace(result.Status),
				"summary":           strings.TrimSpace(result.Summary),
				"failure_class":     strings.TrimSpace(result.FailureClass),
				"request_replan":    result.RequestReplan,
				"artifact_intents":  decodeJSONArrayObjects(result.ArtifactIntents),
				"structured_output": decodeJSONObject(result.StructuredOutput),
			}
			item["artifacts"] = encodeArtifactsForAttempt(artifactsByTaskID[task.ID.String()], result.AttemptID)
		}
		predecessors = append(predecessors, item)
	}
	return predecessors, nil
}

func (r *Runtime) buildBotRunConfig(ctx context.Context, botID, ownerSubject string) (agentpkg.RunConfig, models.GetResponse, sqlc.Provider, error) {
	if r.agent == nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, errors.New("agent is not configured")
	}
	if r.modelsService == nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, errors.New("models service is not configured")
	}
	if r.settingsService == nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, errors.New("settings service is not configured")
	}
	botSettings, err := r.settingsService.GetBot(ctx, botID)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("load bot settings: %w", err)
	}
	modelRef := strings.TrimSpace(botSettings.ChatModelID)
	if modelRef == "" {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, errors.New("bot chat model is not configured")
	}
	model, provider, err := r.resolveChatModel(ctx, modelRef)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, err
	}
	reasoningEffort := ""
	if model.HasCompatibility(models.CompatReasoning) && botSettings.ReasoningEnabled {
		reasoningEffort = strings.TrimSpace(botSettings.ReasoningEffort)
	}
	var reasoningConfig *models.ReasoningConfig
	if reasoningEffort != "" {
		reasoningConfig = &models.ReasoningConfig{Enabled: true, Effort: reasoningEffort}
	}

	credentialsResolver := providers.NewService(nil, r.queries, "")
	authCtx := oauthctx.WithUserID(ctx, ownerSubject)
	creds, err := credentialsResolver.ResolveModelCredentials(authCtx, provider)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("resolve model credentials: %w", err)
	}
	timezoneName, timezoneLocation := r.resolveBotTimezone(ctx, botID)
	return agentpkg.RunConfig{
		Model: models.NewSDKChatModel(models.SDKModelConfig{
			ModelID:         model.ModelID,
			ClientType:      provider.ClientType,
			APIKey:          creds.APIKey,
			CodexAccountID:  creds.CodexAccountID,
			BaseURL:         providers.ProviderConfigString(provider, "base_url"),
			HTTPClient:      r.httpClient,
			ReasoningConfig: reasoningConfig,
		}),
		ReasoningEffort:    reasoningEffort,
		SessionType:        "orchestration",
		SupportsToolCall:   model.HasCompatibility(models.CompatToolCall),
		SupportsImageInput: model.HasCompatibility(models.CompatVision),
		Identity: agentpkg.SessionContext{
			BotID:            botID,
			ChatID:           botID,
			SessionID:        botID,
			Timezone:         timezoneName,
			TimezoneLocation: timezoneLocation,
		},
		LoopDetection: agentpkg.LoopDetectionConfig{Enabled: false},
	}, model, provider, nil
}

func (r *Runtime) resolveChatModel(ctx context.Context, modelRef string) (models.GetResponse, sqlc.Provider, error) {
	var model models.GetResponse
	var err error
	if _, parseErr := db.ParseUUID(modelRef); parseErr == nil {
		model, err = r.modelsService.GetByID(ctx, modelRef)
		if err == nil {
			goto resolved
		}
	}
	model, err = r.modelsService.GetByModelID(ctx, modelRef)
	if err != nil {
		return models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("resolve chat model %q: %w", modelRef, err)
	}

resolved:
	if model.Type != models.ModelTypeChat {
		return models.GetResponse{}, sqlc.Provider{}, errors.New("configured bot chat model is not a chat model")
	}
	provider, err := models.FetchProviderByID(ctx, r.queries, model.ProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("load provider: %w", err)
	}
	return model, provider, nil
}

func (r *Runtime) resolveBotTimezone(ctx context.Context, botID string) (string, *time.Location) {
	if strings.TrimSpace(botID) != "" && r.queries != nil {
		if botUUID, err := db.ParseUUID(botID); err == nil {
			if row, getErr := r.queries.GetBotByID(ctx, botUUID); getErr == nil && row.Timezone.Valid {
				if loc, name, resolveErr := tzutil.Resolve(strings.TrimSpace(row.Timezone.String)); resolveErr == nil {
					return name, loc
				}
			}
		}
	}
	if r.clockLocation != nil {
		return r.clockLocation.String(), r.clockLocation
	}
	return tzutil.DefaultName, tzutil.MustResolve(tzutil.DefaultName)
}

const workerSystemPrompt = `You are the execution runtime for a single orchestration task.

Complete only the current task. Use tools when needed, especially container file and command tools. Prefer real execution over guessing.

For human-readable fields such as summary, terminal_reason, artifact summaries, and child task goals, use the same language as the run goal / task goal / user request. Preserve Chinese when the request is Chinese.

Return exactly one JSON object and no markdown. The JSON shape must be:
{
  "status": "completed" | "failed",
  "summary": string,
  "failure_class": string,
  "terminal_reason": string,
  "request_replan": boolean,
  "artifact_intents": [
    {
      "kind": string,
      "uri": string,
      "version": string,
      "digest": string,
      "content_type": string,
      "summary": string,
      "metadata": object
    }
  ],
  "structured_output": object
}

When requesting replan, use status="completed", set "request_replan": true, and put replacement tasks in structured_output.child_tasks.
Do not use status="failed" for a replan-only handoff, or the runtime will treat the task as terminal failure instead of expanding the DAG.
Each child task must be an object with:
{
  "alias": string,
  "kind": string,
  "goal": string,
  "inputs": object,
  "depends_on": string[],
  "worker_profile": string,
  "priority": number,
  "retry_policy": object,
  "verification_policy": object,
  "blackboard_scope": string
}

If the task succeeds, use status="completed". If it cannot be completed, use status="failed" with clear failure_class and terminal_reason.`

const verifierSystemPrompt = `You are the verification runtime for a single orchestration task result.

Inspect the task goal, verification policy, produced structured output, and artifacts. Use tools only when necessary to validate the result.

For human-readable fields such as summary and terminal_reason, use the same language as the task goal / produced result / user request. Preserve Chinese when the request is Chinese.

Return exactly one JSON object and no markdown. The JSON shape must be:
{
  "status": "completed" | "failed",
  "verdict": "accepted" | "rejected",
  "summary": string,
  "failure_class": string,
  "terminal_reason": string,
  "request_replan": boolean
}

Use status="completed", verdict="accepted" when the result is valid.
Use verdict="rejected" when validation fails.
Only set request_replan=true when the existing task result already contains structured_output.child_tasks that should replace the current subtree.`

func buildWorkerPrompt(execCtx attemptExecutionContext) string {
	payload := map[string]any{
		"run": map[string]any{
			"run_id":           execCtx.Run.ID.String(),
			"goal":             strings.TrimSpace(execCtx.Run.Goal),
			"planner_epoch":    execCtx.Run.PlannerEpoch,
			"lifecycle_status": strings.TrimSpace(execCtx.Run.LifecycleStatus),
			"source_metadata":  decodeJSONObject(execCtx.Run.SourceMetadata),
			"requested_output": decodeJSONObject(execCtx.Run.OutputSchema),
			"run_input":        decodeJSONObject(execCtx.Run.Input),
			"run_policies":     decodeJSONObject(execCtx.Run.Policies),
			"control_policy":   decodeJSONObject(execCtx.Run.ControlPolicy),
		},
		"task": map[string]any{
			"task_id":             execCtx.Task.ID.String(),
			"goal":                strings.TrimSpace(execCtx.Task.Goal),
			"kind":                strings.TrimSpace(execCtx.Task.Kind),
			"worker_profile":      strings.TrimSpace(execCtx.Task.WorkerProfile),
			"priority":            execCtx.Task.Priority,
			"inputs":              execCtx.TaskInputs,
			"retry_policy":        decodeJSONObject(execCtx.Task.RetryPolicy),
			"verification_policy": decodeJSONObject(execCtx.Task.VerificationPolicy),
			"blackboard_scope":    strings.TrimSpace(execCtx.Task.BlackboardScope),
			"planner_epoch":       execCtx.Task.PlannerEpoch,
		},
		"attempt": map[string]any{
			"attempt_id":          execCtx.Attempt.ID,
			"attempt_no":          execCtx.Attempt.AttemptNo,
			"input_manifest":      execCtx.InputManifest,
			"predecessor_results": execCtx.Predecessors,
		},
	}
	return "Execute the following orchestration task.\n\nContext JSON:\n" + mustJSON(payload)
}

func buildVerifierPrompt(execCtx verificationExecutionContext) string {
	payload := map[string]any{
		"run": map[string]any{
			"run_id":           execCtx.Run.ID.String(),
			"goal":             strings.TrimSpace(execCtx.Run.Goal),
			"planner_epoch":    execCtx.Run.PlannerEpoch,
			"lifecycle_status": strings.TrimSpace(execCtx.Run.LifecycleStatus),
		},
		"task": map[string]any{
			"task_id":             execCtx.Task.ID.String(),
			"goal":                strings.TrimSpace(execCtx.Task.Goal),
			"worker_profile":      strings.TrimSpace(execCtx.Task.WorkerProfile),
			"verification_policy": execCtx.VerificationPolicy,
		},
		"result": map[string]any{
			"result_id":         execCtx.Result.ID.String(),
			"attempt_id":        pgUUIDString(execCtx.Result.AttemptID),
			"status":            strings.TrimSpace(execCtx.Result.Status),
			"summary":           strings.TrimSpace(execCtx.Result.Summary),
			"failure_class":     strings.TrimSpace(execCtx.Result.FailureClass),
			"request_replan":    execCtx.Result.RequestReplan,
			"artifact_intents":  decodeJSONArrayObjects(execCtx.Result.ArtifactIntents),
			"structured_output": decodeJSONObject(execCtx.Result.StructuredOutput),
			"artifacts":         execCtx.ResultArtifacts,
		},
		"verification": map[string]any{
			"verification_id":  execCtx.Verification.ID,
			"attempt_no":       execCtx.Verification.AttemptNo,
			"verifier_profile": strings.TrimSpace(execCtx.Verification.VerifierProfile),
		},
	}
	return "Verify the following orchestration task result.\n\nContext JSON:\n" + mustJSON(payload)
}

func decodeAttemptCompletionPayload(attempt orchestration.TaskAttempt, taskRow sqlc.OrchestrationTask, payload map[string]any) (orchestration.AttemptCompletion, error) {
	status := normalizeAttemptStatus(payload["status"])
	if status == "" {
		return orchestration.AttemptCompletion{}, errors.New("worker response is missing a valid status")
	}
	structuredOutput := normalizeObject(mapValue(payload["structured_output"]))
	if childTasks, ok := payload["child_tasks"].([]any); ok && len(childTasks) > 0 {
		structuredOutput["child_tasks"] = childTasks
	}
	summary := strings.TrimSpace(stringValue(payload["summary"]))
	if summary == "" {
		summary = strings.TrimSpace(taskRow.Goal)
	}
	terminalReason := strings.TrimSpace(stringValue(payload["terminal_reason"]))
	if status == orchestration.TaskAttemptStatusFailed && terminalReason == "" {
		terminalReason = summary
	}
	return orchestration.AttemptCompletion{
		AttemptID:        attempt.ID,
		ClaimToken:       attempt.ClaimToken,
		Status:           status,
		Summary:          summary,
		StructuredOutput: structuredOutput,
		FailureClass:     strings.TrimSpace(stringValue(payload["failure_class"])),
		TerminalReason:   terminalReason,
		RequestReplan:    boolValue(payload["request_replan"]),
		ArtifactIntents:  decodeAttemptArtifactIntentsFromAny(payload["artifact_intents"]),
	}, nil
}

func decodeVerificationCompletionPayload(
	verification orchestration.TaskVerification,
	taskRow sqlc.OrchestrationTask,
	resultRow sqlc.OrchestrationTaskResult,
	payload map[string]any,
) (orchestration.VerificationCompletion, error) {
	status := normalizeVerificationStatus(payload["status"])
	if status == "" {
		return orchestration.VerificationCompletion{}, errors.New("verifier response is missing a valid status")
	}
	verdict := normalizeVerificationVerdict(payload["verdict"])
	if verdict == "" {
		return orchestration.VerificationCompletion{}, errors.New("verifier response is missing a valid verdict")
	}
	summary := strings.TrimSpace(stringValue(payload["summary"]))
	if summary == "" {
		summary = strings.TrimSpace(resultRow.Summary)
	}
	if summary == "" {
		summary = strings.TrimSpace(taskRow.Goal)
	}
	terminalReason := strings.TrimSpace(stringValue(payload["terminal_reason"]))
	if verdict == orchestration.VerificationVerdictRejected && terminalReason == "" {
		terminalReason = summary
	}
	return orchestration.VerificationCompletion{
		VerificationID: verification.ID,
		ClaimToken:     verification.ClaimToken,
		Status:         status,
		Verdict:        verdict,
		Summary:        summary,
		FailureClass:   strings.TrimSpace(stringValue(payload["failure_class"])),
		TerminalReason: terminalReason,
		RequestReplan:  boolValue(payload["request_replan"]),
	}, nil
}

func decodeAttemptArtifactIntentsFromAny(raw any) []orchestration.AttemptArtifactIntent {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	intents := make([]orchestration.AttemptArtifactIntent, 0, len(items))
	for _, item := range items {
		payload := normalizeObject(mapValue(item))
		if len(payload) == 0 {
			continue
		}
		intents = append(intents, orchestration.AttemptArtifactIntent{
			Kind:        strings.TrimSpace(stringValue(payload["kind"])),
			URI:         strings.TrimSpace(stringValue(payload["uri"])),
			Version:     strings.TrimSpace(stringValue(payload["version"])),
			Digest:      strings.TrimSpace(stringValue(payload["digest"])),
			ContentType: strings.TrimSpace(stringValue(payload["content_type"])),
			Summary:     strings.TrimSpace(stringValue(payload["summary"])),
			Metadata:    normalizeObject(mapValue(payload["metadata"])),
		})
	}
	if len(intents) == 0 {
		return nil
	}
	return intents
}

func encodeArtifactsForAttempt(artifacts []sqlc.OrchestrationArtifact, attemptID pgtype.UUID) []map[string]any {
	filtered := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		if !attemptID.Valid {
			continue
		}
		if !artifact.AttemptID.Valid || artifact.AttemptID.String() != attemptID.String() {
			continue
		}
		createdAt := ""
		if artifact.CreatedAt.Valid {
			createdAt = artifact.CreatedAt.Time.UTC().Format(time.RFC3339Nano)
		}
		filtered = append(filtered, map[string]any{
			"artifact_id":  artifact.ID.String(),
			"kind":         strings.TrimSpace(artifact.Kind),
			"uri":          strings.TrimSpace(artifact.Uri),
			"version":      strings.TrimSpace(artifact.Version),
			"digest":       strings.TrimSpace(artifact.Digest),
			"content_type": strings.TrimSpace(artifact.ContentType),
			"summary":      strings.TrimSpace(artifact.Summary),
			"metadata":     decodeJSONObject(artifact.Metadata),
			"created_at":   createdAt,
		})
	}
	return filtered
}

func decodeJSONObjectText(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("empty model response")
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```JSON")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("decode json response: %w", err)
	}
	return normalizeObject(payload), nil
}

func decodeJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{}
	}
	return normalizeObject(value)
}

func decodeJSONArrayObjects(raw []byte) []map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return []map[string]any{}
	}
	var value []map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return []map[string]any{}
	}
	for i := range value {
		value[i] = normalizeObject(value[i])
	}
	return value
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func failAttemptCompletion(base orchestration.AttemptCompletion, failureClass string, err error) orchestration.AttemptCompletion {
	base.Status = orchestration.TaskAttemptStatusFailed
	base.FailureClass = strings.TrimSpace(failureClass)
	base.TerminalReason = err.Error()
	base.Summary = err.Error()
	if base.StructuredOutput == nil {
		base.StructuredOutput = map[string]any{}
	}
	return base
}

func failVerificationCompletion(base orchestration.VerificationCompletion, failureClass string, err error) orchestration.VerificationCompletion {
	base.Status = orchestration.TaskVerificationStatusFailed
	base.Verdict = orchestration.VerificationVerdictRejected
	base.FailureClass = strings.TrimSpace(failureClass)
	base.TerminalReason = err.Error()
	base.Summary = err.Error()
	return base
}

func normalizeAttemptStatus(raw any) string {
	switch strings.TrimSpace(stringValue(raw)) {
	case "", orchestration.TaskAttemptStatusCompleted:
		return orchestration.TaskAttemptStatusCompleted
	case orchestration.TaskAttemptStatusFailed:
		return orchestration.TaskAttemptStatusFailed
	default:
		return ""
	}
}

func normalizeVerificationStatus(raw any) string {
	switch strings.TrimSpace(stringValue(raw)) {
	case "", orchestration.TaskVerificationStatusCompleted:
		return orchestration.TaskVerificationStatusCompleted
	case orchestration.TaskVerificationStatusFailed:
		return orchestration.TaskVerificationStatusFailed
	default:
		return ""
	}
}

func normalizeVerificationVerdict(raw any) string {
	switch strings.TrimSpace(stringValue(raw)) {
	case orchestration.VerificationVerdictAccepted:
		return orchestration.VerificationVerdictAccepted
	case orchestration.VerificationVerdictRejected:
		return orchestration.VerificationVerdictRejected
	default:
		return ""
	}
}

func normalizeObject(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	normalized := make(map[string]any, len(value))
	for key, item := range value {
		normalized[key] = normalizeValue(item)
	}
	return normalized
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeObject(typed)
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, normalizeValue(item))
		}
		return items
	default:
		return typed
	}
}

func pgUUIDString(value interface{ String() string }) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(value.String())
}

func mapValue(raw any) map[string]any {
	value, _ := raw.(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func boolValue(raw any) bool {
	value, _ := raw.(bool)
	return value
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}
