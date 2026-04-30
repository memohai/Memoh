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
	"github.com/jackc/pgx/v5/pgxpool"

	agentpkg "github.com/memohai/memoh/internal/agent"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/orchestration"
	"github.com/memohai/memoh/internal/orchestrationexec"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/workspace"
)

type verificationExecutor = orchestration.VerificationExecutor

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "verifyd: %v\n", err)
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
	log := logger.L.With(slog.String("component", "verifyd"))

	pool, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	queries := dbsqlc.New(pool)
	svc := orchestration.NewService(log, pool, queries)
	var executor orchestration.VerificationWorkExecutor = svc
	var workerLeases orchestration.WorkerLeaseRuntime = svc
	llmRuntime, cleanupLLMRuntime, err := buildLLMRuntime(ctx, log, cfg, pool, queries)
	if err != nil {
		log.Warn("llm runtime unavailable, verifier will use builtin fallback", slog.Any("error", err))
	}
	if cleanupLLMRuntime != nil {
		defer cleanupLLMRuntime()
	}

	workerID := strings.TrimSpace(os.Getenv("VERIFIER_ID"))
	if workerID == "" {
		workerID = "verifyd-" + uuid.NewString()
	}
	executorID := strings.TrimSpace(os.Getenv("VERIFIER_EXECUTOR_ID"))
	if executorID == "" {
		executorID = orchestration.DefaultVerifierExecutorID
	}
	verifierProfiles := orchestration.NormalizeExecutionProfiles(envCSV("VERIFIER_PROFILES", []string{orchestration.DefaultVerifierProfile}))
	leaseTTLSeconds := envInt("VERIFIER_LEASE_TTL_SECONDS", 30)
	pollInterval := time.Duration(envInt("VERIFIER_POLL_INTERVAL_MS", 500)) * time.Millisecond

	workerLease, err := svc.RegisterWorker(ctx, orchestration.WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      executorID,
		DisplayName:     workerID,
		Capabilities:    orchestration.VerifierProfileCapabilities(verifierProfiles),
		LeaseTTLSeconds: leaseTTLSeconds,
	})
	if err != nil {
		return fmt.Errorf("verifier registration failed: %w", err)
	}
	workerLeaseToken := workerLease.LeaseToken

	go runWorkerHeartbeatLoop(ctx, workerLeases, log, workerID, workerLeaseToken, leaseTTLSeconds, cancel)

	for {
		if ctx.Err() != nil {
			return nil
		}

		verification, err := executor.ClaimNextVerification(ctx, orchestration.VerificationClaim{
			WorkerID:         workerID,
			ExecutorID:       executorID,
			VerifierProfiles: verifierProfiles,
			LeaseToken:       workerLeaseToken,
			LeaseTTLSeconds:  leaseTTLSeconds,
		})
		if err != nil {
			if errors.Is(err, orchestration.ErrWorkerLeaseConflict) {
				log.Error("worker lease lost; stopping verifier", slog.String("worker_id", workerID))
				return nil
			}
			if errors.Is(err, orchestration.ErrNoRunnableVerification) {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(pollInterval):
					continue
				}
			}
			log.Error("claim verification failed", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(pollInterval):
				continue
			}
		}

		if err := sleepWithContext(ctx, time.Duration(envInt("VERIFIER_START_DELAY_MS", 0))*time.Millisecond); err != nil {
			return nil
		}
		claimedFence := orchestration.NewVerificationFence(*verification)
		runningVerification, err := executor.StartClaimedVerification(ctx, claimedFence)
		if err != nil {
			log.Error("start verification failed", slog.String("verification_id", verification.ID), slog.Any("error", err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(pollInterval):
				continue
			}
		}

		leaseLost := runVerification(ctx, executor, log, *runningVerification, leaseTTLSeconds, verifierProfiles, func(execCtx context.Context, verification orchestration.TaskVerification, profiles []string) orchestration.VerificationCompletion {
			return executeVerification(execCtx, queries, llmRuntime, verification, profiles)
		})
		if leaseLost {
			log.Warn("dropping stale verification completion after lease loss", slog.String("verification_id", runningVerification.ID))
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func runVerification(ctx context.Context, svc orchestration.ClaimedVerificationRuntime, log *slog.Logger, verification orchestration.TaskVerification, leaseTTLSeconds int, verifierProfiles []string, execute verificationExecutor) bool {
	return orchestration.RunClaimedVerification(ctx, svc, log, verification, leaseTTLSeconds, verifierProfiles, execute)
}

func runVerificationWithInterval(ctx context.Context, svc orchestration.ClaimedVerificationRuntime, log *slog.Logger, verification orchestration.TaskVerification, leaseTTLSeconds int, heartbeatEvery time.Duration, verifierProfiles []string, execute verificationExecutor) bool {
	if heartbeatEvery <= 0 {
		return orchestration.RunClaimedVerification(ctx, svc, log, verification, leaseTTLSeconds, verifierProfiles, execute)
	}
	return orchestration.RunClaimedVerificationWithInterval(ctx, svc, log, verification, leaseTTLSeconds, heartbeatEvery, verifierProfiles, execute)
}

func runWorkerHeartbeatLoop(ctx context.Context, svc orchestration.WorkerLeaseRuntime, log *slog.Logger, workerID, leaseToken string, leaseTTLSeconds int, cancel context.CancelFunc) {
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
					log.Error("worker lease conflict detected; stopping verifier", slog.String("worker_id", workerID))
					cancel()
					return
				}
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					log.Error("worker heartbeat failed repeatedly; stopping verifier", slog.String("worker_id", workerID))
					cancel()
					return
				}
				continue
			}
			consecutiveFailures = 0
		}
	}
}

