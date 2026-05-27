// Package agentteam implements Agent Team collaboration: teams, members,
// issues, comments, agent-to-agent handoffs and shared filesystem locks.
package agentteam

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// MemberType enumerates supported team member kinds.
type MemberType string

const (
	MemberBot  MemberType = "bot"
	MemberUser MemberType = "user"
)

// ActorType identifies the actor that performs an action (issue/comment/handoff).
type ActorType string

const (
	ActorBot    ActorType = "bot"
	ActorUser   ActorType = "user"
	ActorSystem ActorType = "system"
)

// IssueStatus enumerates Issue lifecycle states.
type IssueStatus string

const (
	StatusBacklog    IssueStatus = "backlog"
	StatusTodo       IssueStatus = "todo"
	StatusInProgress IssueStatus = "in_progress"
	StatusBlocked    IssueStatus = "blocked"
	StatusReview     IssueStatus = "review"
	StatusDone       IssueStatus = "done"
	StatusCancelled  IssueStatus = "cancelled"
)

// HandoffStatus enumerates Handoff lifecycle states.
type HandoffStatus string

const (
	HandoffPending    HandoffStatus = "pending"
	HandoffDispatched HandoffStatus = "dispatched"
	HandoffRunning    HandoffStatus = "running"
	HandoffCompleted  HandoffStatus = "completed"
	HandoffFailed     HandoffStatus = "failed"
	HandoffCancelled  HandoffStatus = "cancelled"
	HandoffReturned   HandoffStatus = "returned"
)

// LockScope enumerates supported shared-fs lock granularities.
type LockScope string

const (
	LockFile   LockScope = "file"
	LockPrefix LockScope = "prefix"
)

