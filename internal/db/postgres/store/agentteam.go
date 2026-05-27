package postgresstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/agentteam"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// AgentTeamStore implements agentteam.Store on top of the Postgres sqlc queries.
type AgentTeamStore struct {
	queries *dbsqlc.Queries
}

func NewAgentTeamStore(queries *dbsqlc.Queries) *AgentTeamStore {
	return &AgentTeamStore{queries: queries}
}

func mapAgentTeamErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.Join(agentteam.ErrNotFound, db.ErrNotFound)
	}
	return err
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func ptrText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func optionalUUID(value string) pgtype.UUID {
	return db.ParseUUIDOrEmpty(value)
}

func optionalUUIDFromPtr(value *string) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return db.ParseUUIDOrEmpty(*value)
}

func pgTimestamptz(t time.Time, valid bool) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: valid}
}

// ── Conversions ─────────────────────────────────────────────────────────────

func teamRecord(row dbsqlc.AgentTeam) agentteam.Team {
	rec := agentteam.Team{
		ID:            row.ID.String(),
		OwnerUserID:   row.OwnerUserID.String(),
		Name:          row.Name,
		Description:   row.Description,
		AvatarURL:     row.AvatarUrl,
		SharedDirName: row.SharedDirName,
		Instructions:  row.Instructions,
		Metadata:      row.Metadata,
	}
	if row.CreatedAt.Valid {
		rec.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		rec.UpdatedAt = row.UpdatedAt.Time
	}
	if row.ArchivedAt.Valid {
		rec.ArchivedAt = row.ArchivedAt.Time
		rec.HasArchivedAt = true
	}
	return rec
}

func teamRecords(rows []dbsqlc.AgentTeam) []agentteam.Team {
	out := make([]agentteam.Team, 0, len(rows))
	for _, row := range rows {
		out = append(out, teamRecord(row))
	}
	return out
}

// memberFields is the common subset of fields shared by the various member
// row shapes returned by sqlc (some include the JOINed display name, some
// don't). Using a generic helper keeps the conversion code DRY.
type memberFields struct {
	ID                  pgtype.UUID
	TeamID              pgtype.UUID
	MemberType          string
	BotID               pgtype.UUID
	UserID              pgtype.UUID
	Role                string
	Instructions        string
	Metadata            []byte
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
	ResolvedDisplayName string
}

func memberFromFields(f memberFields) agentteam.Member {
	rec := agentteam.Member{
		ID:           f.ID.String(),
		TeamID:       f.TeamID.String(),
		MemberType:   agentteam.MemberType(f.MemberType),
		Role:         f.Role,
		Instructions: f.Instructions,
		Metadata:     f.Metadata,
		DisplayName:  f.ResolvedDisplayName,
	}
	if f.BotID.Valid {
		rec.BotID = f.BotID.String()
	}
	if f.UserID.Valid {
		rec.UserID = f.UserID.String()
	}
	if f.CreatedAt.Valid {
		rec.CreatedAt = f.CreatedAt.Time
	}
	if f.UpdatedAt.Valid {
		rec.UpdatedAt = f.UpdatedAt.Time
	}
	return rec
}

// memberFromCreateRow handles the bare AgentTeamMember row returned by
// AddMember / UpdateMember (no JOIN, so DisplayName must be derived).
func memberFromCreateRow(row dbsqlc.AgentTeamMember) agentteam.Member {
	return memberFromFields(memberFields{
		ID:           row.ID,
		TeamID:       row.TeamID,
		MemberType:   row.MemberType,
		BotID:        row.BotID,
		UserID:       row.UserID,
		Role:         row.Role,
		Instructions: row.Instructions,
		Metadata:     row.Metadata,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	})
}

func memberFromGet(row dbsqlc.GetAgentTeamMemberRow) agentteam.Member {
	return memberFromFields(memberFields{
		ID:                  row.ID,
		TeamID:              row.TeamID,
		MemberType:          row.MemberType,
		BotID:               row.BotID,
		UserID:              row.UserID,
		Role:                row.Role,
		Instructions:        row.Instructions,
		Metadata:            row.Metadata,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		ResolvedDisplayName: row.ResolvedDisplayName,
	})
}

