package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries  *sqlc.Queries
	logger   *slog.Logger
	registry *Registry
}

func NewService(log *slog.Logger, queries *sqlc.Queries, registry *Registry) *Service {
	return &Service{
		queries:  queries,
		logger:   log.With(slog.String("service", "tts")),
		registry: registry,
	}
}

func (s *Service) Registry() *Registry { return s.registry }

func (s *Service) ListMeta(_ context.Context) []ProviderMetaResponse {
	return s.registry.ListMeta()
}

// ---------------------------------------------------------------------------
// Provider CRUD
// ---------------------------------------------------------------------------

func (s *Service) CreateProvider(ctx context.Context, req CreateProviderRequest) (ProviderResponse, error) {
	adapter, err := s.registry.Get(req.Provider)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("unsupported provider: %s", req.Provider)
	}
	row, err := s.queries.CreateTtsProvider(ctx, sqlc.CreateTtsProviderParams{
		Name:     strings.TrimSpace(req.Name),
		Provider: string(req.Provider),
		Config:   []byte("{}"),
	})
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("create tts provider: %w", err)
	}

	if importErr := s.importModelsForProvider(ctx, row.ID, adapter); importErr != nil {
		s.logger.Warn("auto-import models failed", slog.String("provider_id", row.ID.String()), slog.Any("error", importErr))
	}

	return s.toProviderResponse(row), nil
}

func (s *Service) GetProvider(ctx context.Context, id string) (ProviderResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ProviderResponse{}, err
	}
	row, err := s.queries.GetTtsProviderByID(ctx, pgID)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("get tts provider: %w", err)
	}
	return s.toProviderResponse(row), nil
}

func (s *Service) ListProviders(ctx context.Context, provider string) ([]ProviderResponse, error) {
	provider = strings.TrimSpace(provider)
	var (
		rows []sqlc.TtsProvider
		err  error
	)
	if provider == "" {
		rows, err = s.queries.ListTtsProviders(ctx)
	} else {
		rows, err = s.queries.ListTtsProvidersByProvider(ctx, provider)
	}
	if err != nil {
		return nil, fmt.Errorf("list tts providers: %w", err)
	}
	items := make([]ProviderResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toProviderResponse(row))
	}
	return items, nil
}

func (s *Service) UpdateProvider(ctx context.Context, id string, req UpdateProviderRequest) (ProviderResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ProviderResponse{}, err
	}
	current, err := s.queries.GetTtsProviderByID(ctx, pgID)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("get tts provider: %w", err)
	}
	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	updated, err := s.queries.UpdateTtsProvider(ctx, sqlc.UpdateTtsProviderParams{
		ID:       pgID,
		Name:     name,
		Provider: current.Provider,
		Config:   current.Config,
	})
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("update tts provider: %w", err)
	}
	return s.toProviderResponse(updated), nil
}

func (s *Service) DeleteProvider(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteTtsProvider(ctx, pgID)
}

// ---------------------------------------------------------------------------
// Model CRUD
// ---------------------------------------------------------------------------

func (s *Service) ListModelsByProvider(ctx context.Context, providerID string) ([]ModelResponse, error) {
	pgID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	provider, err := s.queries.GetTtsProviderByID(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("get tts provider: %w", err)
	}
	rows, err := s.queries.ListTtsModelsByProviderID(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("list tts models: %w", err)
	}
	items := make([]ModelResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toModelResponse(row, provider.Provider))
	}
	return items, nil
}

func (s *Service) ListAllModels(ctx context.Context) ([]ModelResponse, error) {
	rows, err := s.queries.ListTtsModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tts models: %w", err)
	}
	providerCache := make(map[string]string)
	items := make([]ModelResponse, 0, len(rows))
	for _, row := range rows {
		providerType, ok := providerCache[row.TtsProviderID.String()]
		if !ok {
			p, pErr := s.queries.GetTtsProviderByID(ctx, row.TtsProviderID)
			if pErr != nil {
				providerType = ""
			} else {
				providerType = p.Provider
			}
			providerCache[row.TtsProviderID.String()] = providerType
		}
		items = append(items, s.toModelResponse(row, providerType))
	}
	return items, nil
}

func (s *Service) GetModel(ctx context.Context, id string) (ModelResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ModelResponse{}, err
	}
	row, err := s.queries.GetTtsModelWithProvider(ctx, pgID)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("get tts model: %w", err)
	}
	return s.toModelWithProviderResponse(row), nil
}

