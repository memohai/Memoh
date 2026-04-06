package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// ---------------------------------------------------------------------------
// Read helpers (speech-filtered views of unified tables)
// ---------------------------------------------------------------------------

// ListSpeechProviders returns providers with speech client types.
func (s *Service) ListSpeechProviders(ctx context.Context) ([]SpeechProviderResponse, error) {
	rows, err := s.queries.ListSpeechProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list speech providers: %w", err)
	}
	items := make([]SpeechProviderResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSpeechProviderResponse(row))
	}
	return items, nil
}

// ListSpeechModels returns all speech-type models.
func (s *Service) ListSpeechModels(ctx context.Context) ([]SpeechModelResponse, error) {
	rows, err := s.queries.ListSpeechModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list speech models: %w", err)
	}
	items := make([]SpeechModelResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSpeechModelFromListRow(row))
	}
	return items, nil
}

// ListSpeechModelsByProvider returns speech models for a given provider.
func (s *Service) ListSpeechModelsByProvider(ctx context.Context, providerID string) ([]SpeechModelResponse, error) {
	pgID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSpeechModelsByProviderID(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("list speech models by provider: %w", err)
	}
	items := make([]SpeechModelResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSpeechModelFromModel(row, ""))
	}
	return items, nil
}

// GetSpeechModel returns a speech model by ID.
func (s *Service) GetSpeechModel(ctx context.Context, id string) (SpeechModelResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return SpeechModelResponse{}, err
	}
	row, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return SpeechModelResponse{}, fmt.Errorf("get speech model: %w", err)
	}
	return toSpeechModelWithProviderResponse(row), nil
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
	modelRow, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return nil, "", fmt.Errorf("get speech model: %w", err)
	}
	adapterType := clientTypeToTtsType(modelRow.ProviderType)
	adapter, err := s.registry.Get(adapterType)
	if err != nil {
		return nil, "", fmt.Errorf("unsupported provider: %s", modelRow.ProviderType)
	}

	savedCfg := parseModelConfig(modelRow.Config)
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
	modelRow, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return "", fmt.Errorf("get speech model: %w", err)
	}
	adapterType := clientTypeToTtsType(modelRow.ProviderType)
	adapter, err := s.registry.Get(adapterType)
	if err != nil {
		return "", fmt.Errorf("unsupported provider: %s", modelRow.ProviderType)
	}

	savedCfg := parseModelConfig(modelRow.Config)
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
// Capabilities
// ---------------------------------------------------------------------------

// GetModelCapabilities returns the adapter-level capabilities for a stored model.
func (s *Service) GetModelCapabilities(ctx context.Context, modelID string) (*ModelCapabilities, error) {
	pgID, err := db.ParseUUID(modelID)
	if err != nil {
		return nil, err
	}
	modelRow, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("get speech model: %w", err)
	}
	adapterType := clientTypeToTtsType(modelRow.ProviderType)
	adapter, err := s.registry.Get(adapterType)
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// clientTypeToTtsType maps the unified client_type to the TTS adapter type.
func clientTypeToTtsType(clientType string) TtsType {
	switch clientType {
	case "edge-speech":
		return "edge"
	default:
		return TtsType(clientType)
	}
}

func parseModelConfig(raw []byte) map[string]any {
	if len(raw) == 0 {
		return make(map[string]any)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return make(map[string]any)
	}
	if cfg == nil {
		return make(map[string]any)
	}
	return cfg
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

func toSpeechProviderResponse(row sqlc.Provider) SpeechProviderResponse {
	return SpeechProviderResponse{
		ID:         row.ID.String(),
		Name:       row.Name,
		ClientType: row.ClientType,
		Enable:     row.Enable,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toSpeechModelFromListRow(row sqlc.ListSpeechModelsRow) SpeechModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return SpeechModelResponse{
		ID:           row.ID.String(),
		ModelID:      row.ModelID,
		Name:         name,
		ProviderID:   row.ProviderID.String(),
		ProviderType: row.ProviderType,
		Config:       cfg,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func toSpeechModelFromModel(row sqlc.Model, providerType string) SpeechModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return SpeechModelResponse{
		ID:           row.ID.String(),
		ModelID:      row.ModelID,
		Name:         name,
		ProviderID:   row.ProviderID.String(),
		ProviderType: providerType,
		Config:       cfg,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func toSpeechModelWithProviderResponse(row sqlc.GetSpeechModelWithProviderRow) SpeechModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return SpeechModelResponse{
		ID:           row.ID.String(),
		ModelID:      row.ModelID,
		Name:         name,
		ProviderID:   row.ProviderID.String(),
		ProviderType: row.ProviderType,
		Config:       cfg,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}