func memberFromGetByBot(row dbsqlc.GetAgentTeamMemberByBotRow) agentteam.Member {
	return memberFromFields(memberFields{
		ID:                  row.ID,
		TeamID:              row.TeamID,
		MemberType:          row.MemberType,
		BotID:               row.BotID,
		UserID:              row.UserID,
		Role:                row.Role,
		Instructions:        row.Instructions,
		Metadata:            row.Metadata,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		ResolvedDisplayName: row.ResolvedDisplayName,
	})
}

func memberFromGetByUser(row dbsqlc.GetAgentTeamMemberByUserRow) agentteam.Member {
	return memberFromFields(memberFields{
		ID:                  row.ID,
		TeamID:              row.TeamID,
		MemberType:          row.MemberType,
		BotID:               row.BotID,
		UserID:              row.UserID,
		Role:                row.Role,
		Instructions:        row.Instructions,
		Metadata:            row.Metadata,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		ResolvedDisplayName: row.ResolvedDisplayName,
	})
}

func memberFromList(row dbsqlc.ListAgentTeamMembersRow) agentteam.Member {
	return memberFromFields(memberFields{
		ID:                  row.ID,
		TeamID:              row.TeamID,
		MemberType:          row.MemberType,
		BotID:               row.BotID,
		UserID:              row.UserID,
		Role:                row.Role,
		Instructions:        row.Instructions,
		Metadata:            row.Metadata,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		ResolvedDisplayName: row.ResolvedDisplayName,
	})
}

func issueRecord(row dbsqlc.TeamIssue) agentteam.Issue {
	rec := agentteam.Issue{
		ID:            row.ID.String(),
		TeamID:        row.TeamID.String(),
		Number:        row.Number,
		Title:         row.Title,
		Description:   row.Description,
		Status:        agentteam.IssueStatus(row.Status),
		CreatedByType: agentteam.ActorType(row.CreatedByType),
		Metadata:      row.Metadata,
	}
	if row.AssigneeType.Valid {
		rec.AssigneeType = row.AssigneeType.String
	}
	if row.AssigneeBotID.Valid {
		rec.AssigneeBotID = row.AssigneeBotID.String()
	}
	if row.AssigneeUserID.Valid {
		rec.AssigneeUserID = row.AssigneeUserID.String()
	}
	if row.CreatedByBotID.Valid {
		rec.CreatedByBotID = row.CreatedByBotID.String()
	}
	if row.CreatedByUserID.Valid {
		rec.CreatedByUserID = row.CreatedByUserID.String()
	}
	if row.ParentIssueID.Valid {
		rec.ParentIssueID = row.ParentIssueID.String()
	}
	if row.ClosedAt.Valid {
		rec.ClosedAt = row.ClosedAt.Time
		rec.HasClosedAt = true
	}
	if row.CreatedAt.Valid {
		rec.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		rec.UpdatedAt = row.UpdatedAt.Time
	}
	return rec
}

func issueRecords(rows []dbsqlc.TeamIssue) []agentteam.Issue {
	out := make([]agentteam.Issue, 0, len(rows))
	for _, row := range rows {
		out = append(out, issueRecord(row))
	}
	return out
}

func commentRecord(row dbsqlc.TeamIssueComment) agentteam.Comment {
	rec := agentteam.Comment{
		ID:              row.ID.String(),
		IssueID:         row.IssueID.String(),
		TeamID:          row.TeamID.String(),
		AuthorType:      agentteam.ActorType(row.AuthorType),
		Content:         row.Content,
		Metadata:        row.Metadata,
		SourceSessionID: agentteam.ReadSourceSessionFromMetadata(row.Metadata),
	}
	if row.ParentCommentID.Valid {
		rec.ParentCommentID = row.ParentCommentID.String()
	}
	if row.AuthorBotID.Valid {
		rec.AuthorBotID = row.AuthorBotID.String()
	}
	if row.AuthorUserID.Valid {
		rec.AuthorUserID = row.AuthorUserID.String()
	}
	if row.CreatedAt.Valid {
		rec.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		rec.UpdatedAt = row.UpdatedAt.Time
	}
	return rec
}

