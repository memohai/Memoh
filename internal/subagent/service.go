package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "subagent")),
	}
}

func (s *Service) Create(ctx context.Context, botID string, req CreateRequest) (Subagent, error) {
	if s.queries == nil {
		return Subagent{}, fmt.Errorf("subagent queries not configured")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Subagent{}, fmt.Errorf("name is required")
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return Subagent{}, fmt.Errorf("description is required")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Subagent{}, err
	}
	messagesPayload, err := marshalMessages(req.Messages)
	if err != nil {
		return Subagent{}, err
	}
	metadataPayload, err := marshalMetadata(req.Metadata)
	if err != nil {
		return Subagent{}, err
	}
	skillsPayload, err := marshalSkills(req.Skills)
	if err != nil {
		return Subagent{}, err
	}
	row, err := s.queries.CreateSubagent(ctx, sqlc.CreateSubagentParams{
		Name:        name,
		Description: description,
		BotID:       pgBotID,
		Messages:    messagesPayload,
		Metadata:    metadataPayload,
		Skills:      skillsPayload,
	})
	if err != nil {
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) Get(ctx context.Context, id string) (Subagent, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Subagent{}, err
	}
	row, err := s.queries.GetSubagentByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Subagent{}, fmt.Errorf("subagent not found")
		}
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) GetByBotAndName(ctx context.Context, botID string, name string) (Subagent, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Subagent{}, err
	}
	row, err := s.queries.GetSubagentByBotAndName(ctx, sqlc.GetSubagentByBotAndNameParams{
		BotID: pgBotID,
		Name:  strings.TrimSpace(name),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Subagent{}, fmt.Errorf("subagent not found")
		}
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) GetOrCreate(ctx context.Context, botID string, req CreateRequest) (Subagent, error) {
	existing, err := s.GetByBotAndName(ctx, botID, req.Name)
	if err == nil {
		return existing, nil
	}
	return s.Create(ctx, botID, req)
}

func (s *Service) List(ctx context.Context, botID string) ([]Subagent, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSubagentsByBot(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	items := make([]Subagent, 0, len(rows))
	for _, row := range rows {
		item, err := toSubagent(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Subagent, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return Subagent{}, err
	}
	name := existing.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			return Subagent{}, fmt.Errorf("name is required")
		}
	}
	description := existing.Description
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
		if description == "" {
			return Subagent{}, fmt.Errorf("description is required")
		}
	}
	metadata := existing.Metadata
	if req.Metadata != nil {
		metadata = req.Metadata
	}
	metadataPayload, err := marshalMetadata(metadata)
	if err != nil {
		return Subagent{}, err
	}
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Subagent{}, err
	}
	row, err := s.queries.UpdateSubagent(ctx, sqlc.UpdateSubagentParams{
		ID:          pgID,
		Name:        name,
		Description: description,
		Metadata:    metadataPayload,
	})
	if err != nil {
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) UpdateContext(ctx context.Context, id string, req UpdateContextRequest) (Subagent, error) {
	messagesPayload, err := marshalMessages(req.Messages)
	if err != nil {
		return Subagent{}, err
	}
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Subagent{}, err
	}
	if req.Usage != nil {
		usagePayload, err := marshalUsage(req.Usage)
		if err != nil {
			return Subagent{}, err
		}
		row, err := s.queries.UpdateSubagentMessagesAndUsage(ctx, sqlc.UpdateSubagentMessagesAndUsageParams{
			ID:       pgID,
			Messages: messagesPayload,
			Usage:    usagePayload,
		})
		if err != nil {
			return Subagent{}, err
		}
		return toSubagent(row)
	}
	row, err := s.queries.UpdateSubagentMessages(ctx, sqlc.UpdateSubagentMessagesParams{
		ID:       pgID,
		Messages: messagesPayload,
	})
	if err != nil {
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) UpdateSkills(ctx context.Context, id string, req UpdateSkillsRequest) (Subagent, error) {
	skillsPayload, err := marshalSkills(req.Skills)
	if err != nil {
		return Subagent{}, err
	}
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Subagent{}, err
	}
	row, err := s.queries.UpdateSubagentSkills(ctx, sqlc.UpdateSubagentSkillsParams{
		ID:     pgID,
		Skills: skillsPayload,
	})
	if err != nil {
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) AddSkills(ctx context.Context, id string, req AddSkillsRequest) (Subagent, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return Subagent{}, err
	}
	merged := mergeSkills(existing.Skills, req.Skills)
	payload, err := marshalSkills(merged)
	if err != nil {
		return Subagent{}, err
	}
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Subagent{}, err
	}
	row, err := s.queries.UpdateSubagentSkills(ctx, sqlc.UpdateSubagentSkillsParams{
		ID:     pgID,
		Skills: payload,
	})
	if err != nil {
		return Subagent{}, err
	}
	return toSubagent(row)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.SoftDeleteSubagent(ctx, pgID)
}

