package orchestration

import "time"

const (
	LifecycleStatusCreated      = "created"
	LifecycleStatusRunning      = "running"
	LifecycleStatusWaitingHuman = "waiting_human"
	LifecycleStatusCancelling   = "cancelling"
	LifecycleStatusCompleted    = "completed"
	LifecycleStatusFailed       = "failed"
	LifecycleStatusCancelled    = "cancelled"

	PlanningStatusIdle   = "idle"
	PlanningStatusActive = "active"

	PlanningIntentKindStartRun         = "start_run"
	PlanningIntentKindCheckpointResume = "checkpoint_resume"
	PlanningIntentKindAttemptFinalize  = "attempt_finalize"
	PlanningIntentKindReplan           = "replan"
	PlanningIntentStatusPending        = "pending"
	PlanningIntentStatusProcessing     = "processing"
	PlanningIntentStatusCompleted      = "completed"
	PlanningIntentStatusFailed         = "failed"
	PlanningIntentDefaultLeaseTTL      = 30 * time.Second
	TaskAttemptStatusCreated           = "created"
	TaskAttemptStatusClaimed           = "claimed"
	TaskAttemptStatusRunning           = "running"
	TaskAttemptStatusCompleted         = "completed"
	TaskAttemptStatusFailed            = "failed"
	TaskAttemptStatusLost              = "lost"
	TaskAttemptDefaultLeaseTTL         = 30 * time.Second
	TaskVerificationStatusCreated      = "created"
	TaskVerificationStatusClaimed      = "claimed"
	TaskVerificationStatusRunning      = "running"
	TaskVerificationStatusCompleted    = "completed"
	TaskVerificationStatusFailed       = "failed"
	TaskVerificationStatusLost         = "lost"
	TaskVerificationDefaultLeaseTTL    = 30 * time.Second
	VerificationVerdictAccepted        = "accepted"
	VerificationVerdictRejected        = "rejected"
	VerificationModeBuiltinBasic       = "builtin_basic"
	VerificationRejectActionFailTask   = "fail_task"
	VerificationRejectActionReplan     = "request_replan"
	WorkerStatusActive                 = "active"
	WorkerStatusUnavailable            = "unavailable"
	DefaultWorkerExecutorID            = "builtin.workerd"
	DefaultWorkerDisplayName           = "Builtin Workerd"
	DefaultRootWorkerProfile           = "builtin.echo"
	DefaultVerifierExecutorID          = "builtin.verifyd"
	DefaultVerifierDisplayName         = "Builtin Verifyd"
	DefaultVerifierProfile             = "builtin.basic"

	TaskStatusCreated      = "created"
	TaskStatusReady        = "ready"
	TaskStatusDispatching  = "dispatching"
	TaskStatusRunning      = "running"
	TaskStatusVerifying    = "verifying"
	TaskStatusWaitingHuman = "waiting_human"
	TaskStatusCompleted    = "completed"
	TaskStatusBlocked      = "blocked"
	TaskStatusFailed       = "failed"
	TaskStatusCancelled    = "cancelled"

	CheckpointStatusOpen       = "open"
	CheckpointStatusResolved   = "resolved"
	CheckpointStatusTimedOut   = "timed_out"
	CheckpointStatusCancelled  = "cancelled"
	CheckpointStatusSuperseded = "superseded"

	CheckpointOptionKindChoice   = "choice"
	CheckpointOptionKindFreeform = "freeform"

	CheckpointResolutionModeSelectOption = "select_option"
	CheckpointResolutionModeFreeform     = "freeform"
	CheckpointResolutionModeUseDefault   = "use_default"

	CheckpointResumeModeNewAttempt    = "new_attempt"
	CheckpointResumeModeResumeHeldEnv = "resume_held_env"

	ControlPolicyModeOwnerOnly = "owner_only"

	ProjectionKindTasks       = "tasks"
	ProjectionKindCheckpoints = "checkpoints"
	ProjectionKindArtifacts   = "artifacts"
	ProjectionKindRun         = "run"

	defaultListLimit  = 50
	maxListLimit      = 200
	defaultEventLimit = 100
	maxEventLimit     = 500

	methodStartRun              = "StartRun"
	methodCreateHumanCheckpoint = "CreateHumanCheckpoint"
	methodCommitArtifact        = "CommitArtifact"
	methodResolveCheckpoint     = "ResolveCheckpoint"
	methodCompleteAttempt       = "CompleteAttempt"
	methodFailAttempt           = "FailAttempt"
	methodCompleteVerification  = "CompleteVerification"
)

