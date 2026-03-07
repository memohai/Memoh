package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

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

func (s *Service) CreateProvider(ctx context.Context, req CreateProviderRequest) (ProviderResponse, error) {
	if _, err := s.registry.Get(req.Provider); err != nil {
		return ProviderResponse{}, fmt.Errorf("unsupported provider: %s", req.Provider)
	}
	if req.Config == nil {
		req.Config = make(map[string]any)
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("marshal config: %w", err)
	}
	row, err := s.queries.CreateTtsProvider(ctx, sqlc.CreateTtsProviderParams{
		Name:     strings.TrimSpace(req.Name),
		Provider: string(req.Provider),
		Config:   configJSON,
	})
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("create tts provider: %w", err)
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
	provider := current.Provider
	if req.Provider != nil {
		if _, err := s.registry.Get(*req.Provider); err != nil {
			return ProviderResponse{}, fmt.Errorf("unsupported provider: %s", *req.Provider)
		}
		provider = string(*req.Provider)
	}
	config := current.Config
	if req.Config != nil {
		configJSON, marshalErr := json.Marshal(req.Config)
		if marshalErr != nil {
			return ProviderResponse{}, fmt.Errorf("marshal config: %w", marshalErr)
		}
		config = configJSON
	}
	updated, err := s.queries.UpdateTtsProvider(ctx, sqlc.UpdateTtsProviderParams{
		ID:       pgID,
		Name:     name,
		Provider: provider,
		Config:   config,
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

// Synthesize runs text-to-speech using the saved provider config, optionally
// overridden by fields in overrideCfg. Returns raw audio bytes.
func (s *Service) Synthesize(ctx context.Context, id string, text string, overrideCfg map[string]any) ([]byte, string, error) {
	resp, err := s.GetProvider(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("get tts provider: %w", err)
	}
	adapter, err := s.registry.Get(TtsType(resp.Provider))
	if err != nil {
		return nil, "", fmt.Errorf("unsupported provider: %s", resp.Provider)
	}

	merged := make(map[string]any)
	for k, v := range resp.Config {
		merged[k] = v
	}
	for k, v := range overrideCfg {
		merged[k] = v
	}

	audioCfg := buildAudioConfig(merged)
	if err := audioCfg.Validate(); err != nil {
		return nil, "", fmt.Errorf("invalid audio config: %w", err)
	}

	audio, synthErr := adapter.Synthesize(ctx, text, audioCfg)
	if synthErr != nil {
		return nil, "", fmt.Errorf("synthesize: %w", synthErr)
	}

	contentType := resolveContentType(audioCfg.Format)
	return audio, contentType, nil
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

func (s *Service) toProviderResponse(row sqlc.TtsProvider) ProviderResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("tts provider config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	return ProviderResponse{
		ID:        row.ID.String(),
		Name:      row.Name,
		Provider:  row.Provider,
		Config:    cfg,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
