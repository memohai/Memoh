package agent

import (
	"embed"
	"fmt"
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

	skillsList := ""
	if len(params.Skills) > 0 {
		lines := make([]string, len(params.Skills))
		for i, s := range params.Skills {
			lines[i] = "- " + s.Name + ": " + s.Description
		}
		skillsList = strings.Join(lines, "\n")
	}

	enabledSkillsSection := ""
	for _, s := range params.EnabledSkills {
		enabledSkillsSection += "\n\n---\n\n" + formatSkillPrompt(s)
	}

	fileSections := ""
	for _, f := range params.Files {
		if f.Content == "" {
			continue
		}
		fileSections += "\n\n" + formatSystemFile(f)
	}

	return render(systemTmpl, map[string]string{
		"home":                 home,
		"basicTools":           strings.Join(basicTools, "\n"),
		"fileSections":         fileSections,
		"skillsCount":          fmt.Sprintf("%d", len(params.Skills)),
		"skillsList":           skillsList,
		"enabledSkillsSection": enabledSkillsSection,
		"inboxSection":         formatInbox(params.Inbox),
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
		maxCallsStr = fmt.Sprintf("%d", *s.MaxCalls)
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
		"interval":         fmt.Sprintf("%d", interval),
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
	sb.WriteString(fmt.Sprintf("## Inbox (%d unread)\n\n", len(items)))
	sb.WriteString("These are messages from other channels — NOT from the current conversation. Use `send` or `react` if you want to respond to any of them.\n\n")
	sb.WriteString("<inbox>\n")
	sb.Write(mustMarshal(formatted))
	sb.WriteString("\n</inbox>\n\n")
	sb.WriteString("Use `search_inbox` to find older messages by keyword.")
	return sb.String()
}