func handoffRecord(row dbsqlc.AgentHandoff) agentteam.Handoff {
	rec := agentteam.Handoff{
		ID:            row.ID.String(),
		TeamID:        row.TeamID.String(),
		FromActorType: agentteam.ActorType(row.FromActorType),
		ToBotID:       row.ToBotID.String(),
		Status:        agentteam.HandoffStatus(row.Status),
		FailureReason: row.FailureReason,
		Metadata:      row.Metadata,
	}
	if row.IssueID.Valid {
		rec.IssueID = row.IssueID.String()
	}
	if row.FromBotID.Valid {
		rec.FromBotID = row.FromBotID.String()
	}
	if row.FromUserID.Valid {
		rec.FromUserID = row.FromUserID.String()
	}
	if row.TriggerCommentID.Valid {
		rec.TriggerCommentID = row.TriggerCommentID.String()
	}
	if row.SourceSessionID.Valid {
		rec.SourceSessionID = row.SourceSessionID.String()
	}
	if row.TargetSessionID.Valid {
		rec.TargetSessionID = row.TargetSessionID.String()
	}
	if row.ResultCommentID.Valid {
		rec.ResultCommentID = row.ResultCommentID.String()
	}
	if row.ReturnHandoffID.Valid {
		rec.ReturnHandoffID = row.ReturnHandoffID.String()
	}
	if row.CreatedAt.Valid {
		rec.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		rec.UpdatedAt = row.UpdatedAt.Time
	}
	if row.CompletedAt.Valid {
		rec.CompletedAt = row.CompletedAt.Time
		rec.HasCompletedAt = true
	}
	return rec
}

func lockRecord(row dbsqlc.TeamFileLock) agentteam.FileLock {
	rec := agentteam.FileLock{
		ID:        row.ID.String(),
		TeamID:    row.TeamID.String(),
		Path:      row.Path,
		Scope:     agentteam.LockScope(row.Scope),
		OwnerType: agentteam.ActorType(row.OwnerType),
		Metadata:  row.Metadata,
	}
	if row.OwnerBotID.Valid {
		rec.OwnerBotID = row.OwnerBotID.String()
	}
	if row.OwnerUserID.Valid {
		rec.OwnerUserID = row.OwnerUserID.String()
	}
	if row.IssueID.Valid {
		rec.IssueID = row.IssueID.String()
	}
	if row.SessionID.Valid {
		rec.SessionID = row.SessionID.String()
	}
	if row.HandoffID.Valid {
		rec.HandoffID = row.HandoffID.String()
	}
	if row.AcquiredAt.Valid {
		rec.AcquiredAt = row.AcquiredAt.Time
	}
	if row.RefreshedAt.Valid {
		rec.RefreshedAt = row.RefreshedAt.Time
	}
	if row.ExpiresAt.Valid {
		rec.ExpiresAt = row.ExpiresAt.Time
	}
	return rec
}

// ── Teams ────────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateTeam(ctx context.Context, input agentteam.CreateTeamInput) (agentteam.Team, error) {
	ownerID, err := db.ParseUUID(input.OwnerUserID)
	if err != nil {
		return agentteam.Team{}, err
	}
	row, err := s.queries.CreateAgentTeam(ctx, dbsqlc.CreateAgentTeamParams{
		OwnerUserID:   ownerID,
		Name:          input.Name,
		Description:   input.Description,
		AvatarUrl:     input.AvatarURL,
		SharedDirName: input.SharedDirName,
		Instructions:  input.Instructions,
		Metadata:      jsonOrEmpty(input.Metadata),
	})
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamRecord(row), nil
}

func (s *AgentTeamStore) GetTeam(ctx context.Context, id string) (agentteam.Team, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Team{}, err
	}
	row, err := s.queries.GetAgentTeam(ctx, uid)
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamRecord(row), nil
}