func executeVerification(ctx context.Context, queries *dbsqlc.Queries, llmRuntime *orchestrationexec.Runtime, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
	if err := sleepWithContext(ctx, time.Duration(envInt("VERIFIER_EXECUTION_DELAY_MS", 0))*time.Millisecond); err != nil {
		return workerShutdownVerificationCompletion(verification)
	}

	if strings.TrimSpace(verification.VerifierProfile) == orchestration.BuiltinBasicVerifierProfile {
		return orchestration.ExecuteBuiltinVerification(ctx, queries, verification)
	}

	runID, err := db.ParseUUID(verification.RunID)
	if err != nil {
		return llmVerificationUnavailable(verification, "invalid_run_id", err.Error())
	}
	runRow, err := queries.GetOrchestrationRunByID(ctx, runID)
	if err != nil {
		return llmVerificationUnavailable(verification, "run_lookup_failed", err.Error())
	}
	sourceMetadata := decodeObject(runRow.SourceMetadata)
	botID := strings.TrimSpace(stringValue(sourceMetadata["bot_id"]))
	if llmRuntime != nil && botID != "" {
		return llmRuntime.ExecuteVerification(ctx, verification)
	}
	if botID == "" {
		return llmVerificationUnavailable(verification, "llm_runtime_unavailable", "llm verifier profile requires run source_metadata.bot_id")
	}
	return llmVerificationUnavailable(verification, "llm_runtime_unavailable", "llm runtime is not configured")
}

func llmVerificationUnavailable(verification orchestration.TaskVerification, failureClass, reason string) orchestration.VerificationCompletion {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "llm verification runtime is not available"
	}
	return orchestration.VerificationCompletion{
		VerificationID: verification.ID,
		ClaimToken:     verification.ClaimToken,
		Status:         orchestration.TaskVerificationStatusFailed,
		Verdict:        orchestration.VerificationVerdictRejected,
		Summary:        reason,
		FailureClass:   failureClass,
		TerminalReason: reason,
	}
}

func heartbeatInterval(leaseTTLSeconds int) time.Duration {
	ttl := time.Duration(leaseTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = orchestration.TaskVerificationDefaultLeaseTTL
	}
	interval := ttl / 3
	if interval <= 0 {
		return time.Second
	}
	return interval
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envCSV(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	if len(items) == 0 {
		return fallback
	}
	return items
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

func buildLLMRuntime(
	ctx context.Context,
	log *slog.Logger,
	cfg config.Config,
	pool *pgxpool.Pool,
	queries *dbsqlc.Queries,
) (*orchestrationexec.Runtime, func(), error) {
	rc, err := boot.ProvideRuntimeConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	containerSvc, cleanup, err := ctr.ProvideService(ctx, log, cfg, rc.ContainerBackend)
	if err != nil {
		return nil, nil, err
	}
	manager := workspace.NewManager(log, containerSvc, cfg.Workspace, cfg.Containerd.Namespace, pool)
	a := agentpkg.New(agentpkg.Deps{
		BridgeProvider: manager,
		Logger:         log,
	})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(log, manager, nil, config.DefaultDataMount),
	})
	return orchestrationexec.NewRuntime(
		log,
		queries,
		settings.NewService(log, queries, nil),
		models.NewService(log, queries),
		a,
		rc.TimezoneLocation,
	), cleanup, nil
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

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func workerShutdownVerificationCompletion(verification orchestration.TaskVerification) orchestration.VerificationCompletion {
	return orchestration.VerificationCompletion{
		VerificationID: verification.ID,
		ClaimToken:     verification.ClaimToken,
		Status:         orchestration.TaskVerificationStatusFailed,
		Verdict:        orchestration.VerificationVerdictRejected,
		Summary:        "verifier shutdown interrupted verification",
		FailureClass:   "worker_shutdown",
		TerminalReason: "verifier shutdown interrupted verification",
	}
}
