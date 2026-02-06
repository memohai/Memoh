package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
}

func NewService(queries *sqlc.Queries) *Service {
	return &Service{queries: queries}
}

func (s *Service) GetByID(ctx context.Context, contactID string) (Contact, error) {
	if s.queries == nil {
		return Contact{}, fmt.Errorf("contacts queries not configured")
	}
	pgID, err := parseUUID(contactID)
	if err != nil {
		return Contact{}, err
	}
	row, err := s.queries.GetContactByID(ctx, pgID)
	if err != nil {
		return Contact{}, err
	}
	return normalizeContact(row)
}

func (s *Service) GetByUserID(ctx context.Context, botID, userID string) (Contact, error) {
	if s.queries == nil {
		return Contact{}, fmt.Errorf("contacts queries not configured")
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Contact{}, err
	}
	pgUserID, err := parseUUID(userID)
	if err != nil {
		return Contact{}, err
	}
	row, err := s.queries.GetContactByUserID(ctx, sqlc.GetContactByUserIDParams{
		BotID:  pgBotID,
		UserID: pgUserID,
	})
	if err != nil {
		return Contact{}, err
	}
	return normalizeContact(row)
}

func (s *Service) GetByChannelIdentity(ctx context.Context, botID, platform, externalID string) (ContactChannel, error) {
	if s.queries == nil {
		return ContactChannel{}, fmt.Errorf("contacts queries not configured")
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return ContactChannel{}, err
	}
	row, err := s.queries.GetContactChannelByIdentity(ctx, sqlc.GetContactChannelByIdentityParams{
		BotID:      pgBotID,
		Platform:   platform,
		ExternalID: externalID,
	})
	if err != nil {
		return ContactChannel{}, err
	}
	return normalizeContactChannel(row)
}

