package agent

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed prompts/*.md
var promptsFS embed.FS

var (
	systemTmpl    string
	scheduleTmpl  string
	heartbeatTmpl string
	subagentTmpl  string
)

func init() {
	systemTmpl = mustReadPrompt("prompts/system.md")
	scheduleTmpl = mustReadPrompt("prompts/schedule.md")
	heartbeatTmpl = mustReadPrompt("prompts/heartbeat.md")
	subagentTmpl = mustReadPrompt("prompts/subagent.md")
}

func mustReadPrompt(name string) string {
	data, err := promptsFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("failed to read embedded prompt %s: %v", name, err))
	}
	return string(data)
}

// render replaces all {{key}} placeholders in tmpl with values from vars.
func render(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return strings.TrimSpace(result)
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

	skillsSection := buildSkillsSection(params.Skills, params.EnabledSkills)

	fileSections := ""
	var fileSectionsSb strings.Builder
	for _, f := range params.Files {
		if f.Content == "" {
			continue
		}
		fileSectionsSb.WriteString("\n\n" + formatSystemFile(f))
	}
	fileSections += fileSectionsSb.String()

	return render(systemTmpl, map[string]string{
		"home":          home,
		"basicTools":    strings.Join(basicTools, "\n"),
		"fileSections":  fileSections,
		"skillsSection": skillsSection,
		"inboxSection":  formatInbox(params.Inbox),
	})
}

// SystemPromptParams holds all inputs for system prompt generation.
type SystemPromptParams struct {
	Skills             []SkillEntry
	EnabledSkills      []SkillEntry
	Files              []SystemFile
	Inbox              []InboxItem
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
func GenerateHeartbeatPrompt(interval int, checklist string) string {
	checklistSection := ""
	if strings.TrimSpace(checklist) != "" {
		checklistSection = "\n## HEARTBEAT.md (checklist)\n\n" + strings.TrimSpace(checklist) + "\n"
	}
	return render(heartbeatTmpl, map[string]string{
		"interval":         strconv.Itoa(interval),
		"timeNow":          TimeNow().UTC().Format("2006-01-02T15:04:05Z"),
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

func formatSkillPrompt(skill SkillEntry) string {
	return fmt.Sprintf("**`%s`**\n> %s\n\n%s", skill.Name, skill.Description, skill.Content)
}

func formatSystemFile(file SystemFile) string {
	return fmt.Sprintf("## %s\n\n%s", file.Filename, file.Content)
}

func buildSkillsSection(skills []SkillEntry, enabledSkills []SkillEntry) string {
	if len(skills) == 0 && len(enabledSkills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Skills\n")
	sb.WriteString(strconv.Itoa(len(skills)))
	sb.WriteString(" skills available via `use_skill`:\n")
	for _, s := range skills {
		sb.WriteString("- " + s.Name + ": " + s.Description + "\n")
	}
	for _, s := range enabledSkills {
		sb.WriteString("\n---\n\n")
		sb.WriteString(formatSkillPrompt(s))
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatInbox(items []InboxItem) string {
	if len(items) == 0 {
		return ""
	}

	formatted := make([]map[string]any, len(items))
	for i, item := range items {
		formatted[i] = map[string]any{
			"id":        item.ID,
			"source":    item.Source,
			"header":    item.Header,
			"content":   item.Content,
			"createdAt": item.CreatedAt,
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Inbox (%d unread)\n\n", len(items))
	sb.WriteString("These are messages from other channels — NOT from the current conversation. Use `send` or `react` if you want to respond to any of them.\n\n")
	sb.WriteString("<inbox>\n")
	sb.Write(mustMarshal(formatted))
	sb.WriteString("\n</inbox>\n\n")
	sb.WriteString("Use `search_inbox` to find older messages by keyword.")
	return sb.String()
}
