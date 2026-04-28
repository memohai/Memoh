package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/orchestration"
)

type attemptRuntime interface {
	HeartbeatAttempt(context.Context, orchestration.AttemptHeartbeat) (*orchestration.TaskAttempt, error)
	CompleteAttempt(context.Context, orchestration.AttemptCompletion) (*orchestration.TaskAttempt, error)
}

type attemptExecutor func(context.Context, orchestration.TaskAttempt, []string) orchestration.AttemptCompletion

const completionRetryInterval = 250 * time.Millisecond

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "workerd: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()

	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Init(cfg.Log.Level, cfg.Log.Format)
	log := logger.L.With(slog.String("component", "workerd"))

	pool, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	queries := dbsqlc.New(pool)
	svc := orchestration.NewService(log, pool, queries)

	workerID := strings.TrimSpace(os.Getenv("WORKER_ID"))
	if workerID == "" {
		workerID = "workerd-" + uuid.NewString()
	}
	executorID := strings.TrimSpace(os.Getenv("WORKER_EXECUTOR_ID"))
	if executorID == "" {
		executorID = orchestration.DefaultWorkerExecutorID
	}
	workerProfiles := envCSV("WORKER_PROFILES", []string{orchestration.DefaultRootWorkerProfile})
	leaseTTLSeconds := envInt("WORKER_LEASE_TTL_SECONDS", 30)
	pollInterval := time.Duration(envInt("WORKER_POLL_INTERVAL_MS", 500)) * time.Millisecond

	workerLease, err := svc.RegisterWorker(ctx, orchestration.WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      executorID,
		DisplayName:     workerID,
		Capabilities:    map[string]any{"worker_profiles": stringSliceToAny(workerProfiles)},
		LeaseTTLSeconds: leaseTTLSeconds,
	})
	if err != nil {
		return fmt.Errorf("worker registration failed: %w", err)
	}
	workerLeaseToken := workerLease.LeaseToken

	go runWorkerHeartbeatLoop(ctx, svc, log, workerID, workerLeaseToken, leaseTTLSeconds, cancel)

	for {
		if ctx.Err() != nil {
			return nil
		}

		attempt, err := svc.ClaimNextAttempt(ctx, orchestration.AttemptClaim{
			WorkerID:        workerID,
			ExecutorID:      executorID,
			WorkerProfiles:  workerProfiles,
			LeaseToken:      workerLeaseToken,
			LeaseTTLSeconds: leaseTTLSeconds,
		})
		if err != nil {
			if errors.Is(err, orchestration.ErrWorkerLeaseConflict) {
				log.Error("worker lease lost; stopping worker", slog.String("worker_id", workerID))
				return nil
			}
			if errors.Is(err, orchestration.ErrNoRunnableAttempt) {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(pollInterval):
					continue
				}
			}
			log.Error("claim attempt failed", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(pollInterval):
				continue
			}
		}

		if err := sleepWithContext(ctx, time.Duration(envInt("WORKER_START_DELAY_MS", 0))*time.Millisecond); err != nil {
			return nil
		}
		runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
		if err != nil {
			log.Error("start attempt failed", slog.String("attempt_id", attempt.ID), slog.Any("error", err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(pollInterval):
				continue
			}
		}

		leaseLost := runAttempt(ctx, svc, log, *runningAttempt, leaseTTLSeconds, workerProfiles, func(execCtx context.Context, attempt orchestration.TaskAttempt, profiles []string) orchestration.AttemptCompletion {
			return executeAttempt(execCtx, queries, attempt, profiles)
		})
		if leaseLost {
			log.Warn("dropping stale attempt completion after lease loss", slog.String("attempt_id", runningAttempt.ID))
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func runAttempt(ctx context.Context, svc attemptRuntime, log *slog.Logger, attempt orchestration.TaskAttempt, leaseTTLSeconds int, workerProfiles []string, execute attemptExecutor) bool {
	return runAttemptWithInterval(ctx, svc, log, attempt, leaseTTLSeconds, heartbeatInterval(leaseTTLSeconds), workerProfiles, execute)
}

func runAttemptWithInterval(ctx context.Context, svc attemptRuntime, log *slog.Logger, attempt orchestration.TaskAttempt, leaseTTLSeconds int, heartbeatEvery time.Duration, workerProfiles []string, execute attemptExecutor) bool {
	execCtx, cancelExec := context.WithCancel(ctx)
	defer cancelExec()
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	attemptHeartbeatDone := make(chan bool, 1)
	go runAttemptHeartbeatLoopWithInterval(heartbeatCtx, cancelExec, svc, log, attempt, leaseTTLSeconds, heartbeatEvery, attemptHeartbeatDone)

	completion := execute(execCtx, attempt, workerProfiles)

	heartbeatResultRead := false
	checkHeartbeat := func(block bool) (bool, bool) {
		if heartbeatResultRead {
			return true, false
		}
		if block {
			leaseLost := <-attemptHeartbeatDone
			heartbeatResultRead = true
			return true, leaseLost
		}
		select {
		case leaseLost := <-attemptHeartbeatDone:
			heartbeatResultRead = true
			return true, leaseLost
		default:
			return false, false
		}
	}

	if execCtx.Err() != nil {
		_, leaseLost := checkHeartbeat(true)
		if leaseLost {
			return true
		}
		if ctx.Err() != nil {
			completion = workerShutdownCompletion(attempt)
		}
	}

	for {
		if done, leaseLost := checkHeartbeat(false); done && leaseLost {
			return true
		}

		if ctx.Err() != nil && completion.Status == orchestration.TaskAttemptStatusCompleted {
			completion.Status = orchestration.TaskAttemptStatusFailed
			completion.FailureClass = "worker_shutdown"
			completion.TerminalReason = "worker shutdown interrupted attempt"
			completion.Summary = completion.TerminalReason
		}

		completeCtx, cancelComplete := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, completeErr := svc.CompleteAttempt(completeCtx, completion)
		cancelComplete()
		if completeErr == nil {
			cancelHeartbeat()
			_, leaseLost := checkHeartbeat(true)
			return leaseLost
		}

		log.Error("complete attempt failed", slog.String("attempt_id", attempt.ID), slog.Any("error", completeErr))
		if errors.Is(completeErr, orchestration.ErrAttemptLeaseConflict) || errors.Is(completeErr, orchestration.ErrAttemptImmutable) {
			cancelHeartbeat()
			if done, leaseLost := checkHeartbeat(true); done {
				return leaseLost || errors.Is(completeErr, orchestration.ErrAttemptLeaseConflict)
			}
			return errors.Is(completeErr, orchestration.ErrAttemptLeaseConflict)
		}

		select {
		case leaseLost := <-attemptHeartbeatDone:
			heartbeatResultRead = true
			if leaseLost {
				return true
			}
			cancelHeartbeat()
			return false
		case <-ctx.Done():
		case <-time.After(completionRetryInterval):
		}
	}
}

func workerShutdownCompletion(attempt orchestration.TaskAttempt) orchestration.AttemptCompletion {
	return orchestration.AttemptCompletion{
		AttemptID:      attempt.ID,
		ClaimToken:     attempt.ClaimToken,
		Status:         orchestration.TaskAttemptStatusFailed,
		Summary:        "worker shutdown interrupted attempt",
		FailureClass:   "worker_shutdown",
		TerminalReason: "worker shutdown interrupted attempt",
	}
}

func runWorkerHeartbeatLoop(ctx context.Context, svc *orchestration.Service, log *slog.Logger, workerID, leaseToken string, leaseTTLSeconds int, cancel context.CancelFunc) {
	ticker := time.NewTicker(heartbeatInterval(leaseTTLSeconds))
	defer ticker.Stop()
	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := svc.HeartbeatWorker(ctx, workerID, leaseToken, leaseTTLSeconds); err != nil {
				log.Warn("worker heartbeat failed", slog.String("worker_id", workerID), slog.Any("error", err))
				if errors.Is(err, orchestration.ErrWorkerLeaseConflict) {
					log.Error("worker lease conflict detected; stopping worker", slog.String("worker_id", workerID))
					cancel()
					return
				}
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					log.Error("worker lease renewal failed repeatedly; stopping worker", slog.String("worker_id", workerID))
					cancel()
					return
				}
				continue
			}
			consecutiveFailures = 0
		}
	}
}

