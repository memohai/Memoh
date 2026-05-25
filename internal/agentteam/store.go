package agentteam

import (
	"context"
	"time"
)

// Store abstracts persistence for the agentteam domain. Both Postgres and
// SQLite backends implement this interface so the service stays backend
// agnostic.
type Store interface {
	// Teams
	CreateTeam(ctx context.Context, input CreateTeamInput) (Team, error)
	GetTeam(ctx context.Context, id string) (Team, error)
	GetTeamForOwner(ctx context.Context, id, ownerUserID string) (Team, error)
	ListTeamsByOwner(ctx context.Context, ownerUserID string) ([]Team, error)
	ListAllTeamsByOwner(ctx context.Context, ownerUserID string) ([]Team, error)
	ListAllTeams(ctx context.Context) ([]Team, error)
	ListTeamsForBot(ctx context.Context, botID string) ([]Team, error)
	UpdateTeam(ctx context.Context, id string, input UpdateTeamInput) (Team, error)
	ArchiveTeam(ctx context.Context, id string) (Team, error)
	DeleteTeam(ctx context.Context, id string) error

	// Members
	AddMember(ctx context.Context, input CreateMemberInput) (Member, error)
	GetMember(ctx context.Context, id string) (Member, error)
	GetMemberByBot(ctx context.Context, teamID, botID string) (Member, error)
	GetMemberByUser(ctx context.Context, teamID, userID string) (Member, error)
	ListMembers(ctx context.Context, teamID string) ([]Member, error)
	UpdateMember(ctx context.Context, id string, input UpdateMemberInput) (Member, error)
	DeleteMember(ctx context.Context, id string) error

	// Issues
	CreateIssue(ctx context.Context, input CreateIssueInput) (Issue, error)
	GetIssue(ctx context.Context, id string) (Issue, error)
	GetIssueInTeam(ctx context.Context, id, teamID string) (Issue, error)
	GetIssueByNumber(ctx context.Context, teamID string, number int32) (Issue, error)
	ListIssuesByTeam(ctx context.Context, teamID string) ([]Issue, error)
	ListOpenIssuesByTeam(ctx context.Context, teamID string) ([]Issue, error)
	ListIssuesForOwner(ctx context.Context, ownerUserID string) ([]Issue, error)
	UpdateIssue(ctx context.Context, id string, input UpdateIssueInput) (Issue, error)
	SetIssueAssignee(ctx context.Context, id string, input AssignIssueInput) (Issue, error)
	DeleteIssue(ctx context.Context, id string) error

	// Comments
	CreateComment(ctx context.Context, input CreateCommentInput) (Comment, error)
	GetComment(ctx context.Context, id string) (Comment, error)
	ListComments(ctx context.Context, issueID string) ([]Comment, error)
	TouchIssueAfterComment(ctx context.Context, issueID string) error
	DeleteComment(ctx context.Context, id string) error

	// Handoffs
	CreateHandoff(ctx context.Context, input CreateHandoffInput) (Handoff, error)
	GetHandoff(ctx context.Context, id string) (Handoff, error)
	ListPendingHandoffsToBotForIssue(ctx context.Context, botID, issueID string) ([]Handoff, error)
	ListPendingHandoffsToBot(ctx context.Context, botID string) ([]Handoff, error)
	ListPendingReturnsForBotInIssue(ctx context.Context, fromBotID, issueID string) ([]Handoff, error)
	ListHandoffsByIssue(ctx context.Context, issueID string) ([]Handoff, error)
	MarkHandoffDispatched(ctx context.Context, id string, targetSessionID string) (Handoff, error)
	MarkHandoffRunning(ctx context.Context, id string, targetSessionID string) (Handoff, error)
	CompleteHandoff(ctx context.Context, id string, resultCommentID string) (Handoff, error)
	FailHandoff(ctx context.Context, id string, failureReason string) (Handoff, error)
	SetHandoffReturn(ctx context.Context, id string, returnHandoffID string) (Handoff, error)
	CancelHandoff(ctx context.Context, id string, failureReason string) (Handoff, error)

	// File locks
	AcquireFileLock(ctx context.Context, input AcquireLockInput) (FileLock, error)
	GetFileLockByID(ctx context.Context, id string) (FileLock, error)
	GetFileLock(ctx context.Context, teamID, path string, scope LockScope) (FileLock, error)
	ListFileLocks(ctx context.Context, teamID string) ([]FileLock, error)
	ListActiveFileLocks(ctx context.Context, teamID string) ([]FileLock, error)
	RefreshFileLock(ctx context.Context, id string, expiresAt time.Time) (FileLock, error)
	ReleaseFileLock(ctx context.Context, id string) error
	ReleaseExpiredFileLocks(ctx context.Context) error
}