type ControlIdentity struct {
	TenantID string `json:"tenant_id"`
	Subject  string `json:"subject"`
}

type StartRunRequest struct {
	Goal                   string         `json:"goal" validate:"required"`
	Input                  map[string]any `json:"input"`
	OutputSchema           map[string]any `json:"output_schema"`
	IdempotencyKey         string         `json:"idempotency_key" validate:"required"`
	RequestedControlPolicy map[string]any `json:"requested_control_policy"`
	SourceMetadata         map[string]any `json:"source_metadata"`
	Policies               map[string]any `json:"policies"`
}

type RunHandle struct {
	RunID       string `json:"run_id"`
	RootTaskID  string `json:"root_task_id"`
	SnapshotSeq uint64 `json:"snapshot_seq"`
}

type Run struct {
	ID                     string         `json:"id"`
	TenantID               string         `json:"tenant_id"`
	OwnerSubject           string         `json:"owner_subject"`
	LifecycleStatus        string         `json:"lifecycle_status"`
	PlanningStatus         string         `json:"planning_status"`
	StatusVersion          uint64         `json:"status_version"`
	PlannerEpoch           uint64         `json:"planner_epoch"`
	RootTaskID             string         `json:"root_task_id"`
	Goal                   string         `json:"goal"`
	Input                  map[string]any `json:"input"`
	OutputSchema           map[string]any `json:"output_schema"`
	RequestedControlPolicy map[string]any `json:"requested_control_policy"`
	ControlPolicy          map[string]any `json:"control_policy"`
	SourceMetadata         map[string]any `json:"source_metadata"`
	Policies               map[string]any `json:"policies"`
	CreatedBy              string         `json:"created_by"`
	TerminalReason         string         `json:"terminal_reason,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	FinishedAt             time.Time      `json:"finished_at,omitempty"`
}

type RunSnapshot struct {
	Run         Run    `json:"run"`
	SnapshotSeq uint64 `json:"snapshot_seq"`
}

type ListRunEventsRequest struct {
	AfterSeq uint64 `json:"after_seq"`
	UntilSeq uint64 `json:"until_seq"`
	Limit    int    `json:"limit"`
}

type RunEventPage struct {
	Items    []RunEvent `json:"items"`
	UntilSeq uint64     `json:"until_seq"`
}

type ListRunCheckpointsRequest struct {
	Status  []string `json:"status"`
	After   string   `json:"after"`
	Limit   int      `json:"limit"`
	AsOfSeq uint64   `json:"as_of_seq"`
}

type ListRunTasksRequest struct {
	Status  []string `json:"status"`
	After   string   `json:"after"`
	Limit   int      `json:"limit"`
	AsOfSeq uint64   `json:"as_of_seq"`
}

type ListRunArtifactsRequest struct {
	TaskID  string   `json:"task_id"`
	Kind    []string `json:"kind"`
	After   string   `json:"after"`
	Limit   int      `json:"limit"`
	AsOfSeq uint64   `json:"as_of_seq"`
}

type TaskPage struct {
	Items       []Task `json:"items"`
	NextAfter   string `json:"next_after,omitempty"`
	SnapshotSeq uint64 `json:"snapshot_seq"`
}

type HumanCheckpointPage struct {
	Items       []HumanCheckpoint `json:"items"`
	NextAfter   string            `json:"next_after,omitempty"`
	SnapshotSeq uint64            `json:"snapshot_seq"`
}

type ArtifactPage struct {
	Items       []Artifact `json:"items"`
	NextAfter   string     `json:"next_after,omitempty"`
	SnapshotSeq uint64     `json:"snapshot_seq"`
}

type CreateHumanCheckpointResult struct {
	Checkpoint  HumanCheckpoint `json:"checkpoint"`
	SnapshotSeq uint64          `json:"snapshot_seq"`
}

type Task struct {
	ID                       string         `json:"id"`
	RunID                    string         `json:"run_id"`
	DecomposedFromTaskID     string         `json:"decomposed_from_task_id,omitempty"`
	Kind                     string         `json:"kind"`
	Goal                     string         `json:"goal"`
	Inputs                   map[string]any `json:"inputs"`
	PlannerEpoch             uint64         `json:"planner_epoch"`
	SupersededByPlannerEpoch uint64         `json:"superseded_by_planner_epoch,omitempty"`
	WorkerProfile            string         `json:"worker_profile,omitempty"`
	Priority                 int            `json:"priority"`
	RetryPolicy              map[string]any `json:"retry_policy"`
	VerificationPolicy       map[string]any `json:"verification_policy"`
	Status                   string         `json:"status"`
	StatusVersion            uint64         `json:"status_version"`
	WaitingCheckpointID      string         `json:"waiting_checkpoint_id,omitempty"`
	WaitingScope             string         `json:"waiting_scope,omitempty"`
	LatestResultID           string         `json:"latest_result_id,omitempty"`
	ReadyAt                  time.Time      `json:"ready_at,omitempty"`
	BlockedReason            string         `json:"blocked_reason,omitempty"`
	TerminalReason           string         `json:"terminal_reason,omitempty"`
	BlackboardScope          string         `json:"blackboard_scope,omitempty"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
}

