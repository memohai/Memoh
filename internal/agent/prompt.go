package agent

import (
	"embed"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

//go:embed prompts/*.md
var promptsFS embed.FS

var (
	systemChatTmpl      string
	systemHeartbeatTmpl string
	systemScheduleTmpl  string
	scheduleTmpl        string
	heartbeatTmpl       string
	subagentTmpl        string

	includes map[string]string
)

var includeRe = regexp.MustCompile(`\{\{include:(\w+)\}\}`)

func init() {
	systemChatTmpl = mustReadPrompt("prompts/system_chat.md")
	systemHeartbeatTmpl = mustReadPrompt("prompts/system_heartbeat.md")
	systemScheduleTmpl = mustReadPrompt("prompts/system_schedule.md")
	scheduleTmpl = mustReadPrompt("prompts/schedule.md")
	heartbeatTmpl = mustReadPrompt("prompts/heartbeat.md")
	subagentTmpl = mustReadPrompt("prompts/subagent.md")

	includes = map[string]string{
		"_memory":        mustReadPrompt("prompts/_memory.md"),
		"_tools":         mustReadPrompt("prompts/_tools.md"),
		"_contacts":      mustReadPrompt("prompts/_contacts.md"),
		"_schedule_task": mustReadPrompt("prompts/_schedule_task.md"),
		"_subagent":      mustReadPrompt("prompts/_subagent.md"),
	}

	systemChatTmpl = resolveIncludes(systemChatTmpl)
	systemHeartbeatTmpl = resolveIncludes(systemHeartbeatTmpl)
	systemScheduleTmpl = resolveIncludes(systemScheduleTmpl)
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

func selectSystemTemplate(sessionType string) string {
	switch sessionType {
	case "heartbeat":
		return systemHeartbeatTmpl
	case "schedule":
		return systemScheduleTmpl
	default:
		return systemChatTmpl
	}
}

// GenerateSystemPrompt builds the complete system prompt from files, skills, and context.
func GenerateSystemPrompt(params SystemPromptParams) string {
	home := "/data"

	basicTools := []string{
		"- `read`: read file content",
	}
	if params.SupportsImageInput {
		basicTools = append(basicTools, "- `read_media`: view the media")
	}
	basicTools = append(basicTools,
		"- `write`: write file content",
		"- `list`: list directory entries",
		"- `edit`: replace exact text in a file",
		"- `exec`: execute command",
	)

	skillsSection := buildSkillsSection(params.Skills)

	fileSections := ""
	var fileSectionsSb strings.Builder
	for _, f := range params.Files {
		if f.Content == "" {
			continue
		}
		fileSectionsSb.WriteString("\n\n" + formatSystemFile(f))
	}
	fileSections += fileSectionsSb.String()

	tmpl := selectSystemTemplate(params.SessionType)

	return render(tmpl, map[string]string{
		"home":          home,
		"basicTools":    strings.Join(basicTools, "\n"),
		"skillsSection": skillsSection,
		"fileSections":  fileSections,
	})
}

// SystemPromptParams holds all inputs for system prompt generation.
type SystemPromptParams struct {
	SessionType        string
	Skills             []SkillEntry
	Files              []SystemFile
	SupportsImageInput bool
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
func GenerateHeartbeatPrompt(interval int, checklist string, lastHeartbeatAt string) string {
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
		"timeNow":          TimeNow().UTC().Format("2006-01-02T15:04:05Z"),
		"lastHeartbeat":    lastHB,
		"checklistSection": checklistSection,
	})
}

// GenerateSubagentSystemPrompt builds the system prompt for a subagent.
func GenerateSubagentSystemPrompt(name, description string) string {
	return render(subagentTmpl, map[string]string{
		"name":        name,
		"description": description,
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
	sb.WriteString("## Skills\n")
	sb.WriteString(strconv.Itoa(len(sorted)))
	sb.WriteString(" skills available via `use_skill`:\n")
	for _, s := range sorted {
		sb.WriteString("- " + s.Name + ": " + s.Description + "\n")
	}
	return sb.String()
}

func formatSystemFile(file SystemFile) string {
	return fmt.Sprintf("## %s\n\n%s", file.Filename, file.Content)
}
