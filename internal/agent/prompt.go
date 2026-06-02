package agent

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	skillset "github.com/memohai/memoh/internal/skills"
)

//go:embed prompts/*.md
var promptsFS embed.FS

var (
	systemCommonTmpl  string
	modeChatTmpl      string
	modeDiscussTmpl   string
	modeHeartbeatTmpl string
	modeScheduleTmpl  string
	modeSubagentTmpl  string
	scheduleTmpl      string
	heartbeatTmpl     string

	MemoryExtractPrompt string
	MemoryUpdatePrompt  string

	includes map[string]string
)

var includeRe = regexp.MustCompile(`\{\{include:(\w+)\}\}`)

func init() {
	systemCommonTmpl = mustReadPrompt("prompts/system_common.md")
	modeChatTmpl = mustReadPrompt("prompts/mode_chat.md")
	modeDiscussTmpl = mustReadPrompt("prompts/mode_discuss.md")
	modeHeartbeatTmpl = mustReadPrompt("prompts/mode_heartbeat.md")
	modeScheduleTmpl = mustReadPrompt("prompts/mode_schedule.md")
	modeSubagentTmpl = mustReadPrompt("prompts/mode_subagent.md")
	scheduleTmpl = mustReadPrompt("prompts/schedule.md")
	heartbeatTmpl = mustReadPrompt("prompts/heartbeat.md")
	MemoryExtractPrompt = mustReadPrompt("prompts/memory_extract.md")
	MemoryUpdatePrompt = mustReadPrompt("prompts/memory_update.md")

	includes = map[string]string{
		"_memory":        mustReadPrompt("prompts/_memory.md"),
		"_tools":         mustReadPrompt("prompts/_tools.md"),
		"_contacts":      mustReadPrompt("prompts/_contacts.md"),
		"_identities":    mustReadPrompt("prompts/_identities.md"),
		"_schedule_task": mustReadPrompt("prompts/_schedule_task.md"),
		"_subagent":      mustReadPrompt("prompts/_subagent.md"),
	}

	systemCommonTmpl = resolveIncludes(systemCommonTmpl)
	modeChatTmpl = resolveIncludes(modeChatTmpl)
	modeDiscussTmpl = resolveIncludes(modeDiscussTmpl)
	modeHeartbeatTmpl = resolveIncludes(modeHeartbeatTmpl)
	modeScheduleTmpl = resolveIncludes(modeScheduleTmpl)
	modeSubagentTmpl = resolveIncludes(modeSubagentTmpl)
}

func mustReadPrompt(name string) string {
	data, err := promptsFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("failed to read embedded prompt %s: %v", name, err))
	}
	return string(data)
}

// resolveIncludes replaces {{include:_name}} placeholders with the content of the named fragment.
func resolveIncludes(tmpl string) string {
	return includeRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := includeRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		content, ok := includes[sub[1]]
		if !ok {
			return match
		}
		return strings.TrimSpace(content)
	})
}

// render replaces all {{key}} placeholders in tmpl with values from vars.
func render(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return strings.TrimSpace(result)
}

func selectModeTemplate(sessionType string) string {
	switch sessionType {
	case "discuss":
		return modeDiscussTmpl
	case "heartbeat":
		return modeHeartbeatTmpl
	case "schedule":
		return modeScheduleTmpl
	case "subagent":
		return modeSubagentTmpl
	default:
		return modeChatTmpl
	}
}

// GenerateSystemPrompt builds the complete system prompt from files, skills, and context.
func GenerateSystemPrompt(params SystemPromptParams) string {
	home := "/data"
	now := params.Now
	if now.IsZero() {
		now = TimeNow()
	}
	timezoneName := strings.TrimSpace(params.Timezone)
	if timezoneName == "" {
		timezoneName = "UTC"
	}

	readToolDesc := "- `read`: read file content"
	if params.SupportsImageInput {
		readToolDesc += " (also supports images: PNG, JPEG, GIF, WebP)"
	}
	basicTools := []string{readToolDesc}
	basicTools = append(basicTools,
		"- `write`: write file content",
		"- `list`: list directory entries",
		"- `edit`: replace exact text in a file",
		"- `exec`: execute command",
	)

	displayTools := buildDisplayToolsSection(params.DisplayEnabled)
	botInfoSection := buildBotInfoSection(params.Bot)

	skillsSection := buildSkillsSection(params.Skills)

	fileSections := buildFileSections(params.Files)

	tmpl := strings.TrimSpace(systemCommonTmpl + "\n\n" + selectModeTemplate(params.SessionType))

	return render(tmpl, map[string]string{
		"home":                      home,
		"currentTime":               now.Format(time.RFC3339),
		"timezone":                  timezoneName,
		"basicTools":                strings.Join(basicTools, "\n"),
		"displayTools":              displayTools,
		"botInfoSection":            botInfoSection,
		"skillsSection":             skillsSection,
		"platformIdentitiesSection": strings.TrimSpace(params.PlatformIdentitiesSection),
		"mainAgentSections":         buildMainAgentSections(strings.TrimSpace(params.PlatformIdentitiesSection), skillsSection, fileSections),
		"subagentSections":          buildSubagentSections(strings.TrimSpace(params.PlatformIdentitiesSection)),
		"fileSections":              fileSections,
	})
}

// SystemPromptParams holds all inputs for system prompt generation.
type SystemPromptParams struct {
	SessionType               string
	Bot                       BotInfo
	Skills                    []SkillEntry
	Files                     []SystemFile
	Now                       time.Time
	Timezone                  string
	SupportsImageInput        bool
	DisplayEnabled            bool
	PlatformIdentitiesSection string
}