type PlanningIntent struct {
	ID               string         `json:"id"`
	RunID            string         `json:"run_id"`
	TaskID           string         `json:"task_id,omitempty"`
	CheckpointID     string         `json:"checkpoint_id,omitempty"`
	Kind             string         `json:"kind"`
	Status           string         `json:"status"`
	BasePlannerEpoch uint64         `json:"base_planner_epoch"`
	ClaimEpoch       uint64         `json:"claim_epoch"`
	ClaimToken       string         `json:"claim_token,omitempty"`
	ClaimedBy        string         `json:"claimed_by,omitempty"`
	LeaseExpiresAt   time.Time      `json:"lease_expires_at,omitempty"`
	LastHeartbeatAt  time.Time      `json:"last_heartbeat_at,omitempty"`
	FailureReason    string         `json:"failure_reason,omitempty"`
	Payload          map[string]any `json:"payload"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type InputManifest struct {
	ID                          string           `json:"id"`
	RunID                       string           `json:"run_id"`
	TaskID                      string           `json:"task_id"`
	CapturedTaskInputs          map[string]any   `json:"captured_task_inputs"`
	CapturedArtifactVersions    []map[string]any `json:"captured_artifact_versions"`
	CapturedBlackboardRevisions []map[string]any `json:"captured_blackboard_revisions"`
	ProjectionHash              string           `json:"projection_hash"`
	CreatedAt                   time.Time        `json:"created_at"`
}

type TaskAttempt struct {
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	TaskID           string    `json:"task_id"`
	AttemptNo        int       `json:"attempt_no"`
	WorkerID         string    `json:"worker_id,omitempty"`
	ExecutorID       string    `json:"executor_id,omitempty"`
	Status           string    `json:"status"`
	ClaimEpoch       uint64    `json:"claim_epoch"`
	ClaimToken       string    `json:"claim_token,omitempty"`
	LeaseExpiresAt   time.Time `json:"lease_expires_at,omitempty"`
	LastHeartbeatAt  time.Time `json:"last_heartbeat_at,omitempty"`
	InputManifestID  string    `json:"input_manifest_id,omitempty"`
	ParkCheckpointID string    `json:"park_checkpoint_id,omitempty"`
	FailureClass     string    `json:"failure_class,omitempty"`
	TerminalReason   string    `json:"terminal_reason,omitempty"`
	StartedAt        time.Time `json:"started_at,omitempty"`
	FinishedAt       time.Time `json:"finished_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type WorkerRegistration struct {
	WorkerID        string         `json:"worker_id"`
	ExecutorID      string         `json:"executor_id"`
	DisplayName     string         `json:"display_name"`
	Capabilities    map[string]any `json:"capabilities"`
	LeaseToken      string         `json:"lease_token,omitempty"`
	LeaseTTLSeconds int            `json:"lease_ttl_seconds"`
}

type AttemptClaim struct {
	WorkerID        string   `json:"worker_id"`
	ExecutorID      string   `json:"executor_id"`
	WorkerProfiles  []string `json:"worker_profiles"`
	LeaseToken      string   `json:"lease_token,omitempty"`
	LeaseTTLSeconds int      `json:"lease_ttl_seconds"`
}

