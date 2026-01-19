package prompts

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// Schedule represents a scheduled task
type Schedule struct {
	ID          string
	Pattern     string
	Name        string
	Description string
	Command     string
	MaxCalls    *int // nil means unlimited
}

// ScheduleParams contains parameters for generating the schedule prompt
type ScheduleParams struct {
	Schedule Schedule
	Locale   string // e.g., "en-US", "zh-CN"
	Date     time.Time
}

// scheduleTemplateData holds data for the schedule prompt template
type scheduleTemplateData struct {
	Time        string
	Name        string
	Description string
	ID          string
	MaxCalls    string
	Pattern     string
	Command     string
}

const schedulePromptTemplate = `---
notice: **This is a scheduled task automatically send to you by the system, not the user input**
{{.Time}}
schedule-name: {{.Name}}
schedule-description: {{.Description}}
schedule-id: {{.ID}}
max-calls: {{.MaxCalls}}
cron-pattern: {{.Pattern}}
---

**COMMAND**

{{.Command}}`

var scheduleTmpl *template.Template

func init() {
	var err error
	scheduleTmpl, err = template.New("schedule").Parse(schedulePromptTemplate)
	if err != nil {
		panic(err)
	}
}

// SchedulePrompt generates the prompt for a scheduled task
func SchedulePrompt(params ScheduleParams) string {
	timeStr := Time(TimeParams{
		Date:   params.Date,
		Locale: params.Locale,
	})

	maxCallsStr := "Unlimited"
	if params.Schedule.MaxCalls != nil {
		maxCallsStr = fmt.Sprintf("%d", *params.Schedule.MaxCalls)
	}

	data := scheduleTemplateData{
		Time:        timeStr,
		Name:        params.Schedule.Name,
		Description: params.Schedule.Description,
		ID:          params.Schedule.ID,
		MaxCalls:    maxCallsStr,
		Pattern:     params.Schedule.Pattern,
		Command:     params.Schedule.Command,
	}

	var buf bytes.Buffer
	if err := scheduleTmpl.Execute(&buf, data); err != nil {
		panic(err)
	}

	return strings.TrimSpace(buf.String())
}