func (s *Service) ListChannelsByContact(ctx context.Context, contactID string) ([]ContactChannel, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("contacts queries not configured")
	}
	pgContactID, err := parseUUID(contactID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListContactChannelsByContact(ctx, pgContactID)
	if err != nil {
		return nil, err
	}
	items := make([]ContactChannel, 0, len(rows))
	for _, row := range rows {
		item, err := normalizeContactChannel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListByBot(ctx context.Context, botID string) ([]Contact, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("contacts queries not configured")
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListContactsByBot(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	items := make([]Contact, 0, len(rows))
	for _, row := range rows {
		contact, err := normalizeContact(row)
		if err != nil {
			return nil, err
		}
		items = append(items, contact)
	}
	return items, nil
}

func (s *Service) Search(ctx context.Context, botID, query string) ([]Contact, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("contacts queries not configured")
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return s.ListByBot(ctx, botID)
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return nil, err
	}
	search := "%" + trimmed + "%"
	rows, err := s.queries.SearchContacts(ctx, sqlc.SearchContactsParams{
		BotID: pgBotID,
		Query: pgtype.Text{String: search, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	items := make([]Contact, 0, len(rows))
	for _, row := range rows {
		contact, err := normalizeContact(row)
		if err != nil {
			return nil, err
		}
		items = append(items, contact)
	}
	return items, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Contact, error) {
	if s.queries == nil {
		return Contact{}, fmt.Errorf("contacts queries not configured")
	}
	pgBotID, err := parseUUID(req.BotID)
	if err != nil {
		return Contact{}, err
	}
	pgUserID := pgtype.UUID{Valid: false}
	if strings.TrimSpace(req.UserID) != "" {
		parsed, err := parseUUID(req.UserID)
		if err != nil {
			return Contact{}, err
		}
		pgUserID = parsed
	}
	payload, err := json.Marshal(defaultMetadata(req.Metadata))
	if err != nil {
		return Contact{}, err
	}
	row, err := s.queries.CreateContact(ctx, sqlc.CreateContactParams{
		BotID:       pgBotID,
		UserID:      pgUserID,
		DisplayName: pgtype.Text{String: strings.TrimSpace(req.DisplayName), Valid: strings.TrimSpace(req.DisplayName) != ""},
		Alias:       pgtype.Text{String: strings.TrimSpace(req.Alias), Valid: strings.TrimSpace(req.Alias) != ""},
		Tags:        normalizeTags(req.Tags),
		Status:      normalizeStatus(req.Status),
		Metadata:    payload,
	})
	if err != nil {
		return Contact{}, err
	}
	return normalizeContact(row)
}

func (s *Service) CreateGuest(ctx context.Context, botID, displayName string) (Contact, error) {
	return s.Create(ctx, CreateRequest{
		BotID:       botID,
		DisplayName: displayName,
		Status:      "active",
	})
}

func (s *Service) Update(ctx context.Context, contactID string, req UpdateRequest) (Contact, error) {
	if s.queries == nil {
		return Contact{}, fmt.Errorf("contacts queries not configured")
	}
	pgID, err := parseUUID(contactID)
	if err != nil {
		return Contact{}, err
	}
	var displayName pgtype.Text
	if req.DisplayName != nil {
		displayName = pgtype.Text{String: strings.TrimSpace(*req.DisplayName), Valid: strings.TrimSpace(*req.DisplayName) != ""}
	}
	var alias pgtype.Text
	if req.Alias != nil {
		alias = pgtype.Text{String: strings.TrimSpace(*req.Alias), Valid: strings.TrimSpace(*req.Alias) != ""}
	}
	var tags []string
	if req.Tags != nil {
		tags = normalizeTags(*req.Tags)
	}
	status := ""
	if req.Status != nil {
		status = normalizeStatus(*req.Status)
	}
	var metadata []byte
	if req.Metadata != nil {
		encoded, err := json.Marshal(defaultMetadata(req.Metadata))
		if err != nil {
			return Contact{}, err
		}
		metadata = encoded
	}
	row, err := s.queries.UpdateContact(ctx, sqlc.UpdateContactParams{
		ID:          pgID,
		DisplayName: displayName,
		Alias:       alias,
		Tags:        tags,
		Status:      status,
		Metadata:    metadata,
	})
	if err != nil {
		return Contact{}, err
	}
	return normalizeContact(row)
}

func (s *Service) BindUser(ctx context.Context, contactID, userID string) (Contact, error) {
	if s.queries == nil {
		return Contact{}, fmt.Errorf("contacts queries not configured")
	}
	pgContactID, err := parseUUID(contactID)
	if err != nil {
		return Contact{}, err
	}
	pgUserID, err := parseUUID(userID)
	if err != nil {
		return Contact{}, err
	}
	row, err := s.queries.UpdateContactUser(ctx, sqlc.UpdateContactUserParams{
		ID:     pgContactID,
		UserID: pgUserID,
	})
	if err != nil {
		return Contact{}, err
	}
	return normalizeContact(row)
}

func (s *Service) UpsertChannel(ctx context.Context, botID, contactID, platform, externalID string, metadata map[string]any) (ContactChannel, error) {
	if s.queries == nil {
		return ContactChannel{}, fmt.Errorf("contacts queries not configured")
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return ContactChannel{}, err
	}
	pgContactID, err := parseUUID(contactID)
	if err != nil {
		return ContactChannel{}, err
	}
	payload, err := json.Marshal(defaultMetadata(metadata))
	if err != nil {
		return ContactChannel{}, err
	}
	row, err := s.queries.UpsertContactChannel(ctx, sqlc.UpsertContactChannelParams{
		BotID:      pgBotID,
		ContactID:  pgContactID,
		Platform:   strings.TrimSpace(platform),
		ExternalID: strings.TrimSpace(externalID),
		Metadata:   payload,
	})
	if err != nil {
		return ContactChannel{}, err
	}
	return normalizeContactChannel(row)
}

func normalizeContact(row sqlc.Contact) (Contact, error) {
	metadata, err := decodeMetadata(row.Metadata)
	if err != nil {
		return Contact{}, err
	}
	return Contact{
		ID:          toUUIDString(row.ID),
		BotID:       toUUIDString(row.BotID),
		UserID:      toUUIDString(row.UserID),
		DisplayName: strings.TrimSpace(row.DisplayName.String),
		Alias:       strings.TrimSpace(row.Alias.String),
		Tags:        normalizeTags(row.Tags),
		Status:      strings.TrimSpace(row.Status),
		Metadata:    metadata,
		CreatedAt:   timeFromPg(row.CreatedAt),
		UpdatedAt:   timeFromPg(row.UpdatedAt),
	}, nil
}

func normalizeContactChannel(row sqlc.ContactChannel) (ContactChannel, error) {
	metadata, err := decodeMetadata(row.Metadata)
	if err != nil {
		return ContactChannel{}, err
	}
	return ContactChannel{
		ID:         toUUIDString(row.ID),
		BotID:      toUUIDString(row.BotID),
		ContactID:  toUUIDString(row.ContactID),
		Platform:   strings.TrimSpace(row.Platform),
		ExternalID: strings.TrimSpace(row.ExternalID),
		Metadata:   metadata,
		CreatedAt:  timeFromPg(row.CreatedAt),
		UpdatedAt:  timeFromPg(row.UpdatedAt),
	}, nil
}

func decodeMetadata(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func defaultMetadata(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}

func toUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func normalizeStatus(status string) string {
	trimmed := strings.ToLower(strings.TrimSpace(status))
	switch trimmed {
	case "active", "blocked", "pending":
		return trimmed
	case "":
		return "active"
	default:
		return "active"
	}
}