func (s *Service) UpdateModel(ctx context.Context, id string, req UpdateModelRequest) (ModelResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ModelResponse{}, err
	}
	current, err := s.queries.GetTtsModelByID(ctx, pgID)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("get tts model: %w", err)
	}
	name := current.Name
	if req.Name != nil {
		name = pgtype.Text{String: strings.TrimSpace(*req.Name), Valid: true}
	}
	config := current.Config
	if req.Config != nil {
		configJSON, marshalErr := json.Marshal(req.Config)
		if marshalErr != nil {
			return ModelResponse{}, fmt.Errorf("marshal config: %w", marshalErr)
		}
		config = configJSON
	}
	updated, err := s.queries.UpdateTtsModel(ctx, sqlc.UpdateTtsModelParams{
		ID:     pgID,
		Name:   name,
		Config: config,
	})
	if err != nil {
		return ModelResponse{}, fmt.Errorf("update tts model: %w", err)
	}
	provider, _ := s.queries.GetTtsProviderByID(ctx, updated.TtsProviderID)
	return s.toModelResponse(updated, provider.Provider), nil
}

func (s *Service) DeleteModel(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteTtsModel(ctx, pgID)
}

// ImportModels discovers models from the adapter and upserts them into the database.
func (s *Service) ImportModels(ctx context.Context, providerID string) ([]ModelResponse, error) {
	pgID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	provider, err := s.queries.GetTtsProviderByID(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("get tts provider: %w", err)
	}
	adapter, err := s.registry.Get(TtsType(provider.Provider))
	if err != nil {
		return nil, fmt.Errorf("unsupported provider: %s", provider.Provider)
	}
	if importErr := s.importModelsForProvider(ctx, pgID, adapter); importErr != nil {
		return nil, importErr
	}
	return s.ListModelsByProvider(ctx, providerID)
}