type AttemptHeartbeat struct {
	AttemptID       string `json:"attempt_id"`
	ClaimToken      string `json:"claim_token"`
	LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
}

type AttemptArtifactIntent struct {
	Kind        string         `json:"kind"`
	URI         string         `json:"uri"`
	Version     string         `json:"version"`
	Digest      string         `json:"digest"`
	ContentType string         `json:"content_type,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Metadata    map[string]any `json:"metadata"`
}

type AttemptCompletion struct {
	AttemptID          string                  `json:"attempt_id"`
	ClaimToken         string                  `json:"claim_token"`
	Status             string                  `json:"status"`
	Summary            string                  `json:"summary"`
	StructuredOutput   map[string]any          `json:"structured_output"`
	FailureClass       string                  `json:"failure_class"`
	TerminalReason     string                  `json:"terminal_reason"`
	RequestReplan      bool                    `json:"request_replan"`
	ArtifactIntents    []AttemptArtifactIntent `json:"artifact_intents"`
	CompletionMetadata map[string]any          `json:"completion_metadata"`
	IdempotencyKey     string                  `json:"idempotency_key"`
}

type WorkerLease struct {
	ID              string         `json:"id"`
	ExecutorID      string         `json:"executor_id"`
	DisplayName     string         `json:"display_name"`
	Capabilities    map[string]any `json:"capabilities"`
	Status          string         `json:"status"`
	LeaseToken      string         `json:"lease_token,omitempty"`
	LastHeartbeatAt time.Time      `json:"last_heartbeat_at"`
	LeaseExpiresAt  time.Time      `json:"lease_expires_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type TaskVerification struct {
	ID              string    `json:"id"`
	RunID           string    `json:"run_id"`
	TaskID          string    `json:"task_id"`
	ResultID        string    `json:"result_id"`
	AttemptNo       int       `json:"attempt_no"`
	WorkerID        string    `json:"worker_id,omitempty"`
	ExecutorID      string    `json:"executor_id,omitempty"`
	VerifierProfile string    `json:"verifier_profile,omitempty"`
	Status          string    `json:"status"`
	ClaimEpoch      uint64    `json:"claim_epoch"`
	ClaimToken      string    `json:"claim_token,omitempty"`
	LeaseExpiresAt  time.Time `json:"lease_expires_at,omitempty"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty"`
	Verdict         string    `json:"verdict,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	FailureClass    string    `json:"failure_class,omitempty"`
	TerminalReason  string    `json:"terminal_reason,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type VerificationClaim struct {
	WorkerID         string   `json:"worker_id"`
	ExecutorID       string   `json:"executor_id"`
	VerifierProfiles []string `json:"verifier_profiles"`
	LeaseToken       string   `json:"lease_token,omitempty"`
	LeaseTTLSeconds  int      `json:"lease_ttl_seconds"`
}

type VerificationHeartbeat struct {
	VerificationID  string `json:"verification_id"`
	ClaimToken      string `json:"claim_token"`
	LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
}

type VerificationCompletion struct {
	VerificationID string `json:"verification_id"`
	ClaimToken     string `json:"claim_token"`
	Status         string `json:"status"`
	Verdict        string `json:"verdict"`
	Summary        string `json:"summary"`
	FailureClass   string `json:"failure_class"`
	TerminalReason string `json:"terminal_reason"`
	RequestReplan  bool   `json:"request_replan"`
}