// Team is the domain representation of an agent_teams row.
type Team struct {
	ID            string
	OwnerUserID   string
	Name          string
	Description   string
	AvatarURL     string
	SharedDirName string
	Instructions  string
	Metadata      []byte
	ArchivedAt    time.Time
	HasArchivedAt bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Member is the domain representation of an agent_team_members row.
//
// DisplayName is read-only and is resolved at query time from the joined
// bots.display_name (for bot members) or users.display_name (for user
// members). It is intentionally NOT a column on agent_team_members: the
// canonical name lives on the bot / user entity. Team-specific overrides
// belong in Role / Instructions / Metadata.
type Member struct {
	ID           string
	TeamID       string
	MemberType   MemberType
	BotID        string
	UserID       string
	Role         string
	Instructions string
	Metadata     []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// DisplayName is derived from bots.display_name / users.display_name.
	// Read-only — writes are ignored by the store.
	DisplayName string
}

// Issue is the domain representation of a team_issues row.
type Issue struct {
	ID              string
	TeamID          string
	Number          int32
	Title           string
	Description     string
	Status          IssueStatus
	AssigneeType    string
	AssigneeBotID   string
	AssigneeUserID  string
	CreatedByType   ActorType
	CreatedByBotID  string
	CreatedByUserID string
	ParentIssueID   string
	Metadata        []byte
	ClosedAt        time.Time
	HasClosedAt     bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Comment is the domain representation of a team_issue_comments row.
//
// SourceSessionID is a read-only convenience that is parsed from the
// comment's metadata JSON (key: `source_session_id`). It carries the
// LLM session id of the bot that authored the comment, which the
// dispatcher uses to route a return handoff back to the same session.
// User-authored comments (or system-posted comments) carry an empty
// value.
type Comment struct {
	ID              string
	IssueID         string
	TeamID          string
	ParentCommentID string
	AuthorType      ActorType
	AuthorBotID     string
	AuthorUserID    string
	Content         string
	Metadata        []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// SourceSessionID is derived from Metadata.source_session_id. Setting
	// it directly is ignored by the store — write through metadata.
	SourceSessionID string
}

// Handoff is the domain representation of an agent_handoffs row.
type Handoff struct {
	ID               string
	TeamID           string
	IssueID          string
	FromActorType    ActorType
	FromBotID        string
	FromUserID       string
	ToBotID          string
	TriggerCommentID string
	SourceSessionID  string
	TargetSessionID  string
	ResultCommentID  string
	ReturnHandoffID  string
	Status           HandoffStatus
	FailureReason    string
	Metadata         []byte
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      time.Time
	HasCompletedAt   bool
}

// FileLock is the domain representation of a team_file_locks row.
type FileLock struct {
	ID          string
	TeamID      string
	Path        string
	Scope       LockScope
	OwnerType   ActorType
	OwnerBotID  string
	OwnerUserID string
	IssueID     string
	SessionID   string
	HandoffID   string
	AcquiredAt  time.Time
	RefreshedAt time.Time
	ExpiresAt   time.Time
	Metadata    []byte
}

// CreateTeamInput captures the fields needed to create a team.
type CreateTeamInput struct {
	OwnerUserID   string
	Name          string
	Description   string
	AvatarURL     string
	SharedDirName string
	Instructions  string
	Metadata      []byte
}

// UpdateTeamInput captures the optional fields of an UpdateTeam call.
// Empty string / nil means "leave unchanged".
type UpdateTeamInput struct {
	Name          *string
	Description   *string
	AvatarURL     *string
	SharedDirName *string
	Instructions  *string
	Metadata      []byte
}

// CreateMemberInput captures the fields needed to add a team member.
// Note: there is no DisplayName field — the canonical name lives on the
// bot / user entity and is JOINed at read time.
type CreateMemberInput struct {
	TeamID       string
	MemberType   MemberType
	BotID        string
	UserID       string
	Role         string
	Instructions string
	Metadata     []byte
}

// UpdateMemberInput captures the optional fields of UpdateMember.
type UpdateMemberInput struct {
	Role         *string
	Instructions *string
	Metadata     []byte
}

// CreateIssueInput captures the fields needed to create an issue.
type CreateIssueInput struct {
	TeamID          string
	Title           string
	Description     string
	Status          IssueStatus
	AssigneeType    string
	AssigneeBotID   string
	AssigneeUserID  string
	CreatedByType   ActorType
	CreatedByBotID  string
	CreatedByUserID string
	ParentIssueID   string
	Metadata        []byte
}

// UpdateIssueInput captures the optional fields of UpdateIssue.
type UpdateIssueInput struct {
	Title          *string
	Description    *string
	Status         *IssueStatus
	AssigneeType   *string
	AssigneeBotID  *string
	AssigneeUserID *string
	Metadata       []byte
}

// AssignIssueInput captures the fields needed to (re)assign an issue.
type AssignIssueInput struct {
	AssigneeType   string
	AssigneeBotID  string
	AssigneeUserID string
}

// CreateCommentInput captures the fields needed to add a comment.
//
// SourceSessionID, when non-empty, is the LLM session that authored the
// comment. It is recorded in the comment's metadata under the key
// `source_session_id` so the dispatcher can route a return handoff back
// to that exact session (rather than spinning up a new per-issue session
// on the originating bot). For human-authored comments this stays empty.
type CreateCommentInput struct {
	IssueID         string
	TeamID          string
	ParentCommentID string
	AuthorType      ActorType
	AuthorBotID     string
	AuthorUserID    string
	Content         string
	Metadata        []byte
	SourceSessionID string
}

// CreateHandoffInput captures the fields needed to create a handoff.
//
// SourceSessionID is the from-side session (e.g. the session A was in
// when A @mentioned B). TargetSessionID, when non-empty, pins the
// to-side session at creation time — used by the return path so the
// delegator lands back in their original session instead of a per-issue
// scratch session. When empty, the resolver creates / reuses a
// per-issue session for the target bot.
type CreateHandoffInput struct {
	TeamID           string
	IssueID          string
	FromActorType    ActorType
	FromBotID        string
	FromUserID       string
	ToBotID          string
	TriggerCommentID string
	SourceSessionID  string
	TargetSessionID  string
	Status           HandoffStatus
	Metadata         []byte
}

// AcquireLockInput captures the fields needed to acquire a shared-fs lock.
type AcquireLockInput struct {
	TeamID      string
	Path        string
	Scope       LockScope
	OwnerType   ActorType
	OwnerBotID  string
	OwnerUserID string
	IssueID     string
	SessionID   string
	HandoffID   string
	TTL         time.Duration
	Metadata    []byte
}

// Common domain errors surfaced by service / store layers.
var (
	ErrNotFound      = errors.New("agentteam: not found")
	ErrAlreadyExists = errors.New("agentteam: already exists")
	ErrInvalidInput  = errors.New("agentteam: invalid input")
	ErrLockHeld      = errors.New("agentteam: lock already held")
	ErrForbidden     = errors.New("agentteam: forbidden")
)

// CommentMetadataKeySourceSession is the JSON key under which the
// authoring bot's LLM session id is recorded inside a comment's
// metadata. Store implementations read/write this key so callers can
// stay decoupled from the JSON layout.
const CommentMetadataKeySourceSession = "source_session_id"

// MergeCommentMetadata produces the JSON byte payload to store on a
// comment row, merging the explicit Metadata payload with optional
// well-known keys like SourceSessionID. The merge is shallow and
// preserves any unrelated keys the caller supplied.
//
// The function is shared by PG / SQLite stores so all writes converge
// on the same JSON shape.
func MergeCommentMetadata(metadata []byte, sourceSessionID string) ([]byte, error) {
	obj := map[string]any{}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &obj); err != nil {
			// Caller-supplied bytes are not a JSON object — start fresh
			// rather than fail the comment write. Comments must always
			// land; metadata is best-effort.
			obj = map[string]any{}
		}
	}
	if strings.TrimSpace(sourceSessionID) != "" {
		obj[CommentMetadataKeySourceSession] = sourceSessionID
	}
	if len(obj) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(obj)
}

// ReadSourceSessionFromMetadata extracts the source_session_id from a
// comment's stored metadata JSON. Returns "" when absent / unparsable.
func ReadSourceSessionFromMetadata(metadata []byte) string {
	if len(metadata) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(metadata, &obj); err != nil {
		return ""
	}
	raw, ok := obj[CommentMetadataKeySourceSession]
	if !ok {
		return ""
	}
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}