func (s *AgentTeamStore) GetTeamForOwner(ctx context.Context, id, ownerUserID string) (agentteam.Team, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Team{}, err
	}
	ownerID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return agentteam.Team{}, err
	}
	row, err := s.queries.GetAgentTeamForOwner(ctx, dbsqlc.GetAgentTeamForOwnerParams{ID: uid, OwnerUserID: ownerID})
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamRecord(row), nil
}

func (s *AgentTeamStore) ListTeamsByOwner(ctx context.Context, ownerUserID string) ([]agentteam.Team, error) {
	ownerID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListAgentTeamsByOwner(ctx, ownerID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return teamRecords(rows), nil
}

func (s *AgentTeamStore) ListAllTeamsByOwner(ctx context.Context, ownerUserID string) ([]agentteam.Team, error) {
	ownerID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListAllAgentTeamsByOwner(ctx, ownerID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return teamRecords(rows), nil
}

func (s *AgentTeamStore) ListAllTeams(ctx context.Context) ([]agentteam.Team, error) {
	rows, err := s.queries.ListAllAgentTeams(ctx)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return teamRecords(rows), nil
}

func (s *AgentTeamStore) ListTeamsForBot(ctx context.Context, botID string) ([]agentteam.Team, error) {
	bID := db.ParseUUIDOrEmpty(botID)
	rows, err := s.queries.ListAgentTeamsForBot(ctx, bID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return teamRecords(rows), nil
}

func (s *AgentTeamStore) UpdateTeam(ctx context.Context, id string, input agentteam.UpdateTeamInput) (agentteam.Team, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Team{}, err
	}
	row, err := s.queries.UpdateAgentTeam(ctx, dbsqlc.UpdateAgentTeamParams{
		Name:          ptrText(input.Name),
		Description:   ptrText(input.Description),
		AvatarUrl:     ptrText(input.AvatarURL),
		SharedDirName: ptrText(input.SharedDirName),
		Instructions:  ptrText(input.Instructions),
		Metadata:      input.Metadata,
		ID:            uid,
	})
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamRecord(row), nil
}

func (s *AgentTeamStore) ArchiveTeam(ctx context.Context, id string) (agentteam.Team, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Team{}, err
	}
	row, err := s.queries.ArchiveAgentTeam(ctx, uid)
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamRecord(row), nil
}

func (s *AgentTeamStore) DeleteTeam(ctx context.Context, id string) error {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return mapAgentTeamErr(s.queries.DeleteAgentTeam(ctx, uid))
}

// ── Members ──────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) AddMember(ctx context.Context, input agentteam.CreateMemberInput) (agentteam.Member, error) {
	teamID, err := db.ParseUUID(input.TeamID)
	if err != nil {
		return agentteam.Member{}, err
	}
	row, err := s.queries.AddAgentTeamMember(ctx, dbsqlc.AddAgentTeamMemberParams{
		TeamID:       teamID,
		MemberType:   string(input.MemberType),
		BotID:        optionalUUID(input.BotID),
		UserID:       optionalUUID(input.UserID),
		Role:         input.Role,
		Instructions: input.Instructions,
		Metadata:     jsonOrEmpty(input.Metadata),
	})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	// AddAgentTeamMember returns the bare row without the JOIN, so re-read
	// the member through the JOINing query to populate DisplayName.
	full, err := s.queries.GetAgentTeamMember(ctx, row.ID)
	if err == nil {
		return memberFromGet(full), nil
	}
	return memberFromCreateRow(row), nil
}

func (s *AgentTeamStore) GetMember(ctx context.Context, id string) (agentteam.Member, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Member{}, err
	}
	row, err := s.queries.GetAgentTeamMember(ctx, uid)
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	return memberFromGet(row), nil
}

func (s *AgentTeamStore) GetMemberByBot(ctx context.Context, teamID, botID string) (agentteam.Member, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return agentteam.Member{}, err
	}
	bID, err := db.ParseUUID(botID)
	if err != nil {
		return agentteam.Member{}, err
	}
	row, err := s.queries.GetAgentTeamMemberByBot(ctx, dbsqlc.GetAgentTeamMemberByBotParams{TeamID: tID, BotID: bID})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	return memberFromGetByBot(row), nil
}

