package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/settings"
)

func (h *Handler) buildSettingsGroup() *CommandGroup {
	g := newCommandGroup("settings", "View and update bot settings")
	g.DefaultAction = "get"
	g.Register(SubCommand{
		Name:  "get",
		Usage: "get - View current settings",
		Handler: func(cc CommandContext) (string, error) {
			s, err := h.settingsService.GetBot(cc.Ctx, cc.BotID)
			if err != nil {
				return "", err
			}
			return formatKV([]kv{
				{"Language", s.Language},
				{"Allow Guest", boolStr(s.AllowGuest)},
				{"Max Context Load Time", fmt.Sprintf("%d min", s.MaxContextLoadTime)},
				{"Max Context Tokens", strconv.Itoa(s.MaxContextTokens)},
				{"Max Inbox Items", strconv.Itoa(s.MaxInboxItems)},
				{"Reasoning Enabled", boolStr(s.ReasoningEnabled)},
				{"Reasoning Effort", s.ReasoningEffort},
				{"Heartbeat Enabled", boolStr(s.HeartbeatEnabled)},
				{"Heartbeat Interval", fmt.Sprintf("%d min", s.HeartbeatInterval)},
				{"Chat Model", h.resolveModelName(cc, s.ChatModelID)},
				{"Heartbeat Model", h.resolveModelName(cc, s.HeartbeatModelID)},
				{"Search Provider", h.resolveSearchProviderName(cc, s.SearchProviderID)},
				{"Memory Provider", h.resolveMemoryProviderName(cc, s.MemoryProviderID)},
				{"Browser Context", h.resolveBrowserContextName(cc, s.BrowserContextID)},
			}), nil
		},
	})
	g.Register(SubCommand{
		Name:    "update",
		Usage:   "update [--language L] [--allow_guest true|false] ... - Update settings",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if len(cc.Args) == 0 {
				return settingsUpdateUsage(), nil
			}
			req := settings.UpsertRequest{}
			args := cc.Args
			for i := 0; i < len(args); i++ {
				if i+1 >= len(args) {
					return fmt.Sprintf("Missing value for %s.\n\n%s", args[i], settingsUpdateUsage()), nil
				}
				switch args[i] {
				case "--language":
					i++
					req.Language = args[i]
				case "--allow_guest":
					i++
					v := strings.ToLower(args[i]) == "true"
					req.AllowGuest = &v
				case "--reasoning_enabled":
					i++
					v := strings.ToLower(args[i]) == "true"
					req.ReasoningEnabled = &v
				case "--reasoning_effort":
					i++
					req.ReasoningEffort = &args[i]
				case "--heartbeat_enabled":
					i++
					v := strings.ToLower(args[i]) == "true"
					req.HeartbeatEnabled = &v
				case "--heartbeat_interval":
					i++
					val, err := strconv.Atoi(args[i])
					if err != nil {
						return fmt.Sprintf("Invalid heartbeat_interval: %s", args[i]), nil
					}
					req.HeartbeatInterval = &val
				case "--max_context_load_time":
					i++
					val, err := strconv.Atoi(args[i])
					if err != nil {
						return fmt.Sprintf("Invalid max_context_load_time: %s", args[i]), nil
					}
					req.MaxContextLoadTime = &val
				case "--max_context_tokens":
					i++
					val, err := strconv.Atoi(args[i])
					if err != nil {
						return fmt.Sprintf("Invalid max_context_tokens: %s", args[i]), nil
					}
					req.MaxContextTokens = &val
				case "--chat_model_id":
					i++
					req.ChatModelID = args[i]
				case "--heartbeat_model_id":
					i++
					req.HeartbeatModelID = args[i]
				default:
					return fmt.Sprintf("Unknown option: %s\n\n%s", args[i], settingsUpdateUsage()), nil
				}
			}
			_, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, req)
			if err != nil {
				return "", err
			}
			return "Settings updated.", nil
		},
	})
	return g
}

func settingsUpdateUsage() string {
	return "Usage: /settings update [options]\n\n" +
		"Options:\n" +
		"- --language <value>\n" +
		"- --allow_guest <true|false>\n" +
		"- --reasoning_enabled <true|false>\n" +
		"- --reasoning_effort <low|medium|high>\n" +
		"- --heartbeat_enabled <true|false>\n" +
		"- --heartbeat_interval <minutes>\n" +
		"- --max_context_load_time <minutes>\n" +
		"- --max_context_tokens <count>\n" +
		"- --chat_model_id <id>\n" +
		"- --heartbeat_model_id <id>"
}

// resolveModelName resolves a model UUID to "model_name (provider_name)".
func (h *Handler) resolveModelName(cc CommandContext, modelID string) string {
	if modelID == "" {
		return "(none)"
	}
	if h.modelsService == nil {
		return modelID
	}
	m, err := h.modelsService.GetByID(cc.Ctx, modelID)
	if err != nil {
		return modelID
	}
	provName := ""
	if h.providersService != nil {
		p, err := h.providersService.Get(cc.Ctx, m.LlmProviderID)
		if err == nil {
			provName = p.Name
		}
	}
	if provName != "" {
		return fmt.Sprintf("%s (%s)", m.Name, provName)
	}
	return m.Name
}

// resolveSearchProviderName resolves a search provider UUID to its name.
func (h *Handler) resolveSearchProviderName(cc CommandContext, id string) string {
	if id == "" {
		return "(none)"
	}
	if h.searchProvService == nil {
		return id
	}
	p, err := h.searchProvService.Get(cc.Ctx, id)
	if err != nil {
		return id
	}
	return p.Name
}

// resolveMemoryProviderName resolves a memory provider UUID to its name.
func (h *Handler) resolveMemoryProviderName(cc CommandContext, id string) string {
	if id == "" {
		return "(none)"
	}
	if h.memProvService == nil {
		return id
	}
	p, err := h.memProvService.Get(cc.Ctx, id)
	if err != nil {
		return id
	}
	return p.Name
}

// resolveBrowserContextName resolves a browser context UUID to its name.
func (h *Handler) resolveBrowserContextName(cc CommandContext, id string) string {
	if id == "" {
		return "(none)"
	}
	if h.browserCtxService == nil {
		return id
	}
	p, err := h.browserCtxService.GetByID(cc.Ctx, id)
	if err != nil {
		return id
	}
	return p.Name
}
