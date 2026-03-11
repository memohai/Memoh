package command

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildModelGroup() *CommandGroup {
	g := newCommandGroup("model", "Manage bot models")
	g.Register(SubCommand{
		Name:  "list",
		Usage: "list - List all available chat models",
		Handler: func(cc CommandContext) (string, error) {
			items, err := h.modelsService.ListByType(cc.Ctx, models.ModelTypeChat)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "No chat models found.", nil
			}
			records := make([][]kv, 0, len(items))
			for _, item := range items {
				provName := h.resolveProviderName(cc, item.LlmProviderID)
				records = append(records, []kv{
					{"Model", item.Name},
					{"Provider", provName},
					{"Model ID", item.ModelID},
				})
			}
			return formatItems(records), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set",
		Usage:   "set <provider_name> <model_name> - Set the chat model",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 2 {
				return "Usage: /model set <provider_name> <model_name>", nil
			}
			modelResp, err := h.findModelByProviderAndName(cc, cc.Args[0], cc.Args[1])
			if err != nil {
				return "", err
			}
			_, err = h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
				ChatModelID: modelResp.ID,
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Chat model set to %s (%s).", modelResp.Name, cc.Args[0]), nil
		},
	})
	g.Register(SubCommand{
		Name:    "set-heartbeat",
		Usage:   "set-heartbeat <provider_name> <model_name> - Set the heartbeat model",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) < 2 {
				return "Usage: /model set-heartbeat <provider_name> <model_name>", nil
			}
			modelResp, err := h.findModelByProviderAndName(cc, cc.Args[0], cc.Args[1])
			if err != nil {
				return "", err
			}
			_, err = h.settingsService.UpsertBot(cc.Ctx, cc.BotID, settings.UpsertRequest{
				HeartbeatModelID: modelResp.ID,
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Heartbeat model set to %s (%s).", modelResp.Name, cc.Args[0]), nil
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