func runAttemptHeartbeatLoopWithInterval(ctx context.Context, cancel context.CancelFunc, svc attemptRuntime, log *slog.Logger, attempt orchestration.TaskAttempt, leaseTTLSeconds int, interval time.Duration, done chan<- bool) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			done <- false
			return
		case <-ticker.C:
			if _, err := svc.HeartbeatAttempt(ctx, orchestration.AttemptHeartbeat{
				AttemptID:       attempt.ID,
				ClaimToken:      attempt.ClaimToken,
				LeaseTTLSeconds: leaseTTLSeconds,
			}); err != nil {
				log.Warn("attempt heartbeat failed", slog.String("attempt_id", attempt.ID), slog.Any("error", err))
				if errors.Is(err, orchestration.ErrAttemptLeaseConflict) || errors.Is(err, orchestration.ErrAttemptImmutable) {
					cancel()
					done <- true
					return
				}
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					log.Error("attempt lease renewal failed repeatedly; cancelling execution", slog.String("attempt_id", attempt.ID))
					cancel()
					done <- true
					return
				}
				continue
			}
			consecutiveFailures = 0
		}
	}
}

func executeAttempt(ctx context.Context, queries *dbsqlc.Queries, attempt orchestration.TaskAttempt, workerProfiles []string) orchestration.AttemptCompletion {
	completion := orchestration.AttemptCompletion{
		AttemptID:          attempt.ID,
		ClaimToken:         attempt.ClaimToken,
		Status:             orchestration.TaskAttemptStatusCompleted,
		Summary:            "task executed by builtin workerd",
		StructuredOutput:   map[string]any{},
		ArtifactIntents:    []orchestration.AttemptArtifactIntent{},
		CompletionMetadata: map[string]any{"executor": orchestration.DefaultWorkerExecutorID},
	}

	taskID, err := db.ParseUUID(attempt.TaskID)
	if err != nil {
		completion.Status = orchestration.TaskAttemptStatusFailed
		completion.FailureClass = "invalid_task_id"
		completion.TerminalReason = err.Error()
		return completion
	}
	taskRow, err := queries.GetOrchestrationTaskByID(ctx, taskID)
	if err != nil {
		completion.Status = orchestration.TaskAttemptStatusFailed
		completion.FailureClass = "task_lookup_failed"
		completion.TerminalReason = err.Error()
		return completion
	}

	inputs := decodeObject(taskRow.Inputs)
	builtinOptions := decodeObjectFromAny(inputs["builtin_workerd"])
	if err := sleepWithContext(ctx, time.Duration(intValue(builtinOptions["sleep_ms"]))*time.Millisecond); err != nil {
		return workerShutdownCompletion(attempt)
	}
	if err := sleepWithContext(ctx, time.Duration(envInt("WORKER_EXECUTION_DELAY_MS", 0))*time.Millisecond); err != nil {
		return workerShutdownCompletion(attempt)
	}
	output := map[string]any{
		"task_id":        taskRow.ID.String(),
		"goal":           taskRow.Goal,
		"inputs":         inputs,
		"worker_profile": taskRow.WorkerProfile,
		"attempt_id":     attempt.ID,
	}

	if !containsString(workerProfiles, strings.TrimSpace(taskRow.WorkerProfile)) {
		completion.Status = orchestration.TaskAttemptStatusFailed
		completion.FailureClass = "unsupported_worker_profile"
		completion.TerminalReason = fmt.Sprintf("unsupported worker profile %q", taskRow.WorkerProfile)
		completion.Summary = completion.TerminalReason
		completion.StructuredOutput = output
		return completion
	}

	if attempt.InputManifestID != "" {
		manifestID, err := db.ParseUUID(attempt.InputManifestID)
		if err == nil {
			if manifestRow, err := queries.GetOrchestrationInputManifestByID(ctx, manifestID); err == nil {
				output["input_manifest_id"] = manifestRow.ID.String()
				output["projection_hash"] = manifestRow.ProjectionHash
			}
		}
	}

	completion.Summary = strings.TrimSpace(taskRow.Goal)
	if completion.Summary == "" {
		completion.Summary = "builtin workerd completed task"
	}
	if summary := strings.TrimSpace(stringValue(builtinOptions["summary"])); summary != "" {
		completion.Summary = summary
	}
	if requestReplan, ok := builtinOptions["request_replan"].(bool); ok {
		completion.RequestReplan = requestReplan
	}
	if childTasks, ok := builtinOptions["child_tasks"].([]any); ok && len(childTasks) > 0 {
		output["child_tasks"] = childTasks
	}
	completion.ArtifactIntents = decodeArtifactIntents(builtinOptions["artifact_intents"])
	if extraOutput := decodeObjectFromAny(builtinOptions["structured_output"]); len(extraOutput) > 0 {
		for key, value := range extraOutput {
			output[key] = value
		}
	}
	completion.StructuredOutput = output
	return completion
}

func decodeObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return map[string]any{}
	}
	return payload
}

func decodeObjectFromAny(raw any) map[string]any {
	value, _ := raw.(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func decodeArtifactIntents(raw any) []orchestration.AttemptArtifactIntent {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	intents := make([]orchestration.AttemptArtifactIntent, 0, len(items))
	for _, item := range items {
		payload := decodeObjectFromAny(item)
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
			Metadata:    decodeObjectFromAny(payload["metadata"]),
		})
	}
	if len(intents) == 0 {
		return nil
	}
	return intents
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func intValue(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envCSV(name string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	if len(values) == 0 {
		return append([]string(nil), fallback...)
	}
	return values
}

func stringSliceToAny(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func heartbeatInterval(leaseTTLSeconds int) time.Duration {
	if leaseTTLSeconds <= 2 {
		return time.Second
	}
	return time.Duration(leaseTTLSeconds/2) * time.Second
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