func (s *AgentTeamStore) GetMemberByUser(ctx context.Context, teamID, userID string) (agentteam.Member, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return agentteam.Member{}, err
	}
	uID, err := db.ParseUUID(userID)
	if err != nil {
		return agentteam.Member{}, err
	}
	row, err := s.queries.GetAgentTeamMemberByUser(ctx, dbsqlc.GetAgentTeamMemberByUserParams{TeamID: tID, UserID: uID})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	return memberFromGetByUser(row), nil
}

func (s *AgentTeamStore) ListMembers(ctx context.Context, teamID string) ([]agentteam.Member, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListAgentTeamMembers(ctx, tID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Member, 0, len(rows))
	for _, row := range rows {
		out = append(out, memberFromList(row))
	}
	return out, nil
}

func (s *AgentTeamStore) UpdateMember(ctx context.Context, id string, input agentteam.UpdateMemberInput) (agentteam.Member, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Member{}, err
	}
	row, err := s.queries.UpdateAgentTeamMember(ctx, dbsqlc.UpdateAgentTeamMemberParams{
		Role:         ptrText(input.Role),
		Instructions: ptrText(input.Instructions),
		Metadata:     input.Metadata,
		ID:           uid,
	})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	// UpdateAgentTeamMember returns the bare row; re-read for DisplayName.
	full, gerr := s.queries.GetAgentTeamMember(ctx, row.ID)
	if gerr == nil {
		return memberFromGet(full), nil
	}
	return memberFromCreateRow(row), nil
}

func (s *AgentTeamStore) DeleteMember(ctx context.Context, id string) error {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return mapAgentTeamErr(s.queries.DeleteAgentTeamMember(ctx, uid))
}

// ── Issues ───────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateIssue(ctx context.Context, input agentteam.CreateIssueInput) (agentteam.Issue, error) {
	teamID, err := db.ParseUUID(input.TeamID)
	if err != nil {
		return agentteam.Issue{}, err
	}
	status := input.Status
	if status == "" {
		status = agentteam.StatusTodo
	}
	row, err := s.queries.CreateTeamIssue(ctx, dbsqlc.CreateTeamIssueParams{
		TeamID:          teamID,
		Title:           input.Title,
		Description:     input.Description,
		Status:          string(status),
		AssigneeType:    pgtype.Text{String: input.AssigneeType, Valid: input.AssigneeType != ""},
		AssigneeBotID:   optionalUUID(input.AssigneeBotID),
		AssigneeUserID:  optionalUUID(input.AssigneeUserID),
		CreatedByType:   string(input.CreatedByType),
		CreatedByBotID:  optionalUUID(input.CreatedByBotID),
		CreatedByUserID: optionalUUID(input.CreatedByUserID),
		ParentIssueID:   optionalUUID(input.ParentIssueID),
		Metadata:        jsonOrEmpty(input.Metadata),
	})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueRecord(row), nil
}

func (s *AgentTeamStore) GetIssue(ctx context.Context, id string) (agentteam.Issue, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Issue{}, err
	}
	row, err := s.queries.GetTeamIssue(ctx, uid)
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueRecord(row), nil
}

func (s *AgentTeamStore) GetIssueInTeam(ctx context.Context, id, teamID string) (agentteam.Issue, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Issue{}, err
	}
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return agentteam.Issue{}, err
	}
	row, err := s.queries.GetTeamIssueInTeam(ctx, dbsqlc.GetTeamIssueInTeamParams{ID: uid, TeamID: tID})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueRecord(row), nil
}

func (s *AgentTeamStore) GetIssueByNumber(ctx context.Context, teamID string, number int32) (agentteam.Issue, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return agentteam.Issue{}, err
	}
	row, err := s.queries.GetTeamIssueByNumber(ctx, dbsqlc.GetTeamIssueByNumberParams{TeamID: tID, Number: number})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueRecord(row), nil
}