func buildDisplayToolsSection(enabled bool) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(`
## Workspace browser & desktop

This bot has a headed workspace display (Chrome on a virtual desktop). Use GUI tools only when the task needs on-screen interaction:

- **Browser** (browser_observe, browser_action): Web pages in Chrome. Observe before acting; prefer element refs from snapshot over CSS selectors.
- **Computer** (computer_observe, computer_action): Whole-desktop fallback for native dialogs, non-browser apps, or when the browser path fails. Start with computer_observe snapshot to get an accessibility tree with element refs, then drive computer_action with those refs; raw coordinates are a last-resort fallback.
- **browser_remote_session**: Only when running Playwright or other CDP automation inside the workspace is clearly better than the browser tools above.
- **Screenshots**: Both browser_observe and computer_observe save screenshots to a workspace path; they are not attached to the conversation. Read the returned path with the file read tool when you need the image.
`)
}

func buildBotInfoSection(bot BotInfo) string {
	bot.ID = strings.TrimSpace(bot.ID)
	bot.Name = strings.TrimSpace(bot.Name)
	bot.DisplayName = strings.TrimSpace(bot.DisplayName)
	bot.Timezone = strings.TrimSpace(bot.Timezone)
	if bot.ID == "" && bot.Name == "" && bot.DisplayName == "" && bot.Timezone == "" {
		return ""
	}
	raw, err := json.MarshalIndent(bot, "", "  ")
	if err != nil {
		return ""
	}
	return "## Bot\n\nService-provided bot identity. Use `display_name` as your user-facing name when it is present; otherwise use `name`. `name` is the stable slug. Do not invent another name.\n\n```json\n" + string(raw) + "\n```"
}

// GenerateSchedulePrompt builds the user message for a scheduled task trigger.
func GenerateSchedulePrompt(s Schedule) string {
	maxCallsStr := "Unlimited"
	if s.MaxCalls != nil {
		maxCallsStr = strconv.Itoa(*s.MaxCalls)
	}
	return render(scheduleTmpl, map[string]string{
		"name":        s.Name,
		"description": s.Description,
		"maxCalls":    maxCallsStr,
		"pattern":     s.Pattern,
		"command":     s.Command,
	})
}

// GenerateHeartbeatPrompt builds the user message for a heartbeat trigger.
func GenerateHeartbeatPrompt(interval int, checklist string, now time.Time, lastHeartbeatAt string) string {
	checklistSection := ""
	if strings.TrimSpace(checklist) != "" {
		checklistSection = "\n## HEARTBEAT.md (checklist)\n\n" + strings.TrimSpace(checklist) + "\n"
	}
	lastHB := strings.TrimSpace(lastHeartbeatAt)
	if lastHB == "" {
		lastHB = "never (first heartbeat)"
	}
	return render(heartbeatTmpl, map[string]string{
		"interval":         strconv.Itoa(interval),
		"timeNow":          now.Format(time.RFC3339),
		"lastHeartbeat":    lastHB,
		"checklistSection": checklistSection,
	})
}

func buildSkillsSection(skills []SkillEntry) string {
	if len(skills) == 0 {
		return ""
	}
	sorted := make([]SkillEntry, len(skills))
	copy(sorted, skills)
	slices.SortFunc(sorted, func(a, b SkillEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
	var sb strings.Builder
	sb.WriteString("## Skills\n\n")
	sb.WriteString("Memoh-managed skills are stored in `" + skillset.ManagedDir() + "/`. ")
	sb.WriteString("Compatible external skill directories inside the bot container may also be discovered automatically. ")
	sb.WriteString("Each skill is a `SKILL.md` file inside a named subdirectory.\n\n")
	sb.WriteString("Call `use_skill` with the skill name to load its full instructions before following them. ")
	sb.WriteString("Only activate a skill when it is relevant to the current task.\n\n")
	sb.WriteString(strconv.Itoa(len(sorted)))
	sb.WriteString(" skill(s) available:\n")
	for _, s := range sorted {
		sb.WriteString("- **" + s.Name + "**: " + s.Description + "\n")
	}
	return sb.String()
}

func buildFileSections(files []SystemFile) string {
	var sb strings.Builder
	for _, f := range files {
		if f.Content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(formatSystemFile(f))
	}
	return sb.String()
}

func buildMainAgentSections(platformIdentitiesSection, skillsSection, fileSections string) string {
	identitiesSection := render(includes["_identities"], map[string]string{
		"platformIdentitiesSection": platformIdentitiesSection,
	})
	sections := []string{
		includes["_memory"],
		includes["_contacts"],
		identitiesSection,
		includes["_schedule_task"],
		"When a scheduled task triggers, it runs in its own session. Use `send` in the schedule command to deliver results to the intended channel.",
		includes["_subagent"],
		skillsSection,
		fileSections,
	}
	return joinPromptSections(sections...)
}

func buildSubagentSections(platformIdentitiesSection string) string {
	return strings.TrimSpace(render(includes["_identities"], map[string]string{
		"platformIdentitiesSection": platformIdentitiesSection,
	}))
}

func joinPromptSections(sections ...string) string {
	var sb strings.Builder
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(section)
	}
	return sb.String()
}

func formatSystemFile(file SystemFile) string {
	return fmt.Sprintf("## %s\n\n%s", file.Filename, file.Content)
}
