package command

import (
	"errors"
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
		ResultHandler: func(cc CommandContext) (*Result, error) {
			s, err := h.settingsService.GetBot(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			return h.settingsResult(cc, s), nil
		},
	})
	g.Register(SubCommand{
		Name:    "update",
		Usage:   "update [--language L] [--acl_default_effect allow|deny] ... - Update settings",
		IsWrite: true,
		ResultHandler: func(cc CommandContext) (*Result, error) {
			if len(cc.Args) == 0 {
				return &Result{Text: settingsUpdateUsage()}, nil
			}
			req := settings.UpsertRequest{}
			args := cc.Args
			for i := 0; i < len(args); i++ {
				if i+1 >= len(args) {
					return &Result{Text: fmt.Sprintf("Missing value for %s.\n\n%s", args[i], settingsUpdateUsage())}, nil
				}
				switch args[i] {
				case "--language":
					i++
					req.Language = args[i]
				case "--acl_default_effect":
					i++
					req.AclDefaultEffect = args[i]
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
						return &Result{Text: fmt.Sprintf("Invalid heartbeat_interval: %s", args[i])}, nil
					}
					req.HeartbeatInterval = &val
				case "--chat_model_id":
					i++
					req.ChatModelID = args[i]
				case "--heartbeat_model_id":
					i++
					req.HeartbeatModelID = args[i]
				default:
					return &Result{Text: fmt.Sprintf("Unknown option: %s\n\n%s", args[i], settingsUpdateUsage())}, nil
				}
			}
			if _, err := h.settingsService.UpsertBot(cc.Ctx, cc.BotID, req); err != nil {
				return nil, err
			}
			s, err := h.settingsService.GetBot(cc.Ctx, cc.BotID)
			if err != nil {
				return nil, err
			}
			return h.settingsResult(cc, s), nil
		},
	})
	return g
}

// settingsResult renders the settings card (the same KV detail as before) and,
// for button-capable channels, a set of one-tap controls: inline toggles for the
// heartbeat/ACL (re-dispatch /settings update, which re-renders this card in
// place) and drill-downs to the /reasoning and /model pickers. Reuses
// settingsService.UpsertBot — no backend changes.
func (h *Handler) settingsResult(cc CommandContext, s settings.Settings) *Result {
	reasoning := "off"
	if s.ReasoningEnabled {
		reasoning = strings.TrimSpace(s.ReasoningEffort)
		if reasoning == "" {
			reasoning = "on"
		}
	}
	heartbeat := "off"
	if s.HeartbeatEnabled {
		heartbeat = fmt.Sprintf("on · every %d min", s.HeartbeatInterval)
	}
	// Teach the ACL enum in plain English on this orienting surface.
	aclLine := strings.TrimSpace(s.AclDefaultEffect)
	switch strings.ToLower(aclLine) {
	case "deny":
		aclLine = "deny — tools ask before running"
	case "allow":
		aclLine = "allow — tools run without asking"
	}
	card := formatKVTitled("⚙️ Bot Settings", []kv{
		{"Reasoning", reasoning},
		{"Heartbeat", heartbeat},
		{"ACL default", aclLine},
		{"Chat Model", h.resolveModelName(cc, s.ChatModelID)},
		{"Heartbeat Model", h.resolveModelName(cc, s.HeartbeatModelID)},
		{"Search Provider", h.resolveSearchProviderName(cc, s.SearchProviderID)},
		{"Memory Provider", h.resolveMemoryProviderName(cc, s.MemoryProviderID)},
	})
	aclNext := "deny"
	aclAction := "Ask before tools"
	if strings.EqualFold(strings.TrimSpace(s.AclDefaultEffect), "deny") {
		aclNext = "allow"
		aclAction = "Allow tools"
	}
	heartbeatAction := "Turn heartbeat on"
	if s.HeartbeatEnabled {
		heartbeatAction = "Turn heartbeat off"
	}
	choices := []ListItem{
		{Label: "Reasoning ▸", Action: &ItemAction{Resource: "reasoning", Action: "show"}},
		{Label: "Models ▸", Action: &ItemAction{Resource: "model", Action: "list"}},
		{Label: heartbeatAction, Action: &ItemAction{Resource: "settings", Action: "update", Args: []string{"--heartbeat_enabled", strconv.FormatBool(!s.HeartbeatEnabled)}}},
		{Label: aclAction, Action: &ItemAction{Resource: "settings", Action: "update", Args: []string{"--acl_default_effect", aclNext}}},
		{Label: "Search ▸", Action: &ItemAction{Resource: "search", Action: "list"}},
		{Label: "Memory ▸", Action: &ItemAction{Resource: "memory", Action: "list"}},
	}
	return &Result{
		Text:        card,
		Interactive: &Interactive{Kind: InteractiveChoices, Choices: &ChoicesView{Title: card, Choices: choices}},
	}
}

func settingsUpdateUsage() string {
	return "Usage: /settings update [options]\n\n" +
		"Options:\n" +
		"- --language <value>\n" +
		"- --acl_default_effect <allow|deny>\n" +
		"- --reasoning_enabled <true|false>\n" +
		"- --reasoning_effort <none|low|medium|high|xhigh>\n" +
		"- --heartbeat_enabled <true|false>\n" +
		"- --heartbeat_interval <minutes>\n" +
		"- --chat_model_id <id>\n" +
		"- --heartbeat_model_id <id>"
}

func (h *Handler) getBotSettings(cc CommandContext) (settings.Settings, error) {
	if h.settingsService == nil {
		return settings.Settings{}, errors.New("settings service is not available")
	}
	return h.settingsService.GetBot(cc.Ctx, cc.BotID)
}

// resolveModelName resolves a model UUID to "model_name (provider_name)".
func (h *Handler) resolveModelName(cc CommandContext, modelID string) string {
	if modelID == "" {
		return "(none)"
	}
	if h.modelsService == nil {
		return "(unavailable)"
	}
	m, err := h.modelsService.GetByID(cc.Ctx, modelID)
	if err != nil {
		return "(unavailable)"
	}
	provName := ""
	if h.providersService != nil {
		p, err := h.providersService.Get(cc.Ctx, m.ProviderID)
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
		return "(unavailable)"
	}
	p, err := h.searchProvService.Get(cc.Ctx, id)
	if err != nil {
		return "(unavailable)"
	}
	return p.Name
}

// resolveMemoryProviderName resolves a memory provider UUID to its name.
func (h *Handler) resolveMemoryProviderName(cc CommandContext, id string) string {
	if id == "" {
		return "(none)"
	}
	if h.memProvService == nil {
		return "(unavailable)"
	}
	p, err := h.memProvService.Get(cc.Ctx, id)
	if err != nil {
		return "(unavailable)"
	}
	return p.Name
}
