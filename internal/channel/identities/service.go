package identities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/teams"
)

// Service provides channel identity lifecycle operations.
type Service struct {
	queries dbstore.Queries
	logger  *slog.Logger
}

var ErrChannelIdentityNotFound = errors.New("channel identity not found")

// NewService creates a new channel identity service.
func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "channel/identities")),
	}
}

// Create creates a new channel identity for the given channel subject.
func (s *Service) Create(ctx context.Context, channel, channelSubjectID, displayName string) (ChannelIdentity, error) {
	if s.queries == nil {
		return ChannelIdentity{}, errors.New("channel identity queries not configured")
	}
	channel = normalizeChannel(channel)
	channelSubjectID = strings.TrimSpace(channelSubjectID)
	if channel == "" || channelSubjectID == "" {
		return ChannelIdentity{}, errors.New("channel and channel_subject_id are required")
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return ChannelIdentity{}, err
	}
	params := sqlc.CreateChannelIdentityParams{
		ChannelType:      channel,
		ChannelSubjectID: channelSubjectID,
		DisplayName:      toPgText(displayName),
		AvatarUrl:        pgtype.Text{},
		Metadata:         emptyMetadataBytes(),
	}
	applyTeamID(&params, teamID)
	row, err := s.queries.CreateChannelIdentity(ctx, params)
	if err != nil {
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// GetByID returns a channel identity by its ID.
func (s *Service) GetByID(ctx context.Context, channelIdentityID string) (ChannelIdentity, error) {
	if s.queries == nil {
		return ChannelIdentity{}, errors.New("channel identity queries not configured")
	}
	pgID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return ChannelIdentity{}, err
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return ChannelIdentity{}, err
	}
	row, err := getChannelIdentityByID(ctx, s.queries, teamID, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChannelIdentity{}, ErrChannelIdentityNotFound
		}
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// Canonicalize validates and returns the same channel identity ID.
func (s *Service) Canonicalize(ctx context.Context, channelIdentityID string) (string, error) {
	if s.queries == nil {
		return "", errors.New("channel identity queries not configured")
	}
	pgID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return "", err
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	_, err = getChannelIdentityByID(ctx, s.queries, teamID, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrChannelIdentityNotFound
		}
		return "", err
	}
	return channelIdentityID, nil
}

// ResolveByChannelIdentity looks up or creates a channel identity for (channel, channel_subject_id).
// Optional meta may contain avatar_url which is stored as a dedicated column.
func (s *Service) ResolveByChannelIdentity(ctx context.Context, channel, channelSubjectID, displayName string, meta map[string]any) (ChannelIdentity, error) {
	if s.queries == nil {
		return ChannelIdentity{}, errors.New("channel identity queries not configured")
	}
	channel = normalizeChannel(channel)
	channelSubjectID = strings.TrimSpace(channelSubjectID)
	if channel == "" || channelSubjectID == "" {
		return ChannelIdentity{}, errors.New("channel and channel_subject_id are required")
	}

	avatarURL := ""
	if meta != nil {
		if raw, ok := meta["avatar_url"]; ok {
			avatarURL = strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return ChannelIdentity{}, err
	}

	params := sqlc.UpsertChannelIdentityByChannelSubjectParams{
		ChannelType:      channel,
		ChannelSubjectID: channelSubjectID,
		DisplayName:      toPgText(displayName),
		AvatarUrl:        toPgText(avatarURL),
		Metadata:         emptyMetadataBytes(),
	}
	applyTeamID(&params, teamID)
	row, err := s.queries.UpsertChannelIdentityByChannelSubject(ctx, params)
	if err != nil {
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// UpsertChannelIdentity creates or updates a channel identity mapping.
func (s *Service) UpsertChannelIdentity(ctx context.Context, channel, channelSubjectID, displayName string, metadata map[string]any) (ChannelIdentity, error) {
	if s.queries == nil {
		return ChannelIdentity{}, errors.New("channel identity queries not configured")
	}
	channel = normalizeChannel(channel)
	channelSubjectID = strings.TrimSpace(channelSubjectID)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return ChannelIdentity{}, err
	}
	avatarURL := ""
	if raw, ok := metadata["avatar_url"]; ok {
		avatarURL = strings.TrimSpace(fmt.Sprint(raw))
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return ChannelIdentity{}, err
	}
	params := sqlc.UpsertChannelIdentityByChannelSubjectParams{
		ChannelType:      channel,
		ChannelSubjectID: channelSubjectID,
		DisplayName:      toPgText(displayName),
		AvatarUrl:        toPgText(avatarURL),
		Metadata:         metaBytes,
	}
	applyTeamID(&params, teamID)
	row, err := s.queries.UpsertChannelIdentityByChannelSubject(ctx, params)
	if err != nil {
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// ListCanonicalChannelIdentities returns the requested channel identity.
func (s *Service) ListCanonicalChannelIdentities(ctx context.Context, channelIdentityID string) ([]ChannelIdentity, error) {
	if s.queries == nil {
		return nil, errors.New("channel identity queries not configured")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return nil, err
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	row, err := getChannelIdentityByID(ctx, s.queries, teamID, pgChannelIdentityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrChannelIdentityNotFound
		}
		return nil, err
	}
	return []ChannelIdentity{toChannelIdentity(row)}, nil
}

// ListUserIDsByChannelIdentity returns account user IDs currently linked to a
// channel identity. Callers that need a runtime owner should require exactly one
// result; an unbound or ambiguously bound channel identity cannot safely own a
// workspace runtime.
func (s *Service) ListUserIDsByChannelIdentity(ctx context.Context, channelIdentityID string) ([]string, error) {
	if s.queries == nil {
		return nil, errors.New("channel identity queries not configured")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return nil, err
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := listUserIDsByChannelIdentity(ctx, s.queries, teamID, pgChannelIdentityID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, id := range rows {
		userID := strings.TrimSpace(id.String())
		if userID != "" {
			out = append(out, userID)
		}
	}
	return out, nil
}

// Search returns locally observed channel identities for UI search.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if s.queries == nil {
		return nil, errors.New("channel identity queries not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	teamID, err := teamUUIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	params := sqlc.SearchChannelIdentitiesParams{
		Query:      strings.TrimSpace(query),
		LimitCount: int32(limit), //nolint:gosec // limit is capped above
	}
	applyTeamID(&params, teamID)
	rows, err := s.queries.SearchChannelIdentities(ctx, params)
	if err != nil {
		return nil, err
	}
	items := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		item := SearchResult{
			ChannelIdentity: toChannelIdentity(sqlc.ChannelIdentity{
				ID:               row.ID,
				ChannelType:      row.ChannelType,
				ChannelSubjectID: row.ChannelSubjectID,
				DisplayName:      row.DisplayName,
				AvatarUrl:        row.AvatarUrl,
				Metadata:         row.Metadata,
				CreatedAt:        row.CreatedAt,
				UpdatedAt:        row.UpdatedAt,
			}),
		}
		items = append(items, item)
	}
	return items, nil
}

func teamUUIDFromContext(ctx context.Context) (pgtype.UUID, error) {
	scope := teams.ScopeOrDefault(ctx)
	return db.ParseUUID(strings.TrimSpace(scope.TeamID))
}

func applyTeamID(params any, teamID pgtype.UUID) {
	value := reflect.ValueOf(params)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	field := elem.FieldByName("TeamID")
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeOf(pgtype.UUID{}) {
		return
	}
	field.Set(reflect.ValueOf(teamID))
}

func getChannelIdentityByID(ctx context.Context, queries dbstore.Queries, teamID, id pgtype.UUID) (sqlc.ChannelIdentity, error) {
	values, err := callTeamScopedQuery(ctx, queries, "GetChannelIdentityByID", teamID, map[string]reflect.Value{
		"ID": reflect.ValueOf(id),
	}, reflect.ValueOf(id))
	if err != nil {
		return sqlc.ChannelIdentity{}, err
	}
	row, _ := values[0].Interface().(sqlc.ChannelIdentity)
	return row, errorFromValue(values[1])
}

func listUserIDsByChannelIdentity(ctx context.Context, queries dbstore.Queries, teamID, channelIdentityID pgtype.UUID) ([]pgtype.UUID, error) {
	values, err := callTeamScopedQuery(ctx, queries, "ListUserIDsByChannelIdentity", teamID, map[string]reflect.Value{
		"ChannelIdentityID": reflect.ValueOf(channelIdentityID),
	}, reflect.ValueOf(channelIdentityID))
	if err != nil {
		return nil, err
	}
	rows, _ := values[0].Interface().([]pgtype.UUID)
	return rows, errorFromValue(values[1])
}

func callTeamScopedQuery(ctx context.Context, queries dbstore.Queries, methodName string, teamID pgtype.UUID, fields map[string]reflect.Value, legacyArg reflect.Value) ([]reflect.Value, error) {
	method := reflect.ValueOf(queries).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("query method %s not configured", methodName)
	}
	if method.Type().NumIn() != 2 {
		return nil, fmt.Errorf("query method %s has unexpected arity", methodName)
	}
	argType := method.Type().In(1)
	var arg reflect.Value
	if legacyArg.IsValid() && legacyArg.Type().AssignableTo(argType) {
		arg = legacyArg
	} else {
		arg = reflect.New(argType).Elem()
		if arg.Kind() != reflect.Struct {
			return nil, fmt.Errorf("query method %s has unsupported arg type %s", methodName, argType)
		}
		if field := arg.FieldByName("TeamID"); field.IsValid() && field.CanSet() && reflect.TypeOf(teamID).AssignableTo(field.Type()) {
			field.Set(reflect.ValueOf(teamID))
		}
		for name, value := range fields {
			field := arg.FieldByName(name)
			if field.IsValid() && field.CanSet() && value.Type().AssignableTo(field.Type()) {
				field.Set(value)
			}
		}
	}
	return method.Call([]reflect.Value{reflect.ValueOf(ctx), arg}), nil
}

func errorFromValue(value reflect.Value) error {
	if !value.IsValid() || value.IsNil() {
		return nil
	}
	err, _ := value.Interface().(error)
	return err
}

func toChannelIdentity(row sqlc.ChannelIdentity) ChannelIdentity {
	var metadata map[string]any
	if len(row.Metadata) > 0 {
		_ = json.Unmarshal(row.Metadata, &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	displayName := ""
	if row.DisplayName.Valid {
		displayName = strings.TrimSpace(row.DisplayName.String)
	}
	avatarURL := ""
	if row.AvatarUrl.Valid {
		avatarURL = strings.TrimSpace(row.AvatarUrl.String)
	}
	return ChannelIdentity{
		ID:               row.ID.String(),
		Channel:          row.ChannelType,
		ChannelSubjectID: row.ChannelSubjectID,
		DisplayName:      displayName,
		AvatarURL:        avatarURL,
		Metadata:         metadata,
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		UpdatedAt:        db.TimeFromPg(row.UpdatedAt),
	}
}

func normalizeChannel(channel string) string {
	return strings.ToLower(strings.TrimSpace(channel))
}

func toPgText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{
		String: value,
		Valid:  value != "",
	}
}

func emptyMetadataBytes() []byte {
	return []byte("{}")
}
