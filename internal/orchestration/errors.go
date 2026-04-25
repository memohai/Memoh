package orchestration

import "errors"

var (
	ErrInvalidControlIdentity      = errors.New("invalid control identity")
	ErrInvalidArgument             = errors.New("invalid orchestration argument")
	ErrAccessDenied                = errors.New("orchestration access denied")
	ErrRunNotFound                 = errors.New("orchestration run not found")
	ErrTaskNotFound                = errors.New("orchestration task not found")
	ErrTaskImmutable               = errors.New("task does not accept human checkpoint mutation")
	ErrTaskCheckpointUnsupported   = errors.New("task does not support human checkpoint in current state")
	ErrTaskAlreadyWaitingHuman     = errors.New("task already waiting on a human checkpoint")
	ErrCheckpointNotFound          = errors.New("orchestration checkpoint not found")
	ErrIdempotencyConflict         = errors.New("orchestration idempotency conflict")
	ErrIdempotencyIncomplete       = errors.New("orchestration idempotency record incomplete")
	ErrInvalidCursor               = errors.New("invalid orchestration cursor")
	ErrRunImmutable                = errors.New("run does not accept external mutation")
	ErrRunBarrierAlreadyOpen       = errors.New("run already has an open run-blocking checkpoint")
	ErrRunBarrierUnsupported       = errors.New("run-blocking checkpoint cannot pause active sibling tasks")
	ErrCheckpointNotOpen           = errors.New("checkpoint is not open")
	ErrInvalidCheckpointResolution = errors.New("invalid checkpoint resolution")
)