type CheckpointOption struct {
	ID          string `json:"id" validate:"required"`
	Kind        string `json:"kind" validate:"required"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type CheckpointDefaultAction struct {
	Mode          string `json:"mode" validate:"required"`
	OptionID      string `json:"option_id" validate:"required"`
	FreeformInput string `json:"freeform_input,omitempty"`
}

type CheckpointResumePolicy struct {
	ResumeMode string `json:"resume_mode" validate:"required"`
}

type HumanCheckpoint struct {
	ID                       string                   `json:"id"`
	RunID                    string                   `json:"run_id"`
	TaskID                   string                   `json:"task_id"`
	BlocksRun                bool                     `json:"blocks_run"`
	PlannerEpoch             uint64                   `json:"planner_epoch"`
	SupersededByPlannerEpoch uint64                   `json:"superseded_by_planner_epoch,omitempty"`
	Status                   string                   `json:"status"`
	StatusVersion            uint64                   `json:"status_version"`
	Question                 string                   `json:"question"`
	Options                  []CheckpointOption       `json:"options"`
	DefaultAction            *CheckpointDefaultAction `json:"default_action,omitempty"`
	ResumePolicy             *CheckpointResumePolicy  `json:"resume_policy,omitempty"`
	TimeoutAt                time.Time                `json:"timeout_at,omitempty"`
	ResolvedBy               string                   `json:"resolved_by,omitempty"`
	ResolvedMode             string                   `json:"resolved_mode,omitempty"`
	ResolvedOptionID         string                   `json:"resolved_option_id,omitempty"`
	ResolvedFreeformInput    string                   `json:"resolved_freeform_input,omitempty"`
	ResolvedAt               time.Time                `json:"resolved_at,omitempty"`
	Metadata                 map[string]any           `json:"metadata"`
	CreatedAt                time.Time                `json:"created_at"`
	UpdatedAt                time.Time                `json:"updated_at"`
}

type Artifact struct {
	ID          string         `json:"id"`
	RunID       string         `json:"run_id"`
	TaskID      string         `json:"task_id"`
	AttemptID   string         `json:"attempt_id,omitempty"`
	Kind        string         `json:"kind"`
	URI         string         `json:"uri"`
	Version     string         `json:"version"`
	Digest      string         `json:"digest"`
	ContentType string         `json:"content_type,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
}

type RunEvent struct {
	ID               string         `json:"id"`
	RunID            string         `json:"run_id"`
	TaskID           string         `json:"task_id,omitempty"`
	AttemptID        string         `json:"attempt_id,omitempty"`
	CheckpointID     string         `json:"checkpoint_id,omitempty"`
	Seq              uint64         `json:"seq"`
	AggregateType    string         `json:"aggregate_type"`
	AggregateID      string         `json:"aggregate_id"`
	AggregateVersion uint64         `json:"aggregate_version"`
	Type             string         `json:"type"`
	CausationEventID string         `json:"causation_event_id,omitempty"`
	CorrelationID    string         `json:"correlation_id,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key,omitempty"`
	Payload          map[string]any `json:"payload"`
	CreatedAt        time.Time      `json:"created_at"`
	PublishedAt      time.Time      `json:"published_at,omitempty"`
}

type ResolveCheckpointResult struct {
	CheckpointID string `json:"checkpoint_id"`
	SnapshotSeq  uint64 `json:"snapshot_seq"`
}

type CheckpointResolution struct {
	Mode           string         `json:"mode" validate:"required"`
	OptionID       string         `json:"option_id"`
	FreeformInput  string         `json:"freeform_input"`
	Metadata       map[string]any `json:"metadata"`
	IdempotencyKey string         `json:"idempotency_key" validate:"required"`
}

type CreateHumanCheckpointRequest struct {
	RunID          string                   `json:"run_id" swaggerignore:"true"`
	TaskID         string                   `json:"task_id" swaggerignore:"true"`
	BlocksRun      bool                     `json:"blocks_run"`
	Question       string                   `json:"question" validate:"required"`
	Options        []CheckpointOption       `json:"options" validate:"required"`
	DefaultAction  *CheckpointDefaultAction `json:"default_action"`
	ResumePolicy   *CheckpointResumePolicy  `json:"resume_policy"`
	TimeoutAt      time.Time                `json:"timeout_at"`
	Metadata       map[string]any           `json:"metadata"`
	IdempotencyKey string                   `json:"idempotency_key" validate:"required"`
}

type CommitArtifactRequest struct {
	RunID          string         `json:"run_id" validate:"required"`
	TaskID         string         `json:"task_id" validate:"required"`
	AttemptID      string         `json:"attempt_id"`
	Kind           string         `json:"kind" validate:"required"`
	URI            string         `json:"uri" validate:"required"`
	Version        string         `json:"version" validate:"required"`
	Digest         string         `json:"digest" validate:"required"`
	ContentType    string         `json:"content_type"`
	Summary        string         `json:"summary"`
	Metadata       map[string]any `json:"metadata"`
	IdempotencyKey string         `json:"idempotency_key" validate:"required"`
}

type CommitArtifactResult struct {
	Artifact    Artifact `json:"artifact"`
	SnapshotSeq uint64   `json:"snapshot_seq"`
}