func toSubagent(row sqlc.Subagent) (Subagent, error) {
	messages, err := unmarshalMessages(row.Messages)
	if err != nil {
		return Subagent{}, err
	}
	metadata, err := unmarshalMetadata(row.Metadata)
	if err != nil {
		return Subagent{}, err
	}
	skills, err := unmarshalSkills(row.Skills)
	if err != nil {
		return Subagent{}, err
	}
	usage, err := unmarshalUsage(row.Usage)
	if err != nil {
		return Subagent{}, err
	}
	item := Subagent{
		ID:          row.ID.String(),
		Name:        row.Name,
		Description: row.Description,
		BotID:       row.BotID.String(),
		Messages:    messages,
		Metadata:    metadata,
		Skills:      skills,
		Usage:       usage,
		Deleted:     row.Deleted,
	}
	if row.CreatedAt.Valid {
		item.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		item.UpdatedAt = row.UpdatedAt.Time
	}
	if row.DeletedAt.Valid {
		deletedAt := row.DeletedAt.Time
		item.DeletedAt = &deletedAt
	}
	return item, nil
}

func marshalMessages(messages []map[string]any) ([]byte, error) {
	if messages == nil {
		messages = []map[string]any{}
	}
	return json.Marshal(messages)
}

func unmarshalMessages(payload []byte) ([]map[string]any, error) {
	if len(payload) == 0 {
		return []map[string]any{}, nil
	}
	var messages []map[string]any
	if err := json.Unmarshal(payload, &messages); err != nil {
		return nil, err
	}
	if messages == nil {
		messages = []map[string]any{}
	}
	return messages, nil
}

func marshalMetadata(metadata map[string]any) ([]byte, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return json.Marshal(metadata)
}

func unmarshalMetadata(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return nil, err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return metadata, nil
}

func marshalUsage(usage map[string]any) ([]byte, error) {
	if usage == nil {
		usage = map[string]any{}
	}
	return json.Marshal(usage)
}

func unmarshalUsage(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var usage map[string]any
	if err := json.Unmarshal(payload, &usage); err != nil {
		return nil, err
	}
	if usage == nil {
		usage = map[string]any{}
	}
	return usage, nil
}

func marshalSkills(skills []string) ([]byte, error) {
	return json.Marshal(normalizeSkills(skills))
}

func unmarshalSkills(payload []byte) ([]string, error) {
	if len(payload) == 0 {
		return []string{}, nil
	}
	var skills []string
	if err := json.Unmarshal(payload, &skills); err != nil {
		return nil, err
	}
	if skills == nil {
		skills = []string{}
	}
	return skills, nil
}

func normalizeSkills(skills []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(skills))
	for _, skill := range skills {
		trimmed := strings.TrimSpace(skill)
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

func mergeSkills(existing []string, incoming []string) []string {
	merged := append([]string{}, existing...)
	merged = append(merged, incoming...)
	return normalizeSkills(merged)
}
