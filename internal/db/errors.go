package db

import "errors"

var (
	ErrNotFound             = errors.New("database record not found")
	ErrLastActiveAdmin      = errors.New("team must retain at least one active admin")
	ErrWorkspaceTargetInUse = errors.New("workspace target is referenced by a workdir")
)
