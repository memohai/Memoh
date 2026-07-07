package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

func (r *Resolver) selectChatModel(ctx context.Context, req conversation.ChatRequest, botSettings settings.Settings, cs conversation.Settings) (models.GetResponse, sqlc.Provider, error) {
	if r.modelsService == nil {
		return models.GetResponse{}, sqlc.Provider{}, errors.New("models service not configured")
	}
	modelID := strings.TrimSpace(req.Model)
	providerFilter := strings.TrimSpace(req.Provider)

	// Priority: request model > chat settings > bot settings.
	if modelID == "" && providerFilter == "" {
		if value := strings.TrimSpace(cs.ModelID); value != "" {
			modelID = value
		} else if value := strings.TrimSpace(botSettings.ChatModelID); value != "" {
			modelID = value
		}
	}

	if modelID == "" {
		return models.GetResponse{}, sqlc.Provider{}, errors.New("chat model not configured: specify model in request or bot settings")
	}

	if providerFilter == "" {
		return r.fetchChatModel(ctx, modelID)
	}

	candidates, err := r.listCandidates(ctx, providerFilter)
	if err != nil {
		return models.GetResponse{}, sqlc.Provider{}, err
	}
	for _, m := range candidates {
		if matchesModelReference(m, modelID) {
			prov, err := models.FetchProviderByID(ctx, r.queries, m.ProviderID)
			if err != nil {
				return models.GetResponse{}, sqlc.Provider{}, err
			}
			if err := validateSelectedChatModel(m, prov); err != nil {
				return models.GetResponse{}, sqlc.Provider{}, err
			}
			if !prov.Enable {
				return models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("chat model provider %s is disabled", prov.Name)
			}
			return m, prov, nil
		}
	}
	return models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("chat model %q not found for provider %q", modelID, providerFilter)
}

func (r *Resolver) fetchChatModel(ctx context.Context, modelID string) (models.GetResponse, sqlc.Provider, error) {
	modelRef := strings.TrimSpace(modelID)
	if modelRef == "" {
		return models.GetResponse{}, sqlc.Provider{}, errors.New("model id is required")
	}

	// Support both model UUID and model_id slug. UUID-formatted slugs still
	// work because we fall back to GetByModelID when UUID lookup misses.
	var model models.GetResponse
	var err error
	if _, parseErr := db.ParseUUID(modelRef); parseErr == nil {
		model, err = r.modelsService.GetByID(ctx, modelRef)
		if err == nil {
			goto resolved
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return models.GetResponse{}, sqlc.Provider{}, err
		}
	}
	model, err = r.modelsService.GetByModelID(ctx, modelRef)
	if err != nil {
		return models.GetResponse{}, sqlc.Provider{}, err
	}

resolved:
	prov, err := models.FetchProviderByID(ctx, r.queries, model.ProviderID)
	if err != nil {
		return models.GetResponse{}, sqlc.Provider{}, err
	}
	if err := validateSelectedChatModel(model, prov); err != nil {
		return models.GetResponse{}, sqlc.Provider{}, err
	}
	if !prov.Enable {
		return models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("chat model provider %s is disabled", prov.Name)
	}
	return model, prov, nil
}

func validateSelectedChatModel(model models.GetResponse, provider sqlc.Provider) error {
	if model.Type != models.ModelTypeChat {
		return errors.New("model is not a chat model")
	}
	if !model.Enable {
		return fmt.Errorf("chat model %s is disabled", model.ModelID)
	}
	if isImageOnlyChatModel(model, provider) {
		return fmt.Errorf("chat model %s is an image generation model; configure it as the bot image model and use a chat model for conversation", model.ModelID)
	}
	return nil
}

func isImageOnlyChatModel(model models.GetResponse, provider sqlc.Provider) bool {
	// A model that advertises tool calling is usable as a chat model regardless
	// of its name — this is the escape hatch for the name heuristic below, so a
	// tool-capable model that merely looks like an image model (or a genuine
	// multimodal chat model) is never blocked.
	if model.HasCompatibility(models.CompatToolCall) {
		return false
	}
	lowerModel := strings.ToLower(strings.TrimSpace(model.ModelID))
	if lowerModel == "" {
		return false
	}
	if isKnownStandaloneImageModelID(lowerModel) {
		return true
	}
	lowerBase := strings.ToLower(providerConfigString(provider, "base_url"))
	if strings.Contains(lowerBase, "dashscope") && strings.Contains(lowerModel, "image") {
		return true
	}
	if !model.HasCompatibility(models.CompatImageOutput) {
		return false
	}
	ct := models.ClientType(provider.ClientType)
	if ct != models.ClientTypeOpenAICompletions && ct != models.ClientTypeOpenAIResponses {
		return false
	}
	return strings.Contains(lowerBase, "maas.aliyuncs.com") ||
		strings.Contains(lowerBase, "api.openai.com") ||
		strings.Contains(lowerBase, "volces.com") ||
		strings.Contains(lowerBase, "bytepluses.com") ||
		strings.Contains(lowerBase, "siliconflow")
}

// isKnownStandaloneImageModelID matches the naming conventions of dedicated
// text-to-image model families. Prefixes are kept specific enough not to catch
// ordinary chat models that merely share a leading token — e.g. "wan2"/"wanx"
// (Alibaba Wan image/video) rather than a bare "wan" that would also match a
// chat model like "wanjuan-chat", and "flux-"/"flux."/"flux1" rather than a
// bare "flux". A tool-calling model bypasses this check entirely (see
// isImageOnlyChatModel), which is the override when a name collision is wrong.
func isKnownStandaloneImageModelID(lowerModel string) bool {
	return strings.HasPrefix(lowerModel, "qwen-image") ||
		strings.HasPrefix(lowerModel, "wan2") ||
		strings.HasPrefix(lowerModel, "wanx") ||
		strings.HasPrefix(lowerModel, "z-image") ||
		strings.HasPrefix(lowerModel, "flux-") ||
		strings.HasPrefix(lowerModel, "flux.") ||
		strings.HasPrefix(lowerModel, "flux1") ||
		strings.HasPrefix(lowerModel, "stable-diffusion") ||
		strings.HasPrefix(lowerModel, "gpt-image") ||
		strings.HasPrefix(lowerModel, "dall-e") ||
		strings.Contains(lowerModel, "seedream")
}

func providerConfigString(provider sqlc.Provider, key string) string {
	var cfg map[string]any
	if len(provider.Config) == 0 {
		return ""
	}
	if err := json.Unmarshal(provider.Config, &cfg); err != nil {
		return ""
	}
	value, _ := cfg[key].(string)
	return strings.TrimSpace(value)
}

func matchesModelReference(model models.GetResponse, modelRef string) bool {
	ref := strings.TrimSpace(modelRef)
	if ref == "" {
		return false
	}
	return model.ID == ref || model.ModelID == ref
}

func (r *Resolver) listCandidates(ctx context.Context, providerFilter string) ([]models.GetResponse, error) {
	var all []models.GetResponse
	var err error
	if providerFilter != "" {
		all, err = r.modelsService.ListEnabledByProviderClientType(ctx, models.ClientType(providerFilter))
	} else {
		all, err = r.modelsService.ListEnabledByType(ctx, models.ModelTypeChat)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]models.GetResponse, 0, len(all))
	for _, m := range all {
		if m.Type == models.ModelTypeChat {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}
