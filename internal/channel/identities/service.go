package identities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service provides channel identity lifecycle operations.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

var (
	ErrChannelIdentityNotFound = errors.New("channel identity not found")
)

// NewService creates a new channel identity service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
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
		return ChannelIdentity{}, fmt.Errorf("channel identity queries not configured")
	}
	channel = normalizeChannel(channel)
	channelSubjectID = strings.TrimSpace(channelSubjectID)
	if channel == "" || channelSubjectID == "" {
		return ChannelIdentity{}, fmt.Errorf("channel and channel_subject_id are required")
	}
	row, err := s.queries.CreateChannelIdentity(ctx, sqlc.CreateChannelIdentityParams{
		UserID:           pgtype.UUID{},
		ChannelType:      channel,
		ChannelSubjectID: channelSubjectID,
		DisplayName:      toPgText(displayName),
		AvatarUrl:        pgtype.Text{},
		Metadata:         emptyMetadataBytes(),
	})
	if err != nil {
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// GetByID returns a channel identity by its ID.
func (s *Service) GetByID(ctx context.Context, channelIdentityID string) (ChannelIdentity, error) {
	if s.queries == nil {
		return ChannelIdentity{}, fmt.Errorf("channel identity queries not configured")
	}
	pgID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return ChannelIdentity{}, err
	}
	row, err := s.queries.GetChannelIdentityByID(ctx, pgID)
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
		return "", fmt.Errorf("channel identity queries not configured")
	}
	pgID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return "", err
	}
	_, err = s.queries.GetChannelIdentityByID(ctx, pgID)
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
		return ChannelIdentity{}, fmt.Errorf("channel identity queries not configured")
	}
	channel = normalizeChannel(channel)
	channelSubjectID = strings.TrimSpace(channelSubjectID)
	if channel == "" || channelSubjectID == "" {
		return ChannelIdentity{}, fmt.Errorf("channel and channel_subject_id are required")
	}

	avatarURL := ""
	if meta != nil {
		if raw, ok := meta["avatar_url"]; ok {
			avatarURL = strings.TrimSpace(fmt.Sprint(raw))
		}
	}

	row, err := s.queries.UpsertChannelIdentityByChannelSubject(ctx, sqlc.UpsertChannelIdentityByChannelSubjectParams{
		UserID:           pgtype.UUID{},
		ChannelType:      channel,
		ChannelSubjectID: channelSubjectID,
		DisplayName:      toPgText(displayName),
		AvatarUrl:        toPgText(avatarURL),
		Metadata:         emptyMetadataBytes(),
	})
	if err != nil {
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// UpsertChannelIdentity creates or updates a channel identity mapping.
func (s *Service) UpsertChannelIdentity(ctx context.Context, channel, channelSubjectID, displayName string, metadata map[string]any) (ChannelIdentity, error) {
	if s.queries == nil {
		return ChannelIdentity{}, fmt.Errorf("channel identity queries not configured")
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
	row, err := s.queries.UpsertChannelIdentityByChannelSubject(ctx, sqlc.UpsertChannelIdentityByChannelSubjectParams{
		UserID:           pgtype.UUID{},
		ChannelType:      channel,
		ChannelSubjectID: channelSubjectID,
		DisplayName:      toPgText(displayName),
		AvatarUrl:        toPgText(avatarURL),
		Metadata:         metaBytes,
	})
	if err != nil {
		return ChannelIdentity{}, err
	}
	return toChannelIdentity(row), nil
}

// ListCanonicalChannelIdentities lists channel identities under the same linked user.
func (s *Service) ListCanonicalChannelIdentities(ctx context.Context, channelIdentityID string) ([]ChannelIdentity, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("channel identity queries not configured")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetChannelIdentityByID(ctx, pgChannelIdentityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrChannelIdentityNotFound
		}
		return nil, err
	}
	if !row.UserID.Valid {
		return []ChannelIdentity{toChannelIdentity(row)}, nil
	}
	rows, err := s.queries.ListChannelIdentitiesByUserID(ctx, row.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]ChannelIdentity, 0, len(rows))
	for _, item := range rows {
		result = append(result, toChannelIdentity(item))
	}
	return result, nil
}

// ListUserChannelIdentities lists all channel identities linked to a user.
func (s *Service) ListUserChannelIdentities(ctx context.Context, userID string) ([]ChannelIdentity, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("channel identity queries not configured")
	}
	pgUserID, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChannelIdentitiesByUserID(ctx, pgUserID)
	if err != nil {
		return nil, err
	}
	result := make([]ChannelIdentity, 0, len(rows))
	for _, row := range rows {
		result = append(result, toChannelIdentity(row))
	}
	return result, nil
}

// GetLinkedUserID returns the linked user ID for a channel identity.
func (s *Service) GetLinkedUserID(ctx context.Context, channelIdentityID string) (string, error) {
	if s.queries == nil {
		return "", fmt.Errorf("channel identity queries not configured")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return "", err
	}
	row, err := s.queries.GetChannelIdentityByID(ctx, pgChannelIdentityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if !row.UserID.Valid {
		return "", nil
	}
	return row.UserID.String(), nil
}

// LinkChannelIdentityToUser binds a channel identity to a user.
func (s *Service) LinkChannelIdentityToUser(ctx context.Context, channelIdentityID, userID string) error {
	if s.queries == nil {
		return fmt.Errorf("channel identity queries not configured")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return err
	}
	pgUserID, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	_, err = s.queries.SetChannelIdentityLinkedUser(ctx, sqlc.SetChannelIdentityLinkedUserParams{
		ID:     pgChannelIdentityID,
		UserID: pgUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrChannelIdentityNotFound
		}
		return err
	}
	return nil
}

func toChannelIdentity(row sqlc.ChannelIdentity) ChannelIdentity {
	var metadata map[string]any
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			slog.Warn("unmarshal channel identity metadata failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
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
	userID := ""
	if row.UserID.Valid {
		userID = row.UserID.String()
	}
	return ChannelIdentity{
		ID:               row.ID.String(),
		UserID:           userID,
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