func (s *AgentTeamStore) ListIssuesByTeam(ctx context.Context, teamID string) ([]agentteam.Issue, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTeamIssuesByTeam(ctx, tID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return issueRecords(rows), nil
}

func (s *AgentTeamStore) ListOpenIssuesByTeam(ctx context.Context, teamID string) ([]agentteam.Issue, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListOpenTeamIssuesByTeam(ctx, tID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return issueRecords(rows), nil
}

func (s *AgentTeamStore) ListIssuesForOwner(ctx context.Context, ownerUserID string) ([]agentteam.Issue, error) {
	ownerID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTeamIssuesForOwner(ctx, ownerID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	return issueRecords(rows), nil
}

func (s *AgentTeamStore) UpdateIssue(ctx context.Context, id string, input agentteam.UpdateIssueInput) (agentteam.Issue, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Issue{}, err
	}
	var statusText pgtype.Text
	if input.Status != nil {
		statusText = pgtype.Text{String: string(*input.Status), Valid: true}
	}
	row, err := s.queries.UpdateTeamIssue(ctx, dbsqlc.UpdateTeamIssueParams{
		Title:          ptrText(input.Title),
		Description:    ptrText(input.Description),
		Status:         statusText,
		AssigneeType:   ptrText(input.AssigneeType),
		AssigneeBotID:  optionalUUIDFromPtr(input.AssigneeBotID),
		AssigneeUserID: optionalUUIDFromPtr(input.AssigneeUserID),
		Metadata:       input.Metadata,
		ID:             uid,
	})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueRecord(row), nil
}

func (s *AgentTeamStore) SetIssueAssignee(ctx context.Context, id string, input agentteam.AssignIssueInput) (agentteam.Issue, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Issue{}, err
	}
	row, err := s.queries.SetTeamIssueAssignee(ctx, dbsqlc.SetTeamIssueAssigneeParams{
		AssigneeType:   pgtype.Text{String: input.AssigneeType, Valid: input.AssigneeType != ""},
		AssigneeBotID:  optionalUUID(input.AssigneeBotID),
		AssigneeUserID: optionalUUID(input.AssigneeUserID),
		ID:             uid,
	})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueRecord(row), nil
}

func (s *AgentTeamStore) DeleteIssue(ctx context.Context, id string) error {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return mapAgentTeamErr(s.queries.DeleteTeamIssue(ctx, uid))
}

// ── Comments ─────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateComment(ctx context.Context, input agentteam.CreateCommentInput) (agentteam.Comment, error) {
	issueID, err := db.ParseUUID(input.IssueID)
	if err != nil {
		return agentteam.Comment{}, err
	}
	teamID, err := db.ParseUUID(input.TeamID)
	if err != nil {
		return agentteam.Comment{}, err
	}
	metadata, err := agentteam.MergeCommentMetadata(input.Metadata, input.SourceSessionID)
	if err != nil {
		return agentteam.Comment{}, fmt.Errorf("merge comment metadata: %w", err)
	}
	row, err := s.queries.CreateTeamIssueComment(ctx, dbsqlc.CreateTeamIssueCommentParams{
		IssueID:         issueID,
		TeamID:          teamID,
		ParentCommentID: optionalUUID(input.ParentCommentID),
		AuthorType:      string(input.AuthorType),
		AuthorBotID:     optionalUUID(input.AuthorBotID),
		AuthorUserID:    optionalUUID(input.AuthorUserID),
		Content:         input.Content,
		Metadata:        jsonOrEmpty(metadata),
	})
	if err != nil {
		return agentteam.Comment{}, mapAgentTeamErr(err)
	}
	return commentRecord(row), nil
}

func (s *AgentTeamStore) GetComment(ctx context.Context, id string) (agentteam.Comment, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Comment{}, err
	}
	row, err := s.queries.GetTeamIssueComment(ctx, uid)
	if err != nil {
		return agentteam.Comment{}, mapAgentTeamErr(err)
	}
	return commentRecord(row), nil
}

