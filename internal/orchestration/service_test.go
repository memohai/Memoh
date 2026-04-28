package orchestration

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

func TestStartRunRequestHashTreatsNilMapsLikeEmptyMaps(t *testing.T) {
	t.Parallel()

	nilHash, err := startRunRequestHash(StartRunRequest{
		Goal:           "plan a release",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("startRunRequestHash(nil maps) error = %v", err)
	}

	emptyHash, err := startRunRequestHash(StartRunRequest{
		Goal:                   "plan a release",
		IdempotencyKey:         "idem-1",
		Input:                  map[string]any{},
		OutputSchema:           map[string]any{},
		RequestedControlPolicy: map[string]any{},
		SourceMetadata:         map[string]any{},
		Policies:               map[string]any{},
	})
	if err != nil {
		t.Fatalf("startRunRequestHash(empty maps) error = %v", err)
	}

	if nilHash != emptyHash {
		t.Fatalf("startRunRequestHash nil/empty mismatch: %q != %q", nilHash, emptyHash)
	}
}

func TestNormalizeCheckpointResolutionUsesDefaultAction(t *testing.T) {
	t.Parallel()

	rawDefault := marshalJSON(CheckpointDefaultAction{
		Mode:          " " + CheckpointResolutionModeFreeform + " ",
		OptionID:      " freeform ",
		FreeformInput: "continue with cached state",
	})
	rawOptions := marshalJSON([]CheckpointOption{
		{ID: "accept", Kind: CheckpointOptionKindChoice},
		{ID: "freeform", Kind: CheckpointOptionKindFreeform},
	})

	checkpoint := sqlc.OrchestrationHumanCheckpoint{
		ID:            mustUUID(t, "550e8400-e29b-41d4-a716-446655440010"),
		TaskID:        mustUUID(t, "550e8400-e29b-41d4-a716-446655440011"),
		Status:        CheckpointStatusOpen,
		Options:       rawOptions,
		DefaultAction: rawDefault,
	}

	resolution, hash, err := normalizeCheckpointResolution(checkpoint, CheckpointResolution{
		Mode:           CheckpointResolutionModeUseDefault,
		IdempotencyKey: "idem-2",
		Metadata:       nil,
	})
	if err != nil {
		t.Fatalf("normalizeCheckpointResolution(use_default) error = %v", err)
	}

	if resolution.Mode != CheckpointResolutionModeFreeform {
		t.Fatalf("normalized mode = %q, want %q", resolution.Mode, CheckpointResolutionModeFreeform)
	}
	if resolution.OptionID != "freeform" {
		t.Fatalf("normalized option_id = %q, want freeform", resolution.OptionID)
	}
	if resolution.FreeformInput != "continue with cached state" {
		t.Fatalf("normalized freeform_input = %q", resolution.FreeformInput)
	}
	if resolution.Metadata == nil {
		t.Fatal("normalized metadata should be canonicalized to an empty object")
	}
	if hash == "" {
		t.Fatal("normalizeCheckpointResolution should return a non-empty request hash")
	}
}

func TestValidateCheckpointDefinitionRejectsDuplicateOptionIDs(t *testing.T) {
	t.Parallel()

	_, _, err := validateCheckpointDefinition([]CheckpointOption{
		{ID: "approve", Kind: CheckpointOptionKindChoice},
		{ID: " approve ", Kind: CheckpointOptionKindFreeform},
	}, nil, time.Time{})
	if err == nil {
		t.Fatal("validateCheckpointDefinition() error = nil, want duplicate option id error")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("validateCheckpointDefinition() error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestValidateCheckpointDefinitionReturnsNormalizedValues(t *testing.T) {
	t.Parallel()

	options, defaultAction, err := validateCheckpointDefinition(
		[]CheckpointOption{
			{ID: " approve ", Kind: CheckpointOptionKindChoice, Label: "Approve"},
			{ID: "details", Kind: CheckpointOptionKindFreeform, Label: "Details"},
		},
		&CheckpointDefaultAction{
			Mode:     " " + CheckpointResolutionModeSelectOption + " ",
			OptionID: " approve ",
		},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("validateCheckpointDefinition() error = %v", err)
	}
	if options[0].ID != "approve" {
		t.Fatalf("normalized option id = %q, want %q", options[0].ID, "approve")
	}
	if defaultAction == nil {
		t.Fatal("normalized default_action = nil")
	}
	if defaultAction.Mode != CheckpointResolutionModeSelectOption {
		t.Fatalf("normalized default_action mode = %q, want %q", defaultAction.Mode, CheckpointResolutionModeSelectOption)
	}
	if defaultAction.OptionID != "approve" {
		t.Fatalf("normalized default_action option_id = %q, want %q", defaultAction.OptionID, "approve")
	}
}

func TestPaginateTasksProducesStableOpaqueCursor(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	items := []Task{
		{ID: "task-1", CreatedAt: base},
		{ID: "task-2", CreatedAt: base.Add(time.Second)},
		{ID: "task-3", CreatedAt: base.Add(2 * time.Second)},
	}

	page1, after, err := paginateTasks(items, "", 2, 41, filterHash([]string{"ready"}))
	if err != nil {
		t.Fatalf("paginateTasks(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	if after == "" {
		t.Fatal("page1 should emit next_after")
	}
	cursor, err := decodeCursor(after)
	if err != nil {
		t.Fatalf("decodeCursor(page1 after) error = %v", err)
	}
	if cursor.SnapshotSeq != 41 {
		t.Fatalf("cursor snapshot_seq = %d, want 41", cursor.SnapshotSeq)
	}
	if cursor.FilterHash != filterHash([]string{"ready"}) {
		t.Fatalf("cursor filter_hash = %q, want %q", cursor.FilterHash, filterHash([]string{"ready"}))
	}

	page2, nextAfter, err := paginateTasks(items, after, 2, 41, filterHash([]string{"ready"}))
	if err != nil {
		t.Fatalf("paginateTasks(page2) error = %v", err)
	}
	if len(page2) != 1 || page2[0].ID != "task-3" {
		t.Fatalf("page2 = %+v, want only task-3", page2)
	}
	if nextAfter != "" {
		t.Fatalf("final page next_after = %q, want empty", nextAfter)
	}
}

func TestResolvePageAsOfSeqUsesSnapshotEmbeddedInCursor(t *testing.T) {
	t.Parallel()

	cursor := encodeCursor("task-1", time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC), 52, "")
	asOfSeq, err := resolvePageAsOfSeq(80, 0, cursor)
	if err != nil {
		t.Fatalf("resolvePageAsOfSeq(cursor snapshot) error = %v", err)
	}
	if asOfSeq != 52 {
		t.Fatalf("resolvePageAsOfSeq(cursor snapshot) = %d, want 52", asOfSeq)
	}
}

func TestResolvePageAsOfSeqRejectsLegacyContinuationWithoutSnapshot(t *testing.T) {
	t.Parallel()

	legacyCursor := base64.RawURLEncoding.EncodeToString(marshalJSON(struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"created_at"`
	}{
		ID:        "task-1",
		CreatedAt: time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC),
	}))

	_, err := resolvePageAsOfSeq(80, 0, legacyCursor)
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("resolvePageAsOfSeq(legacy cursor) error = %v, want %v", err, ErrInvalidCursor)
	}
}

func TestResolvePageAsOfSeqRejectsSnapshotMismatch(t *testing.T) {
	t.Parallel()

	cursor := encodeCursor("task-1", time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC), 52, "")
	_, err := resolvePageAsOfSeq(80, 53, cursor)
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("resolvePageAsOfSeq(mismatched snapshot) error = %v, want %v", err, ErrInvalidCursor)
	}
}

func TestDecodePlannedChildTasksNormalizesMinimalPlan(t *testing.T) {
	t.Parallel()

	plans := decodePlannedChildTasks(map[string]any{
		"child_tasks": []any{
			map[string]any{
				"id":             "first",
				"goal":           " first child ",
				"worker_profile": " " + DefaultRootWorkerProfile + " ",
				"priority":       float64(2),
				"inputs":         map[string]any{"step": "one"},
				"retry_policy":   map[string]any{"max_attempts": float64(3)},
				"verification_policy": map[string]any{
					"mode": "strict",
				},
				"blackboard_scope": " scope.one ",
			},
			map[string]any{
				"goal":       "second child",
				"depends_on": []any{"first"},
			},
			map[string]any{
				"goal": "   ",
			},
		},
	})

	if len(plans) != 2 {
		t.Fatalf("decodePlannedChildTasks() len = %d, want 2", len(plans))
	}
	if plans[0].Goal != "first child" {
		t.Fatalf("first child goal = %q, want %q", plans[0].Goal, "first child")
	}
	if plans[0].Alias != "first" {
		t.Fatalf("first child alias = %q, want %q", plans[0].Alias, "first")
	}
	if plans[0].WorkerProfile != DefaultRootWorkerProfile {
		t.Fatalf("first child worker_profile = %q, want %q", plans[0].WorkerProfile, DefaultRootWorkerProfile)
	}
	if plans[0].Priority != 2 {
		t.Fatalf("first child priority = %d, want 2", plans[0].Priority)
	}
	if plans[0].BlackboardScope != "scope.one" {
		t.Fatalf("first child blackboard_scope = %q, want %q", plans[0].BlackboardScope, "scope.one")
	}
	if plans[1].Kind != "child" {
		t.Fatalf("second child kind = %q, want %q", plans[1].Kind, "child")
	}
	if plans[1].WorkerProfile != DefaultRootWorkerProfile {
		t.Fatalf("second child worker_profile = %q, want %q", plans[1].WorkerProfile, DefaultRootWorkerProfile)
	}
	if len(plans[1].DependsOnAliases) != 1 || plans[1].DependsOnAliases[0] != "first" {
		t.Fatalf("second child depends_on = %#v, want [first]", plans[1].DependsOnAliases)
	}
}

func TestResolveEventUntilSeqDefaultsToCurrentAndRejectsInvalidBounds(t *testing.T) {
	t.Parallel()

	untilSeq, err := resolveEventUntilSeq(80, 0, 0)
	if err != nil {
		t.Fatalf("resolveEventUntilSeq(default current) error = %v", err)
	}
	if untilSeq != 80 {
		t.Fatalf("resolveEventUntilSeq(default current) = %d, want 80", untilSeq)
	}

	_, err = resolveEventUntilSeq(80, 40, 39)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("resolveEventUntilSeq(after>until) error = %v, want %v", err, ErrInvalidArgument)
	}

	_, err = resolveEventUntilSeq(80, 0, 81)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("resolveEventUntilSeq(until>current) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestPaginateTasksRejectsFilterMismatchAcrossPages(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	items := []Task{
		{ID: "task-1", CreatedAt: base},
		{ID: "task-2", CreatedAt: base.Add(time.Second)},
		{ID: "task-3", CreatedAt: base.Add(2 * time.Second)},
	}

	_, after, err := paginateTasks(items, "", 2, 41, filterHash([]string{"ready"}))
	if err != nil {
		t.Fatalf("paginateTasks(page1) error = %v", err)
	}

	_, _, err = paginateTasks(items, after, 2, 41, filterHash([]string{"waiting_human"}))
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("paginateTasks(filter mismatch) error = %v, want %v", err, ErrInvalidCursor)
	}
}

func TestBuildPhase1ControlPolicyAndAuthorizeRun(t *testing.T) {
	t.Parallel()

	caller := ControlIdentity{TenantID: "user-1", Subject: "user-1"}
	policy := buildPhase1ControlPolicy(caller)
	if got := policy["mode"]; got != ControlPolicyModeOwnerOnly {
		t.Fatalf("control policy mode = %v, want %q", got, ControlPolicyModeOwnerOnly)
	}
	if got := policy["owner_subject"]; got != caller.Subject {
		t.Fatalf("control policy owner_subject = %v, want %q", got, caller.Subject)
	}

	run := sqlc.OrchestrationRun{
		TenantID:     caller.TenantID,
		OwnerSubject: caller.Subject,
		ControlPolicy: marshalJSON(map[string]any{
			"mode":          ControlPolicyModeOwnerOnly,
			"owner_subject": caller.Subject,
		}),
	}
	if err := authorizeRun(caller, run); err != nil {
		t.Fatalf("authorizeRun(owner_only) error = %v", err)
	}
	if err := authorizeRun(ControlIdentity{TenantID: "other-tenant", Subject: caller.Subject}, run); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("authorizeRun(cross-tenant same-subject) error = %v, want %v", err, ErrAccessDenied)
	}
	if err := authorizeRun(ControlIdentity{TenantID: "user-2", Subject: "user-2"}, run); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("authorizeRun(non-owner) error = %v, want %v", err, ErrAccessDenied)
	}
}

func TestTaskAcceptsHumanCheckpoint(t *testing.T) {
	t.Parallel()

	if taskAcceptsHumanCheckpoint(TaskStatusCompleted) {
		t.Fatal("taskAcceptsHumanCheckpoint(completed) = true, want false")
	}
	if taskAcceptsHumanCheckpoint(TaskStatusFailed) {
		t.Fatal("taskAcceptsHumanCheckpoint(failed) = true, want false")
	}
	if taskAcceptsHumanCheckpoint(TaskStatusCancelled) {
		t.Fatal("taskAcceptsHumanCheckpoint(cancelled) = true, want false")
	}
	if !taskAcceptsHumanCheckpoint(TaskStatusRunning) {
		t.Fatal("taskAcceptsHumanCheckpoint(running) = false, want true")
	}
}

func TestNormalizeCheckpointResumePolicy(t *testing.T) {
	t.Parallel()

	policy, err := normalizeCheckpointResumePolicy(&CheckpointResumePolicy{ResumeMode: " " + CheckpointResumeModeNewAttempt + " "})
	if err != nil {
		t.Fatalf("normalizeCheckpointResumePolicy(valid) error = %v", err)
	}
	if policy == nil || policy.ResumeMode != CheckpointResumeModeNewAttempt {
		t.Fatalf("normalizeCheckpointResumePolicy(valid) = %+v, want %q", policy, CheckpointResumeModeNewAttempt)
	}

	if _, err := normalizeCheckpointResumePolicy(&CheckpointResumePolicy{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("normalizeCheckpointResumePolicy(empty) error = %v, want %v", err, ErrInvalidArgument)
	}
	if _, err := normalizeCheckpointResumePolicy(&CheckpointResumePolicy{ResumeMode: CheckpointResumeModeResumeHeldEnv}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("normalizeCheckpointResumePolicy(resume_held_env) error = %v, want %v", err, ErrInvalidArgument)
	}
	if _, err := normalizeCheckpointResumePolicy(&CheckpointResumePolicy{ResumeMode: "resume_later"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("normalizeCheckpointResumePolicy(unsupported) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestNormalizeIdempotencyKey(t *testing.T) {
	t.Parallel()

	if got := normalizeIdempotencyKey("  idem-1  "); got != "idem-1" {
		t.Fatalf("normalizeIdempotencyKey() = %q, want %q", got, "idem-1")
	}
}

func TestTaskPauseableByRunBarrier(t *testing.T) {
	t.Parallel()

	if !taskPauseableByRunBarrier(sqlc.OrchestrationTask{Status: TaskStatusReady}) {
		t.Fatal("taskPauseableByRunBarrier(ready) = false, want true")
	}
	if !taskPauseableByRunBarrier(sqlc.OrchestrationTask{Status: TaskStatusDispatching}) {
		t.Fatal("taskPauseableByRunBarrier(dispatching) = false, want true")
	}
	if !taskPauseableByRunBarrier(sqlc.OrchestrationTask{Status: TaskStatusRunning}) {
		t.Fatal("taskPauseableByRunBarrier(running) = false, want true")
	}
	if !taskPauseableByRunBarrier(sqlc.OrchestrationTask{Status: TaskStatusVerifying}) {
		t.Fatal("taskPauseableByRunBarrier(verifying) = false, want true")
	}
	if taskPauseableByRunBarrier(sqlc.OrchestrationTask{Status: TaskStatusWaitingHuman}) {
		t.Fatal("taskPauseableByRunBarrier(waiting_human) = true, want false")
	}
	if taskPauseableByRunBarrier(sqlc.OrchestrationTask{Status: TaskStatusCompleted}) {
		t.Fatal("taskPauseableByRunBarrier(completed) = true, want false")
	}
	if taskPauseableByRunBarrier(sqlc.OrchestrationTask{
		Status:              TaskStatusVerifying,
		WaitingCheckpointID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440012"),
	}) {
		t.Fatal("taskPauseableByRunBarrier(waiting_checkpoint_id) = true, want false")
	}
}

func TestTaskSupportsHumanCheckpoint(t *testing.T) {
	t.Parallel()

	if !taskSupportsHumanCheckpoint(TaskStatusReady) {
		t.Fatal("taskSupportsHumanCheckpoint(ready) = false, want true")
	}
	if taskSupportsHumanCheckpoint(TaskStatusRunning) {
		t.Fatal("taskSupportsHumanCheckpoint(running) = true, want false")
	}
}

func TestTaskWaitingOnRunBarrier(t *testing.T) {
	t.Parallel()

	checkpointID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440013")
	if !taskWaitingOnRunBarrier(sqlc.OrchestrationTask{
		Status:              TaskStatusWaitingHuman,
		WaitingScope:        "run",
		WaitingCheckpointID: checkpointID,
	}, checkpointID) {
		t.Fatal("taskWaitingOnRunBarrier(valid) = false, want true")
	}
	if taskWaitingOnRunBarrier(sqlc.OrchestrationTask{
		Status:              TaskStatusWaitingHuman,
		WaitingScope:        "task",
		WaitingCheckpointID: checkpointID,
	}, checkpointID) {
		t.Fatal("taskWaitingOnRunBarrier(task scope) = true, want false")
	}
}

func TestNormalizeCheckpointResolutionCanonicalizesChoiceFreeformInput(t *testing.T) {
	t.Parallel()

	checkpoint := sqlc.OrchestrationHumanCheckpoint{
		ID:      mustUUID(t, "550e8400-e29b-41d4-a716-446655440014"),
		TaskID:  mustUUID(t, "550e8400-e29b-41d4-a716-446655440015"),
		Status:  CheckpointStatusOpen,
		Options: marshalJSON([]CheckpointOption{{ID: "approve", Kind: CheckpointOptionKindChoice}}),
	}

	a, hashA, err := normalizeCheckpointResolution(checkpoint, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "approve",
		FreeformInput:  "   ",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("normalizeCheckpointResolution(choice whitespace) error = %v", err)
	}
	b, hashB, err := normalizeCheckpointResolution(checkpoint, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "approve",
		FreeformInput:  "",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("normalizeCheckpointResolution(choice empty) error = %v", err)
	}
	if a.FreeformInput != "" || b.FreeformInput != "" {
		t.Fatalf("normalized choice freeform_input = %q / %q, want empty", a.FreeformInput, b.FreeformInput)
	}
	if hashA != hashB {
		t.Fatalf("choice resolution hash mismatch: %q != %q", hashA, hashB)
	}
}

func TestNormalizeObjectCanonicalizesTypedContainers(t *testing.T) {
	t.Parallel()

	normalized := normalizeObject(map[string]any{
		"labels": []string{"a", "b"},
		"attrs":  map[string]string{"x": "y"},
		"empty":  []string(nil),
	})

	labels, ok := normalized["labels"].([]any)
	if !ok || len(labels) != 2 {
		t.Fatalf("normalizeObject(labels) = %#v, want []any len 2", normalized["labels"])
	}
	attrs, ok := normalized["attrs"].(map[string]any)
	if !ok || attrs["x"] != "y" {
		t.Fatalf("normalizeObject(attrs) = %#v, want map[x:y]", normalized["attrs"])
	}
	empty, ok := normalized["empty"].([]any)
	if !ok || len(empty) != 0 {
		t.Fatalf("normalizeObject(empty) = %#v, want empty []any", normalized["empty"])
	}
}

func TestFilterArtifactsByTaskAndKind(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	items := []Artifact{
		{ID: "a1", TaskID: "task-1", Kind: "image", CreatedAt: base},
		{ID: "a2", TaskID: "task-2", Kind: "text", CreatedAt: base.Add(time.Second)},
		{ID: "a3", TaskID: "task-1", Kind: "text", CreatedAt: base.Add(2 * time.Second)},
	}

	filtered := filterArtifacts(items, "task-1", []string{"text"})
	if len(filtered) != 1 || filtered[0].ID != "a3" {
		t.Fatalf("filterArtifacts(task-1,text) = %+v, want only a3", filtered)
	}

	page, after, err := paginateArtifacts(items, "", 2, 12, filterHash([]string{"task-1"}, []string{"image", "text"}))
	if err != nil {
		t.Fatalf("paginateArtifacts(page1) error = %v", err)
	}
	if len(page) != 2 || after == "" {
		t.Fatalf("paginateArtifacts(page1) = %+v, after=%q", page, after)
	}
	page, after, err = paginateArtifacts(items, after, 2, 12, filterHash([]string{"task-1"}, []string{"image", "text"}))
	if err != nil {
		t.Fatalf("paginateArtifacts(page2) error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "a3" || after != "" {
		t.Fatalf("paginateArtifacts(page2) = %+v, after=%q", page, after)
	}
}

func TestPaginateCheckpointsRejectsFilterMismatchAcrossPages(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	items := []HumanCheckpoint{
		{ID: "cp-1", CreatedAt: base},
		{ID: "cp-2", CreatedAt: base.Add(time.Second)},
		{ID: "cp-3", CreatedAt: base.Add(2 * time.Second)},
	}

	_, after, err := paginateCheckpoints(items, "", 2, 41, filterHash([]string{"open"}))
	if err != nil {
		t.Fatalf("paginateCheckpoints(page1) error = %v", err)
	}

	_, _, err = paginateCheckpoints(items, after, 2, 41, filterHash([]string{"resolved"}))
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("paginateCheckpoints(filter mismatch) error = %v, want %v", err, ErrInvalidCursor)
	}
}

func TestPaginateArtifactsRejectsFilterMismatchAcrossPages(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	items := []Artifact{
		{ID: "a1", TaskID: "task-1", Kind: "image", CreatedAt: base},
		{ID: "a2", TaskID: "task-1", Kind: "text", CreatedAt: base.Add(time.Second)},
		{ID: "a3", TaskID: "task-1", Kind: "report", CreatedAt: base.Add(2 * time.Second)},
	}

	_, after, err := paginateArtifacts(items, "", 2, 12, filterHash([]string{"task-1"}, []string{"image", "text"}))
	if err != nil {
		t.Fatalf("paginateArtifacts(page1) error = %v", err)
	}

	_, _, err = paginateArtifacts(items, after, 2, 12, filterHash([]string{"task-2"}, []string{"image", "text"}))
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("paginateArtifacts(task filter mismatch) error = %v, want %v", err, ErrInvalidCursor)
	}

	_, _, err = paginateArtifacts(items, after, 2, 12, filterHash([]string{"task-1"}, []string{"report"}))
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("paginateArtifacts(kind filter mismatch) error = %v, want %v", err, ErrInvalidCursor)
	}
}

func TestFilterHashDeduplicatesEquivalentFilters(t *testing.T) {
	t.Parallel()

	taskStatusSingle := filterHash([]string{"ready"})
	taskStatusDup := filterHash([]string{" ready ", "ready", "ready"})
	if taskStatusSingle != taskStatusDup {
		t.Fatalf("filterHash duplicate status mismatch: %q != %q", taskStatusSingle, taskStatusDup)
	}

	artifactSingle := filterHash([]string{"task-1"}, []string{"report"})
	artifactDup := filterHash([]string{" task-1 ", "task-1"}, []string{"report", "report"})
	if artifactSingle != artifactDup {
		t.Fatalf("filterHash duplicate artifact filter mismatch: %q != %q", artifactSingle, artifactDup)
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	value, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatalf("db.ParseUUID(%q): %v", raw, err)
	}
	return value
}
