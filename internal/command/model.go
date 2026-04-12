package command

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildModelGroup() *CommandGroup {
	g := newCommandGroup("model", "Manage bot models")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list [provider_name] - List available chat models",
		Handler: func(cc CommandContext) (string, error) {
			if h.modelsService == nil {
				return "Model service is not available.", nil
			}
			items, err := h.modelsService.ListByType(cc.Ctx, models.ModelTypeChat)
			if err != nil {
				return "", err
			}
			filterProvider := ""
			if len(cc.Args) > 0 {
				filterProvider = strings.TrimSpace(strings.Join(cc.Args, " "))
			}
			items = h.filterModelsByProvider(cc, items, filterProvider)
			if len(items) == 0 {
				if filterProvider != "" {
					return fmt.Sprintf("No chat models found for provider %q.", filterProvider), nil
				}
				return "No chat models found.", nil
			}
			settingsResp, _ := h.getBotSettings(cc)
			sort.SliceStable(items, func(i, j int) bool {
				return modelSortRank(items[i], settingsResp) < modelSortRank(items[j], settingsResp)
			})
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				provName := h.resolveProviderName(cc, item.ProviderID)
				label := item.Name
				markers := modelMarkers(item.ID, settingsResp)
				if len(markers) > 0 {
					label += " [" + strings.Join(markers, ", ") + "]"
				}
				records = append(records, []kv{
					{"Model", label},
					{"Provider", provName},
					{"Model ID", item.ModelID},
				})
			}
			hint := "Use /model current to inspect active selections."
			if filterProvider == "" {
				hint = "Use /model list <provider_name> to narrow results."
			}
			return formatLimitedItems(records, defaultListLimit, hint), nil
		},
	})
	g.Register(SubCommand{
		Name:  "current",
		Usage: "current - Show current chat and heartbeat models",
		Handler: func(cc CommandContext) (string, error) {
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			settingsResp, err := h.getBotSettings(cc)
			if err != nil {
				return "", err
			}
			return formatKV([]kv{
				{"Chat Model", h.resolveModelName(cc, settingsResp.ChatModelID)},
				{"Heartbeat Model", h.resolveModelName(cc, settingsResp.HeartbeatModelID)},
			}), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <model_id> | <provider_name> <model_name> - Set the chat model",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /model set <model_id> | <provider_name> <model_name>", nil
			}
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			before, _ := h.getBotSettings(cc)
			modelResp, err := h.findModelForSelection(cc, cc.Args)
			if err != nil {
				return "", err
			}
			_, err = h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
				ChatModelID: modelResp.ID,
			})
			if err != nil {
				return "", err
			}
			return formatChangedValue("Chat model", h.resolveModelName(cc, before.ChatModelID), h.resolveModelName(cc, modelResp.ID)), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set-heartbeat",
		Usage:   "set-heartbeat <model_id> | <provider_name> <model_name> - Set the heartbeat model",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 1 {
				return "Usage: /model set-heartbeat <model_id> | <provider_name> <model_name>", nil
			}
			if h.settingsService == nil {
				return "Settings service is not available.", nil
			}
			before, _ := h.getBotSettings(cc)
			modelResp, err := h.findModelForSelection(cc, cc.Args)
			if err != nil {
				return "", err
			}
			_, err = h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
				HeartbeatModelID: modelResp.ID,
			})
			if err != nil {
				return "", err
			}
			return formatChangedValue("Heartbeat model", h.resolveModelName(cc, before.HeartbeatModelID), h.resolveModelName(cc, modelResp.ID)), nil
		},
	})
	return g
}

func (h *Handler) resolveProviderName(cc CommandContext, providerID string) string {
	if h.providersService == nil || providerID == "" {
		return providerID
	}
	p, err := h.providersService.Get(cc.Ctx, providerID)
	if err != nil {
		return providerID
	}
	return p.Name
}

func (h *Handler) findModelByProviderAndName(cc CommandContext, providerName, modelName string) (models.GetResponse, error) {
	provider, err := h.providersService.GetByName(cc.Ctx, providerName)
	if err != nil {
		return models.GetResponse{}, fmt.Errorf("provider %q not found", providerName)
	}
	chatModels, err := h.modelsService.ListByProviderIDAndType(cc.Ctx, provider.ID, models.ModelTypeChat)
	if err != nil {
		return models.GetResponse{}, err
	}
	for _, m := range chatModels {
		if strings.EqualFold(m.Name, modelName) || strings.EqualFold(m.ModelID, modelName) {
			return m, nil
		}
	}
	return models.GetResponse{}, fmt.Errorf("model %q not found under provider %q", modelName, providerName)
}

func (h *Handler) findModelForSelection(cc CommandContext, args []string) (models.GetResponse, error) {
	if h.modelsService == nil {
		return models.GetResponse{}, errors.New("model service is not available")
	}
	if len(args) == 0 {
		return models.GetResponse{}, errors.New("model identifier is required")
	}
	if len(args) == 1 {
		return h.findModelByIDOrName(cc, args[0])
	}
	return h.findModelByProviderAndName(cc, args[0], strings.Join(args[1:], " "))
}

func (h *Handler) findModelByIDOrName(cc CommandContext, target string) (models.GetResponse, error) {
	items, err := h.modelsService.ListByType(cc.Ctx, models.ModelTypeChat)
	if err != nil {
		return models.GetResponse{}, err
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return models.GetResponse{}, errors.New("model identifier is required")
	}
	for _, item := range items {
		if strings.EqualFold(item.ModelID, target) {
			return item, nil
		}
	}
	matches := make([]models.GetResponse, 0, 4)
	for _, item := range items {
		if strings.EqualFold(item.Name, target) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return models.GetResponse{}, fmt.Errorf("model %q not found", target)
	case 1:
		return matches[0], nil
	default:
		choices := make([]string, 0, len(matches))
		for _, item := range matches {
			choices = append(choices, fmt.Sprintf("%s/%s", h.resolveProviderName(cc, item.ProviderID), item.ModelID))
		}
		return models.GetResponse{}, fmt.Errorf("model %q is ambiguous; use a model ID or provider-qualified name (%s)", target, strings.Join(choices, ", "))
	}
}

func (h *Handler) filterModelsByProvider(cc CommandContext, items []models.GetResponse, providerName string) []models.GetResponse {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return items
	}
	filtered := make([]models.GetResponse, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(h.resolveProviderName(cc, item.ProviderID), providerName) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func modelMarkers(modelID string, settingsResp settings.Settings) []string {
	var markers []string
	if modelID == settingsResp.ChatModelID {
		markers = append(markers, "chat")
	}
	if modelID == settingsResp.HeartbeatModelID {
		markers = append(markers, "heartbeat")
	}
	return markers
}

func modelSortRank(model models.GetResponse, settingsResp settings.Settings) int {
	switch len(modelMarkers(model.ID, settingsResp)) {
	case 2:
		return 0
	case 1:
		return 1
	default:
		return 2
	}
}