func (s *AgentTeamStore) ListComments(ctx context.Context, issueID string) ([]agentteam.Comment, error) {
	iID, err := db.ParseUUID(issueID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTeamIssueComments(ctx, iID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, commentRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) TouchIssueAfterComment(ctx context.Context, issueID string) error {
	iID, err := db.ParseUUID(issueID)
	if err != nil {
		return err
	}
	return mapAgentTeamErr(s.queries.TouchTeamIssueAfterComment(ctx, iID))
}

func (s *AgentTeamStore) DeleteComment(ctx context.Context, id string) error {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return mapAgentTeamErr(s.queries.DeleteTeamIssueComment(ctx, uid))
}

// ── Handoffs ─────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateHandoff(ctx context.Context, input agentteam.CreateHandoffInput) (agentteam.Handoff, error) {
	teamID, err := db.ParseUUID(input.TeamID)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	toBotID, err := db.ParseUUID(input.ToBotID)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	status := input.Status
	if status == "" {
		status = agentteam.HandoffPending
	}
	row, err := s.queries.CreateAgentHandoff(ctx, dbsqlc.CreateAgentHandoffParams{
		TeamID:           teamID,
		IssueID:          optionalUUID(input.IssueID),
		FromActorType:    string(input.FromActorType),
		FromBotID:        optionalUUID(input.FromBotID),
		FromUserID:       optionalUUID(input.FromUserID),
		ToBotID:          toBotID,
		TriggerCommentID: optionalUUID(input.TriggerCommentID),
		SourceSessionID:  optionalUUID(input.SourceSessionID),
		TargetSessionID:  optionalUUID(input.TargetSessionID),
		Status:           string(status),
		Metadata:         jsonOrEmpty(input.Metadata),
	})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) GetHandoff(ctx context.Context, id string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.GetAgentHandoff(ctx, uid)
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) ListPendingHandoffsToBotForIssue(ctx context.Context, botID, issueID string) ([]agentteam.Handoff, error) {
	bID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	iID, err := db.ParseUUID(issueID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPendingHandoffsToBotForIssue(ctx, dbsqlc.ListPendingHandoffsToBotForIssueParams{ToBotID: bID, IssueID: iID})
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, row := range rows {
		out = append(out, handoffRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) ListPendingHandoffsToBot(ctx context.Context, botID string) ([]agentteam.Handoff, error) {
	bID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPendingHandoffsToBot(ctx, bID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, row := range rows {
		out = append(out, handoffRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) ListPendingReturnsForBotInIssue(ctx context.Context, fromBotID, issueID string) ([]agentteam.Handoff, error) {
	bID, err := db.ParseUUID(fromBotID)
	if err != nil {
		return nil, err
	}
	iID, err := db.ParseUUID(issueID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPendingReturnsForBotInIssue(ctx, dbsqlc.ListPendingReturnsForBotInIssueParams{FromBotID: bID, IssueID: iID})
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, row := range rows {
		out = append(out, handoffRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) ListHandoffsByIssue(ctx context.Context, issueID string) ([]agentteam.Handoff, error) {
	iID, err := db.ParseUUID(issueID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListHandoffsByIssue(ctx, iID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, row := range rows {
		out = append(out, handoffRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) MarkHandoffDispatched(ctx context.Context, id string, targetSessionID string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.MarkHandoffDispatched(ctx, dbsqlc.MarkHandoffDispatchedParams{
		TargetSessionID: optionalUUID(targetSessionID),
		ID:              uid,
	})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) MarkHandoffRunning(ctx context.Context, id string, targetSessionID string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.MarkHandoffRunning(ctx, dbsqlc.MarkHandoffRunningParams{
		TargetSessionID: optionalUUID(targetSessionID),
		ID:              uid,
	})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) CompleteHandoff(ctx context.Context, id string, resultCommentID string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.CompleteHandoff(ctx, dbsqlc.CompleteHandoffParams{
		ResultCommentID: optionalUUID(resultCommentID),
		ID:              uid,
	})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) FailHandoff(ctx context.Context, id string, failureReason string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.FailHandoff(ctx, dbsqlc.FailHandoffParams{FailureReason: failureReason, ID: uid})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) SetHandoffReturn(ctx context.Context, id string, returnHandoffID string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	ret, err := db.ParseUUID(returnHandoffID)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.SetHandoffReturn(ctx, dbsqlc.SetHandoffReturnParams{ReturnHandoffID: ret, ID: uid})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

func (s *AgentTeamStore) CancelHandoff(ctx context.Context, id string, failureReason string) (agentteam.Handoff, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.Handoff{}, err
	}
	row, err := s.queries.CancelHandoff(ctx, dbsqlc.CancelHandoffParams{FailureReason: failureReason, ID: uid})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffRecord(row), nil
}

// ── File Locks ───────────────────────────────────────────────────────────────

func (s *AgentTeamStore) AcquireFileLock(ctx context.Context, input agentteam.AcquireLockInput) (agentteam.FileLock, error) {
	teamID, err := db.ParseUUID(input.TeamID)
	if err != nil {
		return agentteam.FileLock{}, err
	}
	expires := time.Now().UTC().Add(input.TTL)
	row, err := s.queries.AcquireTeamFileLock(ctx, dbsqlc.AcquireTeamFileLockParams{
		TeamID:      teamID,
		Path:        input.Path,
		Scope:       string(input.Scope),
		OwnerType:   string(input.OwnerType),
		OwnerBotID:  optionalUUID(input.OwnerBotID),
		OwnerUserID: optionalUUID(input.OwnerUserID),
		IssueID:     optionalUUID(input.IssueID),
		SessionID:   optionalUUID(input.SessionID),
		HandoffID:   optionalUUID(input.HandoffID),
		ExpiresAt:   pgTimestamptz(expires, true),
		Metadata:    jsonOrEmpty(input.Metadata),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return agentteam.FileLock{}, agentteam.ErrLockHeld
		}
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockRecord(row), nil
}

func (s *AgentTeamStore) GetFileLockByID(ctx context.Context, id string) (agentteam.FileLock, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.FileLock{}, err
	}
	row, err := s.queries.GetTeamFileLockByID(ctx, uid)
	if err != nil {
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockRecord(row), nil
}

func (s *AgentTeamStore) GetFileLock(ctx context.Context, teamID, path string, scope agentteam.LockScope) (agentteam.FileLock, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return agentteam.FileLock{}, err
	}
	row, err := s.queries.GetTeamFileLock(ctx, dbsqlc.GetTeamFileLockParams{TeamID: tID, Path: path, Scope: string(scope)})
	if err != nil {
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockRecord(row), nil
}

func (s *AgentTeamStore) ListFileLocks(ctx context.Context, teamID string) ([]agentteam.FileLock, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTeamFileLocks(ctx, tID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.FileLock, 0, len(rows))
	for _, row := range rows {
		out = append(out, lockRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) ListActiveFileLocks(ctx context.Context, teamID string) ([]agentteam.FileLock, error) {
	tID, err := db.ParseUUID(teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListActiveTeamFileLocks(ctx, tID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.FileLock, 0, len(rows))
	for _, row := range rows {
		out = append(out, lockRecord(row))
	}
	return out, nil
}

func (s *AgentTeamStore) RefreshFileLock(ctx context.Context, id string, expiresAt time.Time) (agentteam.FileLock, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return agentteam.FileLock{}, err
	}
	row, err := s.queries.RefreshTeamFileLock(ctx, dbsqlc.RefreshTeamFileLockParams{
		ExpiresAt: pgTimestamptz(expiresAt, true),
		ID:        uid,
	})
	if err != nil {
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockRecord(row), nil
}

func (s *AgentTeamStore) ReleaseFileLock(ctx context.Context, id string) error {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return mapAgentTeamErr(s.queries.ReleaseTeamFileLock(ctx, uid))
}

func (s *AgentTeamStore) ReleaseExpiredFileLocks(ctx context.Context) error {
	return mapAgentTeamErr(s.queries.ReleaseExpiredTeamFileLocks(ctx))
}

func jsonOrEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