func (s *Service) importModelsForProvider(ctx context.Context, providerID pgtype.UUID, adapter TtsAdapter) error {
	models := adapter.Models()
	for _, m := range models {
		_, err := s.queries.UpsertTtsModel(ctx, sqlc.UpsertTtsModelParams{
			ModelID:       m.ID,
			Name:          pgtype.Text{String: m.Name, Valid: m.Name != ""},
			TtsProviderID: providerID,
			Config:        []byte("{}"),
		})
		if err != nil {
			return fmt.Errorf("upsert tts model %s: %w", m.ID, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Synthesis
// ---------------------------------------------------------------------------

// Synthesize runs text-to-speech using the saved model config, optionally
// overridden by fields in overrideCfg. Returns raw audio bytes.
func (s *Service) Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error) {
	pgID, err := db.ParseUUID(modelID)
	if err != nil {
		return nil, "", err
	}
	modelRow, err := s.queries.GetTtsModelWithProvider(ctx, pgID)
	if err != nil {
		return nil, "", fmt.Errorf("get tts model: %w", err)
	}
	adapter, err := s.registry.Get(TtsType(modelRow.ProviderType))
	if err != nil {
		return nil, "", fmt.Errorf("unsupported provider: %s", modelRow.ProviderType)
	}

	var savedCfg map[string]any
	if len(modelRow.Config) > 0 {
		_ = json.Unmarshal(modelRow.Config, &savedCfg)
	}
	if savedCfg == nil {
		savedCfg = make(map[string]any)
	}
	for k, v := range overrideCfg {
		savedCfg[k] = v
	}

	audioCfg := buildAudioConfig(savedCfg)
	if err := audioCfg.Validate(); err != nil {
		return nil, "", fmt.Errorf("invalid audio config: %w", err)
	}

	resolvedModel, _ := adapter.ResolveModel(modelRow.ModelID)
	audio, synthErr := adapter.Synthesize(ctx, text, resolvedModel, audioCfg)
	if synthErr != nil {
		return nil, "", fmt.Errorf("synthesize: %w", synthErr)
	}

	contentType := resolveContentType(audioCfg.Format)
	return audio, contentType, nil
}

// StreamToFile runs text-to-speech using Stream() and writes audio chunks
// directly to the given writer, keeping peak memory low for large audio.
func (s *Service) StreamToFile(ctx context.Context, modelID string, text string, w io.Writer) (string, error) {
	pgID, err := db.ParseUUID(modelID)
	if err != nil {
		return "", err
	}
	modelRow, err := s.queries.GetTtsModelWithProvider(ctx, pgID)
	if err != nil {
		return "", fmt.Errorf("get tts model: %w", err)
	}
	adapter, err := s.registry.Get(TtsType(modelRow.ProviderType))
	if err != nil {
		return "", fmt.Errorf("unsupported provider: %s", modelRow.ProviderType)
	}

	var savedCfg map[string]any
	if len(modelRow.Config) > 0 {
		_ = json.Unmarshal(modelRow.Config, &savedCfg)
	}
	if savedCfg == nil {
		savedCfg = make(map[string]any)
	}

	audioCfg := buildAudioConfig(savedCfg)
	if err := audioCfg.Validate(); err != nil {
		return "", fmt.Errorf("invalid audio config: %w", err)
	}

	resolvedModel, _ := adapter.ResolveModel(modelRow.ModelID)
	dataCh, errCh := adapter.Stream(ctx, text, resolvedModel, audioCfg)
	if dataCh == nil {
		select {
		case streamErr := <-errCh:
			return "", fmt.Errorf("stream: %w", streamErr)
		default:
			return "", errors.New("stream returned nil channels")
		}
	}

	for chunk := range dataCh {
		if _, writeErr := w.Write(chunk); writeErr != nil {
			return "", fmt.Errorf("write chunk: %w", writeErr)
		}
	}
	if streamErr, ok := <-errCh; ok && streamErr != nil {
		return "", fmt.Errorf("stream: %w", streamErr)
	}

	return resolveContentType(audioCfg.Format), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// GetModelCapabilities returns the adapter-level capabilities for a stored model.
func (s *Service) GetModelCapabilities(ctx context.Context, modelID string) (*ModelCapabilities, error) {
	pgID, err := db.ParseUUID(modelID)
	if err != nil {
		return nil, err
	}
	modelRow, err := s.queries.GetTtsModelWithProvider(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("get tts model: %w", err)
	}
	adapter, err := s.registry.Get(TtsType(modelRow.ProviderType))
	if err != nil {
		return nil, fmt.Errorf("unsupported provider: %s", modelRow.ProviderType)
	}
	for _, m := range adapter.Models() {
		if m.ID == modelRow.ModelID {
			return &m.Capabilities, nil
		}
	}
	return nil, fmt.Errorf("model %s not found in adapter", modelRow.ModelID)
}

func buildAudioConfig(cfg map[string]any) AudioConfig {
	ac := AudioConfig{}
	if voice, ok := cfg["voice"].(map[string]any); ok {
		if id, ok := voice["id"].(string); ok {
			ac.Voice.ID = id
		}
		if lang, ok := voice["lang"].(string); ok {
			ac.Voice.Lang = lang
		}
	}
	if format, ok := cfg["format"].(string); ok {
		ac.Format = format
	}
	if speed, ok := toFloat(cfg["speed"]); ok {
		ac.Speed = speed
	}
	if pitch, ok := toFloat(cfg["pitch"]); ok {
		ac.Pitch = pitch
	}
	if sr, ok := toFloat(cfg["sample_rate"]); ok {
		ac.SampleRate = int(sr)
	}
	return ac
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func resolveContentType(format string) string {
	switch {
	case strings.Contains(format, "mp3"):
		return "audio/mpeg"
	case strings.Contains(format, "opus"):
		return "audio/opus"
	case strings.Contains(format, "ogg"):
		return "audio/ogg"
	case strings.Contains(format, "webm"):
		return "audio/webm"
	case strings.Contains(format, "wav"):
		return "audio/wav"
	default:
		return "audio/mpeg"
	}
}

func (*Service) toProviderResponse(row sqlc.TtsProvider) ProviderResponse {
	return ProviderResponse{
		ID:        row.ID.String(),
		Name:      row.Name,
		Provider:  row.Provider,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func (s *Service) toModelResponse(row sqlc.TtsModel, providerType string) ModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("tts model config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return ModelResponse{
		ID:            row.ID.String(),
		ModelID:       row.ModelID,
		Name:          name,
		TtsProviderID: row.TtsProviderID.String(),
		ProviderType:  providerType,
		Config:        cfg,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func (s *Service) toModelWithProviderResponse(row sqlc.GetTtsModelWithProviderRow) ModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("tts model config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return ModelResponse{
		ID:            row.ID.String(),
		ModelID:       row.ModelID,
		Name:          name,
		TtsProviderID: row.TtsProviderID.String(),
		ProviderType:  row.ProviderType,
		Config:        cfg,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}
