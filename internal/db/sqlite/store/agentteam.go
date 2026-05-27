package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/agentteam"
	"github.com/memohai/memoh/internal/db"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

// AgentTeamStore implements agentteam.Store on top of the SQLite sqlc queries.
type AgentTeamStore struct {
	queries *sqlitesqlc.Queries
}

func NewAgentTeamStore(store *Store) *AgentTeamStore {
	if store == nil {
		return &AgentTeamStore{}
	}
	return &AgentTeamStore{queries: store.queries}
}

func mapAgentTeamErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Join(agentteam.ErrNotFound, db.ErrNotFound, pgx.ErrNoRows)
	}
	return err
}

func ptrNull(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

func sqlTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseLockTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func jsonOrEmptyString(b []byte) string {
	if len(b) == 0 {
		return "{}"
	}
	return string(b)
}

// ── Conversions ─────────────────────────────────────────────────────────────

func teamFromSQLite(row sqlitesqlc.AgentTeam) agentteam.Team {
	rec := agentteam.Team{
		ID:            row.ID,
		OwnerUserID:   row.OwnerUserID,
		Name:          row.Name,
		Description:   row.Description,
		AvatarURL:     row.AvatarUrl,
		SharedDirName: row.SharedDirName,
		Instructions:  row.Instructions,
		Metadata:      []byte(row.Metadata),
		CreatedAt:     parseLockTime(row.CreatedAt),
		UpdatedAt:     parseLockTime(row.UpdatedAt),
	}
	if row.ArchivedAt.Valid {
		rec.ArchivedAt = parseLockTime(row.ArchivedAt.String)
		rec.HasArchivedAt = true
	}
	return rec
}

func memberFromSQLite(row sqlitesqlc.AgentTeamMember) agentteam.Member {
	// Bare row (no JOIN); DisplayName left blank — callers should re-read
	// via the JOINing query for the canonical bot / user name.
	return agentteam.Member{
		ID:           row.ID,
		TeamID:       row.TeamID,
		MemberType:   agentteam.MemberType(row.MemberType),
		BotID:        row.BotID.String,
		UserID:       row.UserID.String,
		Role:         row.Role,
		Instructions: row.Instructions,
		Metadata:     []byte(row.Metadata),
		CreatedAt:    parseLockTime(row.CreatedAt),
		UpdatedAt:    parseLockTime(row.UpdatedAt),
	}
}

func memberFromSQLiteListRow(row sqlitesqlc.ListAgentTeamMembersRow) agentteam.Member {
	return agentteam.Member{
		ID:           row.ID,
		TeamID:       row.TeamID,
		MemberType:   agentteam.MemberType(row.MemberType),
		BotID:        row.BotID.String,
		UserID:       row.UserID.String,
		Role:         row.Role,
		Instructions: row.Instructions,
		Metadata:     []byte(row.Metadata),
		CreatedAt:    parseLockTime(row.CreatedAt),
		UpdatedAt:    parseLockTime(row.UpdatedAt),
		DisplayName:  row.ResolvedDisplayName,
	}
}

func memberFromSQLiteGetRow(row sqlitesqlc.GetAgentTeamMemberRow) agentteam.Member {
	return agentteam.Member{
		ID:           row.ID,
		TeamID:       row.TeamID,
		MemberType:   agentteam.MemberType(row.MemberType),
		BotID:        row.BotID.String,
		UserID:       row.UserID.String,
		Role:         row.Role,
		Instructions: row.Instructions,
		Metadata:     []byte(row.Metadata),
		CreatedAt:    parseLockTime(row.CreatedAt),
		UpdatedAt:    parseLockTime(row.UpdatedAt),
		DisplayName:  row.ResolvedDisplayName,
	}
}

func memberFromSQLiteGetByBotRow(row sqlitesqlc.GetAgentTeamMemberByBotRow) agentteam.Member {
	return agentteam.Member{
		ID:           row.ID,
		TeamID:       row.TeamID,
		MemberType:   agentteam.MemberType(row.MemberType),
		BotID:        row.BotID.String,
		UserID:       row.UserID.String,
		Role:         row.Role,
		Instructions: row.Instructions,
		Metadata:     []byte(row.Metadata),
		CreatedAt:    parseLockTime(row.CreatedAt),
		UpdatedAt:    parseLockTime(row.UpdatedAt),
		DisplayName:  row.ResolvedDisplayName,
	}
}

func memberFromSQLiteGetByUserRow(row sqlitesqlc.GetAgentTeamMemberByUserRow) agentteam.Member {
	return agentteam.Member{
		ID:           row.ID,
		TeamID:       row.TeamID,
		MemberType:   agentteam.MemberType(row.MemberType),
		BotID:        row.BotID.String,
		UserID:       row.UserID.String,
		Role:         row.Role,
		Instructions: row.Instructions,
		Metadata:     []byte(row.Metadata),
		CreatedAt:    parseLockTime(row.CreatedAt),
		UpdatedAt:    parseLockTime(row.UpdatedAt),
		DisplayName:  row.ResolvedDisplayName,
	}
}

func issueFromSQLite(row sqlitesqlc.TeamIssue) agentteam.Issue {
	rec := agentteam.Issue{
		ID:              row.ID,
		TeamID:          row.TeamID,
		Number:          safeInt32(row.Number),
		Title:           row.Title,
		Description:     row.Description,
		Status:          agentteam.IssueStatus(row.Status),
		AssigneeType:    row.AssigneeType.String,
		AssigneeBotID:   row.AssigneeBotID.String,
		AssigneeUserID:  row.AssigneeUserID.String,
		CreatedByType:   agentteam.ActorType(row.CreatedByType),
		CreatedByBotID:  row.CreatedByBotID.String,
		CreatedByUserID: row.CreatedByUserID.String,
		ParentIssueID:   row.ParentIssueID.String,
		Metadata:        []byte(row.Metadata),
		CreatedAt:       parseLockTime(row.CreatedAt),
		UpdatedAt:       parseLockTime(row.UpdatedAt),
	}
	if row.ClosedAt.Valid {
		rec.ClosedAt = parseLockTime(row.ClosedAt.String)
		rec.HasClosedAt = true
	}
	return rec
}

func commentFromSQLite(row sqlitesqlc.TeamIssueComment) agentteam.Comment {
	meta := []byte(row.Metadata)
	return agentteam.Comment{
		ID:              row.ID,
		IssueID:         row.IssueID,
		TeamID:          row.TeamID,
		ParentCommentID: row.ParentCommentID.String,
		AuthorType:      agentteam.ActorType(row.AuthorType),
		AuthorBotID:     row.AuthorBotID.String,
		AuthorUserID:    row.AuthorUserID.String,
		Content:         row.Content,
		Metadata:        meta,
		SourceSessionID: agentteam.ReadSourceSessionFromMetadata(meta),
		CreatedAt:       parseLockTime(row.CreatedAt),
		UpdatedAt:       parseLockTime(row.UpdatedAt),
	}
}

func handoffFromSQLite(row sqlitesqlc.AgentHandoff) agentteam.Handoff {
	rec := agentteam.Handoff{
		ID:               row.ID,
		TeamID:           row.TeamID,
		IssueID:          row.IssueID.String,
		FromActorType:    agentteam.ActorType(row.FromActorType),
		FromBotID:        row.FromBotID.String,
		FromUserID:       row.FromUserID.String,
		ToBotID:          row.ToBotID,
		TriggerCommentID: row.TriggerCommentID.String,
		SourceSessionID:  row.SourceSessionID.String,
		TargetSessionID:  row.TargetSessionID.String,
		ResultCommentID:  row.ResultCommentID.String,
		ReturnHandoffID:  row.ReturnHandoffID.String,
		Status:           agentteam.HandoffStatus(row.Status),
		FailureReason:    row.FailureReason,
		Metadata:         []byte(row.Metadata),
		CreatedAt:        parseLockTime(row.CreatedAt),
		UpdatedAt:        parseLockTime(row.UpdatedAt),
	}
	if row.CompletedAt.Valid {
		rec.CompletedAt = parseLockTime(row.CompletedAt.String)
		rec.HasCompletedAt = true
	}
	return rec
}

func lockFromSQLite(row sqlitesqlc.TeamFileLock) agentteam.FileLock {
	return agentteam.FileLock{
		ID:          row.ID,
		TeamID:      row.TeamID,
		Path:        row.Path,
		Scope:       agentteam.LockScope(row.Scope),
		OwnerType:   agentteam.ActorType(row.OwnerType),
		OwnerBotID:  row.OwnerBotID.String,
		OwnerUserID: row.OwnerUserID.String,
		IssueID:     row.IssueID.String,
		SessionID:   row.SessionID.String,
		HandoffID:   row.HandoffID.String,
		AcquiredAt:  parseLockTime(row.AcquiredAt),
		RefreshedAt: parseLockTime(row.RefreshedAt),
		ExpiresAt:   parseLockTime(row.ExpiresAt),
		Metadata:    []byte(row.Metadata),
	}
}

// ── Teams ───────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateTeam(ctx context.Context, input agentteam.CreateTeamInput) (agentteam.Team, error) {
	row, err := s.queries.CreateAgentTeam(ctx, sqlitesqlc.CreateAgentTeamParams{
		OwnerUserID:   input.OwnerUserID,
		Name:          input.Name,
		Description:   input.Description,
		AvatarUrl:     input.AvatarURL,
		SharedDirName: input.SharedDirName,
		Instructions:  input.Instructions,
		Metadata:      jsonOrEmptyString(input.Metadata),
	})
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamFromSQLite(row), nil
}

