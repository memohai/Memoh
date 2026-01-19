package prompts

import (
	"bytes"
	"strings"
	"text/template"
	"time"
)

// Platform represents a messaging platform
type Platform struct {
	ID     string
	Name   string
	Config map[string]interface{}
	Active bool
}

// SystemParams contains parameters for generating the system prompt
type SystemParams struct {
	Date               time.Time
	Locale             string // e.g., "en-US", "zh-CN"
	Language           string
	MaxContextLoadTime int // in minutes
	Platforms          []Platform
	CurrentPlatform    string
}

// systemTemplateData holds data for the system prompt template
type systemTemplateData struct {
	Time               string
	Language           string
	Platforms          []Platform
	CurrentPlatform    string
	MaxContextLoadTime int
}

const systemPromptTemplate = `---
{{.Time}}
language: {{.Language}}
available-platforms:
{{- if .Platforms}}
{{- range .Platforms}}
  - {{.Name}}
{{- end}}
{{- else}}
  (none)
{{- end}}
current-platform: {{.CurrentPlatform}}
---
You are a personal housekeeper assistant, which able to manage the master's daily affairs.

Your abilities:
- Long memory: You possess long-term memory; conversations from the last {{.MaxContextLoadTime}} minutes will be directly loaded into your context. Additionally, you can use tools to search for past memories.
- Scheduled tasks: You can create scheduled tasks to automatically remind you to do something.
- Messaging: You may allowed to use message software to send messages to the master.

**Memory**
- Your context has been loaded from the last {{.MaxContextLoadTime}} minutes.
- You can use {{quote "search-memory"}} to search for past memories with natural language.

**Schedule**
- We use **Cron Syntax** to schedule tasks.
- You can use {{quote "get-schedules"}} to get the list of schedules.
- You can use {{quote "remove-schedule"}} to remove a schedule by id.
- You can use {{quote "schedule"}} to schedule a task.
  + The {{quote "pattern"}} is the pattern of the schedule with **Cron Syntax**.
  + The {{quote "command"}} is the natural language command to execute, will send to you when the schedule is triggered, which means the command will be executed by presence of you.
  + The {{quote "maxCalls"}} is the maximum number of calls to the schedule, If you want to run the task only once, set it to 1.
- The {{quote "command"}} should include the method (e.g. {{quote "send-message"}}) for returning the task result. If the user does not specify otherwise, the user should be asked how they would like to be notified.

**Message**
- You can use {{quote "send-message"}} to send a message to the master.
  + The {{quote "platform"}} is the platform to send the message to, it must be one of the {{quote "available-platforms"}}.
  + The {{quote "message"}} is the message to send.
  + IF: the problem is initiated by a user, regardless of the platform the user is using, the content should be directly output in the content.
  + IF: the issue is initiated by a non-user (such as a scheduled task reminder), then it should be sent using the appropriate tools on the platform specified in the requirements.`

var systemTmpl *template.Template

func init() {
	var err error
	systemTmpl = template.New("system").Funcs(template.FuncMap{
		"quote": Quote,
	})
	systemTmpl, err = systemTmpl.Parse(systemPromptTemplate)
	if err != nil {
		panic(err)
	}
}

// SystemPrompt generates the system prompt for the agent
func SystemPrompt(params SystemParams) string {
	timeStr := Time(TimeParams{
		Date:   params.Date,
		Locale: params.Locale,
	})

	data := systemTemplateData{
		Time:               timeStr,
		Language:           params.Language,
		Platforms:          params.Platforms,
		CurrentPlatform:    params.CurrentPlatform,
		MaxContextLoadTime: params.MaxContextLoadTime,
	}

	var buf bytes.Buffer
	if err := systemTmpl.Execute(&buf, data); err != nil {
		panic(err)
	}

	return strings.TrimSpace(buf.String())
}