func (s *AgentTeamStore) GetTeam(ctx context.Context, id string) (agentteam.Team, error) {
	row, err := s.queries.GetAgentTeam(ctx, id)
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamFromSQLite(row), nil
}

func (s *AgentTeamStore) GetTeamForOwner(ctx context.Context, id, ownerUserID string) (agentteam.Team, error) {
	row, err := s.queries.GetAgentTeamForOwner(ctx, sqlitesqlc.GetAgentTeamForOwnerParams{ID: id, OwnerUserID: ownerUserID})
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamFromSQLite(row), nil
}

func (s *AgentTeamStore) ListTeamsByOwner(ctx context.Context, ownerUserID string) ([]agentteam.Team, error) {
	rows, err := s.queries.ListAgentTeamsByOwner(ctx, ownerUserID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Team, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListAllTeamsByOwner(ctx context.Context, ownerUserID string) ([]agentteam.Team, error) {
	rows, err := s.queries.ListAllAgentTeamsByOwner(ctx, ownerUserID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Team, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListAllTeams(ctx context.Context) ([]agentteam.Team, error) {
	rows, err := s.queries.ListAllAgentTeams(ctx)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Team, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListTeamsForBot(ctx context.Context, botID string) ([]agentteam.Team, error) {
	rows, err := s.queries.ListAgentTeamsForBot(ctx, sql.NullString{String: botID, Valid: botID != ""})
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Team, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) UpdateTeam(ctx context.Context, id string, input agentteam.UpdateTeamInput) (agentteam.Team, error) {
	var metaParam sql.NullString
	if input.Metadata != nil {
		metaParam = sql.NullString{String: string(input.Metadata), Valid: true}
	}
	row, err := s.queries.UpdateAgentTeam(ctx, sqlitesqlc.UpdateAgentTeamParams{
		Name:          ptrNull(input.Name),
		Description:   ptrNull(input.Description),
		AvatarUrl:     ptrNull(input.AvatarURL),
		SharedDirName: ptrNull(input.SharedDirName),
		Instructions:  ptrNull(input.Instructions),
		Metadata:      metaParam,
		ID:            id,
	})
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamFromSQLite(row), nil
}

func (s *AgentTeamStore) ArchiveTeam(ctx context.Context, id string) (agentteam.Team, error) {
	row, err := s.queries.ArchiveAgentTeam(ctx, id)
	if err != nil {
		return agentteam.Team{}, mapAgentTeamErr(err)
	}
	return teamFromSQLite(row), nil
}

func (s *AgentTeamStore) DeleteTeam(ctx context.Context, id string) error {
	return mapAgentTeamErr(s.queries.DeleteAgentTeam(ctx, id))
}

// ── Members ─────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) AddMember(ctx context.Context, input agentteam.CreateMemberInput) (agentteam.Member, error) {
	row, err := s.queries.AddAgentTeamMember(ctx, sqlitesqlc.AddAgentTeamMemberParams{
		TeamID:       input.TeamID,
		MemberType:   string(input.MemberType),
		BotID:        nullableID(input.BotID),
		UserID:       nullableID(input.UserID),
		Role:         input.Role,
		Instructions: input.Instructions,
		Metadata:     jsonOrEmptyString(input.Metadata),
	})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	// Re-read via the JOINing query to populate DisplayName from
	// bots.display_name / users.display_name.
	if full, gerr := s.queries.GetAgentTeamMember(ctx, row.ID); gerr == nil {
		return memberFromSQLiteGetRow(full), nil
	}
	return memberFromSQLite(row), nil
}

func (s *AgentTeamStore) GetMember(ctx context.Context, id string) (agentteam.Member, error) {
	row, err := s.queries.GetAgentTeamMember(ctx, id)
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	return memberFromSQLiteGetRow(row), nil
}

func (s *AgentTeamStore) GetMemberByBot(ctx context.Context, teamID, botID string) (agentteam.Member, error) {
	row, err := s.queries.GetAgentTeamMemberByBot(ctx, sqlitesqlc.GetAgentTeamMemberByBotParams{TeamID: teamID, BotID: nullableID(botID)})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	return memberFromSQLiteGetByBotRow(row), nil
}

func (s *AgentTeamStore) GetMemberByUser(ctx context.Context, teamID, userID string) (agentteam.Member, error) {
	row, err := s.queries.GetAgentTeamMemberByUser(ctx, sqlitesqlc.GetAgentTeamMemberByUserParams{TeamID: teamID, UserID: nullableID(userID)})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	return memberFromSQLiteGetByUserRow(row), nil
}

func (s *AgentTeamStore) ListMembers(ctx context.Context, teamID string) ([]agentteam.Member, error) {
	rows, err := s.queries.ListAgentTeamMembers(ctx, teamID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Member, 0, len(rows))
	for _, r := range rows {
		out = append(out, memberFromSQLiteListRow(r))
	}
	return out, nil
}

func (s *AgentTeamStore) UpdateMember(ctx context.Context, id string, input agentteam.UpdateMemberInput) (agentteam.Member, error) {
	var metaParam sql.NullString
	if input.Metadata != nil {
		metaParam = sql.NullString{String: string(input.Metadata), Valid: true}
	}
	row, err := s.queries.UpdateAgentTeamMember(ctx, sqlitesqlc.UpdateAgentTeamMemberParams{
		Role:         ptrNull(input.Role),
		Instructions: ptrNull(input.Instructions),
		Metadata:     metaParam,
		ID:           id,
	})
	if err != nil {
		return agentteam.Member{}, mapAgentTeamErr(err)
	}
	if full, gerr := s.queries.GetAgentTeamMember(ctx, row.ID); gerr == nil {
		return memberFromSQLiteGetRow(full), nil
	}
	return memberFromSQLite(row), nil
}

func (s *AgentTeamStore) DeleteMember(ctx context.Context, id string) error {
	return mapAgentTeamErr(s.queries.DeleteAgentTeamMember(ctx, id))
}

// ── Issues ──────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateIssue(ctx context.Context, input agentteam.CreateIssueInput) (agentteam.Issue, error) {
	status := input.Status
	if status == "" {
		status = agentteam.StatusTodo
	}
	row, err := s.queries.CreateTeamIssue(ctx, sqlitesqlc.CreateTeamIssueParams{
		TeamID:          input.TeamID,
		Title:           input.Title,
		Description:     input.Description,
		Status:          string(status),
		AssigneeType:    nullableID(input.AssigneeType),
		AssigneeBotID:   nullableID(input.AssigneeBotID),
		AssigneeUserID:  nullableID(input.AssigneeUserID),
		CreatedByType:   string(input.CreatedByType),
		CreatedByBotID:  nullableID(input.CreatedByBotID),
		CreatedByUserID: nullableID(input.CreatedByUserID),
		ParentIssueID:   nullableID(input.ParentIssueID),
		Metadata:        jsonOrEmptyString(input.Metadata),
	})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueFromSQLite(row), nil
}

func (s *AgentTeamStore) GetIssue(ctx context.Context, id string) (agentteam.Issue, error) {
	row, err := s.queries.GetTeamIssue(ctx, id)
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueFromSQLite(row), nil
}

func (s *AgentTeamStore) GetIssueInTeam(ctx context.Context, id, teamID string) (agentteam.Issue, error) {
	row, err := s.queries.GetTeamIssueInTeam(ctx, sqlitesqlc.GetTeamIssueInTeamParams{ID: id, TeamID: teamID})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueFromSQLite(row), nil
}

func (s *AgentTeamStore) GetIssueByNumber(ctx context.Context, teamID string, number int32) (agentteam.Issue, error) {
	row, err := s.queries.GetTeamIssueByNumber(ctx, sqlitesqlc.GetTeamIssueByNumberParams{TeamID: teamID, Number: int64(number)})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueFromSQLite(row), nil
}

func (s *AgentTeamStore) ListIssuesByTeam(ctx context.Context, teamID string) ([]agentteam.Issue, error) {
	rows, err := s.queries.ListTeamIssuesByTeam(ctx, teamID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Issue, 0, len(rows))
	for _, r := range rows {
		out = append(out, issueFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListOpenIssuesByTeam(ctx context.Context, teamID string) ([]agentteam.Issue, error) {
	rows, err := s.queries.ListOpenTeamIssuesByTeam(ctx, teamID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Issue, 0, len(rows))
	for _, r := range rows {
		out = append(out, issueFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListIssuesForOwner(ctx context.Context, ownerUserID string) ([]agentteam.Issue, error) {
	rows, err := s.queries.ListTeamIssuesForOwner(ctx, ownerUserID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Issue, 0, len(rows))
	for _, r := range rows {
		out = append(out, issueFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) UpdateIssue(ctx context.Context, id string, input agentteam.UpdateIssueInput) (agentteam.Issue, error) {
	var statusNull sql.NullString
	if input.Status != nil {
		statusNull = sql.NullString{String: string(*input.Status), Valid: true}
	}
	var metaParam sql.NullString
	if input.Metadata != nil {
		metaParam = sql.NullString{String: string(input.Metadata), Valid: true}
	}
	var aBot, aUser sql.NullString
	if input.AssigneeBotID != nil {
		aBot = nullableID(*input.AssigneeBotID)
	}
	if input.AssigneeUserID != nil {
		aUser = nullableID(*input.AssigneeUserID)
	}
	row, err := s.queries.UpdateTeamIssue(ctx, sqlitesqlc.UpdateTeamIssueParams{
		Title:          ptrNull(input.Title),
		Description:    ptrNull(input.Description),
		Status:         statusNull,
		AssigneeType:   ptrNull(input.AssigneeType),
		AssigneeBotID:  aBot,
		AssigneeUserID: aUser,
		Metadata:       metaParam,
		ID:             id,
	})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueFromSQLite(row), nil
}

func (s *AgentTeamStore) SetIssueAssignee(ctx context.Context, id string, input agentteam.AssignIssueInput) (agentteam.Issue, error) {
	row, err := s.queries.SetTeamIssueAssignee(ctx, sqlitesqlc.SetTeamIssueAssigneeParams{
		AssigneeType:   nullableID(input.AssigneeType),
		AssigneeBotID:  nullableID(input.AssigneeBotID),
		AssigneeUserID: nullableID(input.AssigneeUserID),
		ID:             id,
	})
	if err != nil {
		return agentteam.Issue{}, mapAgentTeamErr(err)
	}
	return issueFromSQLite(row), nil
}

func (s *AgentTeamStore) DeleteIssue(ctx context.Context, id string) error {
	return mapAgentTeamErr(s.queries.DeleteTeamIssue(ctx, id))
}

// ── Comments ────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateComment(ctx context.Context, input agentteam.CreateCommentInput) (agentteam.Comment, error) {
	metadata, err := agentteam.MergeCommentMetadata(input.Metadata, input.SourceSessionID)
	if err != nil {
		return agentteam.Comment{}, fmt.Errorf("merge comment metadata: %w", err)
	}
	row, err := s.queries.CreateTeamIssueComment(ctx, sqlitesqlc.CreateTeamIssueCommentParams{
		IssueID:         input.IssueID,
		TeamID:          input.TeamID,
		ParentCommentID: nullableID(input.ParentCommentID),
		AuthorType:      string(input.AuthorType),
		AuthorBotID:     nullableID(input.AuthorBotID),
		AuthorUserID:    nullableID(input.AuthorUserID),
		Content:         input.Content,
		Metadata:        jsonOrEmptyString(metadata),
	})
	if err != nil {
		return agentteam.Comment{}, mapAgentTeamErr(err)
	}
	return commentFromSQLite(row), nil
}

func (s *AgentTeamStore) GetComment(ctx context.Context, id string) (agentteam.Comment, error) {
	row, err := s.queries.GetTeamIssueComment(ctx, id)
	if err != nil {
		return agentteam.Comment{}, mapAgentTeamErr(err)
	}
	return commentFromSQLite(row), nil
}

func (s *AgentTeamStore) ListComments(ctx context.Context, issueID string) ([]agentteam.Comment, error) {
	rows, err := s.queries.ListTeamIssueComments(ctx, issueID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, commentFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) TouchIssueAfterComment(ctx context.Context, issueID string) error {
	return mapAgentTeamErr(s.queries.TouchTeamIssueAfterComment(ctx, issueID))
}

func (s *AgentTeamStore) DeleteComment(ctx context.Context, id string) error {
	return mapAgentTeamErr(s.queries.DeleteTeamIssueComment(ctx, id))
}

// ── Handoffs ────────────────────────────────────────────────────────────────

func (s *AgentTeamStore) CreateHandoff(ctx context.Context, input agentteam.CreateHandoffInput) (agentteam.Handoff, error) {
	status := input.Status
	if status == "" {
		status = agentteam.HandoffPending
	}
	row, err := s.queries.CreateAgentHandoff(ctx, sqlitesqlc.CreateAgentHandoffParams{
		TeamID:           input.TeamID,
		IssueID:          nullableID(input.IssueID),
		FromActorType:    string(input.FromActorType),
		FromBotID:        nullableID(input.FromBotID),
		FromUserID:       nullableID(input.FromUserID),
		ToBotID:          input.ToBotID,
		TriggerCommentID: nullableID(input.TriggerCommentID),
		SourceSessionID:  nullableID(input.SourceSessionID),
		TargetSessionID:  nullableID(input.TargetSessionID),
		Status:           string(status),
		Metadata:         jsonOrEmptyString(input.Metadata),
	})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) GetHandoff(ctx context.Context, id string) (agentteam.Handoff, error) {
	row, err := s.queries.GetAgentHandoff(ctx, id)
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) ListPendingHandoffsToBotForIssue(ctx context.Context, botID, issueID string) ([]agentteam.Handoff, error) {
	rows, err := s.queries.ListPendingHandoffsToBotForIssue(ctx, sqlitesqlc.ListPendingHandoffsToBotForIssueParams{ToBotID: botID, IssueID: nullableID(issueID)})
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, r := range rows {
		out = append(out, handoffFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListPendingHandoffsToBot(ctx context.Context, botID string) ([]agentteam.Handoff, error) {
	rows, err := s.queries.ListPendingHandoffsToBot(ctx, botID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, r := range rows {
		out = append(out, handoffFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListPendingReturnsForBotInIssue(ctx context.Context, fromBotID, issueID string) ([]agentteam.Handoff, error) {
	rows, err := s.queries.ListPendingReturnsForBotInIssue(ctx, sqlitesqlc.ListPendingReturnsForBotInIssueParams{FromBotID: nullableID(fromBotID), IssueID: nullableID(issueID)})
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, r := range rows {
		out = append(out, handoffFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListHandoffsByIssue(ctx context.Context, issueID string) ([]agentteam.Handoff, error) {
	rows, err := s.queries.ListHandoffsByIssue(ctx, nullableID(issueID))
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.Handoff, 0, len(rows))
	for _, r := range rows {
		out = append(out, handoffFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) MarkHandoffDispatched(ctx context.Context, id, targetSessionID string) (agentteam.Handoff, error) {
	row, err := s.queries.MarkHandoffDispatched(ctx, sqlitesqlc.MarkHandoffDispatchedParams{TargetSessionID: nullableID(targetSessionID), ID: id})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) MarkHandoffRunning(ctx context.Context, id, targetSessionID string) (agentteam.Handoff, error) {
	row, err := s.queries.MarkHandoffRunning(ctx, sqlitesqlc.MarkHandoffRunningParams{TargetSessionID: nullableID(targetSessionID), ID: id})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) CompleteHandoff(ctx context.Context, id, resultCommentID string) (agentteam.Handoff, error) {
	row, err := s.queries.CompleteHandoff(ctx, sqlitesqlc.CompleteHandoffParams{ResultCommentID: nullableID(resultCommentID), ID: id})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) FailHandoff(ctx context.Context, id, failureReason string) (agentteam.Handoff, error) {
	row, err := s.queries.FailHandoff(ctx, sqlitesqlc.FailHandoffParams{FailureReason: failureReason, ID: id})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) SetHandoffReturn(ctx context.Context, id, returnHandoffID string) (agentteam.Handoff, error) {
	row, err := s.queries.SetHandoffReturn(ctx, sqlitesqlc.SetHandoffReturnParams{ReturnHandoffID: nullableID(returnHandoffID), ID: id})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

func (s *AgentTeamStore) CancelHandoff(ctx context.Context, id, failureReason string) (agentteam.Handoff, error) {
	row, err := s.queries.CancelHandoff(ctx, sqlitesqlc.CancelHandoffParams{FailureReason: failureReason, ID: id})
	if err != nil {
		return agentteam.Handoff{}, mapAgentTeamErr(err)
	}
	return handoffFromSQLite(row), nil
}

// ── File Locks ──────────────────────────────────────────────────────────────

func (s *AgentTeamStore) AcquireFileLock(ctx context.Context, input agentteam.AcquireLockInput) (agentteam.FileLock, error) {
	expires := time.Now().UTC().Add(input.TTL)
	row, err := s.queries.AcquireTeamFileLock(ctx, sqlitesqlc.AcquireTeamFileLockParams{
		TeamID:      input.TeamID,
		Path:        input.Path,
		Scope:       string(input.Scope),
		OwnerType:   string(input.OwnerType),
		OwnerBotID:  nullableID(input.OwnerBotID),
		OwnerUserID: nullableID(input.OwnerUserID),
		IssueID:     nullableID(input.IssueID),
		SessionID:   nullableID(input.SessionID),
		HandoffID:   nullableID(input.HandoffID),
		ExpiresAt:   sqlTime(expires),
		Metadata:    jsonOrEmptyString(input.Metadata),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentteam.FileLock{}, agentteam.ErrLockHeld
		}
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockFromSQLite(row), nil
}

func (s *AgentTeamStore) GetFileLockByID(ctx context.Context, id string) (agentteam.FileLock, error) {
	row, err := s.queries.GetTeamFileLockByID(ctx, id)
	if err != nil {
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockFromSQLite(row), nil
}

func (s *AgentTeamStore) GetFileLock(ctx context.Context, teamID, path string, scope agentteam.LockScope) (agentteam.FileLock, error) {
	row, err := s.queries.GetTeamFileLock(ctx, sqlitesqlc.GetTeamFileLockParams{TeamID: teamID, Path: path, Scope: string(scope)})
	if err != nil {
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockFromSQLite(row), nil
}

func (s *AgentTeamStore) ListFileLocks(ctx context.Context, teamID string) ([]agentteam.FileLock, error) {
	rows, err := s.queries.ListTeamFileLocks(ctx, teamID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.FileLock, 0, len(rows))
	for _, r := range rows {
		out = append(out, lockFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) ListActiveFileLocks(ctx context.Context, teamID string) ([]agentteam.FileLock, error) {
	rows, err := s.queries.ListActiveTeamFileLocks(ctx, teamID)
	if err != nil {
		return nil, mapAgentTeamErr(err)
	}
	out := make([]agentteam.FileLock, 0, len(rows))
	for _, r := range rows {
		out = append(out, lockFromSQLite(r))
	}
	return out, nil
}

func (s *AgentTeamStore) RefreshFileLock(ctx context.Context, id string, expiresAt time.Time) (agentteam.FileLock, error) {
	row, err := s.queries.RefreshTeamFileLock(ctx, sqlitesqlc.RefreshTeamFileLockParams{ExpiresAt: sqlTime(expiresAt), ID: id})
	if err != nil {
		return agentteam.FileLock{}, mapAgentTeamErr(err)
	}
	return lockFromSQLite(row), nil
}

func (s *AgentTeamStore) ReleaseFileLock(ctx context.Context, id string) error {
	return mapAgentTeamErr(s.queries.ReleaseTeamFileLock(ctx, id))
}

func (s *AgentTeamStore) ReleaseExpiredFileLocks(ctx context.Context) error {
	return mapAgentTeamErr(s.queries.ReleaseExpiredTeamFileLocks(ctx))
}

func nullableID(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func safeInt32(v int64) int32 {
	switch {
	case v > int64(^uint32(0)>>1):
		return int32(^uint32(0) >> 1)
	case v < -int64(^uint32(0)>>1)-1:
		return -int32(^uint32(0)>>1) - 1
	default:
		return int32(v) // #nosec G115 -- guarded above.
	}
}
