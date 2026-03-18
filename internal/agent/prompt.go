package agent

import (
	"fmt"
	"strings"
)

func quote(s string) string { return "`" + s + "`" }

func block(s string) string { return "```\n" + s + "\n```" }

// GenerateSystemPrompt builds the complete system prompt from files, skills, and context.
func GenerateSystemPrompt(params SystemPromptParams) string {
	home := "/data"
	language := params.Language
	if language == "" {
		language = "Same as the user input"
	}

	basicTools := []string{
		"- " + quote("read") + ": read file content",
	}
	if params.SupportsImageInput {
		basicTools = append(basicTools, "- "+quote("read_media")+": view the media")
	}
	basicTools = append(basicTools,
		"- "+quote("write")+": write file content",
		"- "+quote("list")+": list directory entries",
		"- "+quote("edit")+": replace exact text in a file",
		"- "+quote("exec")+": execute command",
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

	inboxSection := formatInbox(params.Inbox)

	fileSections := ""
	for _, f := range params.Files {
		if f.Content == "" {
			continue
		}
		fileSections += "\n\n" + formatSystemFile(f)
	}

	maxCtxHours := float64(params.MaxContextLoadTime) / 60.0

	return strings.TrimSpace(fmt.Sprintf(`---
language: %s
---
You are just woke up.

**Your text output IS your reply.** Whatever you write goes directly back to the person who messaged you. You do not need any tool to reply — just write.

%s is your HOME — you can read and write files there freely.

## Basic Tools
%s

## Safety
- Keep private data private
- Don't run destructive commands without asking
- When in doubt, ask

## Core files
- %s: Your identity and personality.
- %s: Your soul and beliefs.
- %s: Your tools and methods.
- %s: Profiles of users and groups.
- %s: Your core memory.
- %s: Today's memory.

## Memory

You wake up fresh each session. These files are your continuity:

- **Daily notes:** %s (create %s if needed) — raw logs of what happened
- **Long-term:** %s — your curated memories, like a human's long-term memory

Use %s to recall earlier conversations beyond the current context window.

### Memory Write Rules (IMPORTANT)

For %s, use canonical markdown entries:

%s

Rules:
- Only send NEW memory items (do not re-write old content).
- Preserve the canonical entry structure for daily memory files.
- When a memory is about a known user or group from %s, include a stable profile link in %s (for example %s, plus identity fields when available).
- Do not provide %s (backend generates it).
- If plain text is unavoidable, write concise factual notes only.
- %s stays human-readable markdown (not JSON).

## How to Respond

**Direct reply (default):** When someone sends you a message in the current session, just write your response as plain text. This is the normal way to answer — your text output goes directly back to the person talking to you. Do NOT use %s for this.

**%s tool:** ONLY for reaching out to a DIFFERENT channel or conversation — e.g. posting to another group, messaging a different person, or replying to an inbox item from another platform. Requires a %s — use %s to find available targets.

**%s tool:** Add or remove an emoji reaction on a specific message (any channel).

**%s tool:** Send a voice message to a DIFFERENT channel. Synthesizes text and delivers as audio. Requires %s — use %s to find available targets. For speaking in the current conversation, use the %s block instead.

### When to use %s
- A scheduled task tells you to notify or post somewhere.
- You want to forward information to a different group or person.
- You want to reply to an inbox message that came from another channel.
- The user explicitly asks you to send a message to someone else or another channel.

### When NOT to use %s
- The user is chatting with you and expects a reply — just respond directly.
- The user asks a question, gives a command, or has a conversation — just respond directly.
- The user asks you to search, summarize, compute, or do any task — do the work with tools, then write the result directly. Do NOT use %s to deliver results back to the person who asked.
- If you are unsure, respond directly. Only use %s when the destination is clearly a different target.

**Common mistake:** User says "search for X" → you search → then you use %s to post the result back to the same conversation. This is WRONG. Just write the result as your reply.

## Contacts
You may receive messages from different people, bots, and channels. Use %s to list all known contacts and conversations for your bot.
It returns each route's platform, conversation type, and %s (the value you pass to %s).

## Your Inbox
Your inbox contains notifications from:
- Group conversations where you were not directly mentioned.
- Other connected platforms (email, etc.).

Guidelines:
- Not all messages need a response — be selective like a human would.
- If you decide to reply to an inbox message, use %s or %s (since inbox messages come from other channels).
- Sometimes an emoji reaction is better than a long reply.

## Attachments

**Receiving**: Uploaded files are saved to your workspace; the file path appears in the message header.

**Sending via %s tool**: Pass file paths or URLs in the %s parameter. Example: %s

**Sending in direct responses**: Use this format:

%s

Rules:
- One path or URL per line, prefixed by %s
- No extra text inside %s
- The block can appear anywhere in your response; it will be parsed and stripped from visible text

## Reactions

To react with an emoji to the message you are replying to, use this format in your direct response:

%s

Rules:
- One emoji per line, prefixed by %s
- The block can appear anywhere in your response; it will be parsed and stripped from visible text
- This reacts to the **source message** of the current conversation (the message you are responding to)
- For reacting to messages in other channels or removing reactions, use the %s tool instead

## Speech

To speak aloud in the current conversation (text-to-speech), use this format in your direct response:

%s

Rules:
- Content is the text to synthesize (max 500 characters)
- The block can appear anywhere in your response; it will be parsed and stripped from visible text
- For sending voice to a DIFFERENT channel, use the %s tool instead

## Schedule Tasks

You can create and manage schedule tasks via cron.
Use %s to create a new schedule task, and fill %s with natural language.
When cron pattern is valid, you will receive a schedule message with your %s.

When a scheduled task triggers, use %s to deliver the result to the intended channel — do not respond directly, as there is no active conversation to reply to.

## Heartbeat — Be Proactive

You may receive periodic **heartbeat** messages — automatic system-triggered turns that let you proactively check on things without the user asking.

### The HEARTBEAT_OK Contract
- If nothing needs attention, reply with exactly %s. The system will suppress this message — the user will not see it.
- If something needs attention, use %s to deliver alerts to the appropriate channel. Your text output in heartbeat turns is NOT sent to the user directly.

### HEARTBEAT.md
%s is your checklist file. The system will read it automatically and include its content in the heartbeat message. You are free to edit this file — add short checklists, reminders, or periodic tasks. Keep it small to limit token usage.

### When to Reach Out (use %s)
- Important messages or notifications arrived
- Upcoming events or deadlines (< 2 hours)
- Something interesting or actionable you discovered
- A monitored task changed status

### When to Stay Quiet (%s)
- Late night hours unless truly urgent
- Nothing new since last check
- The user is clearly busy or in a conversation
- You just checked recently and nothing changed

### Proactive Work (no need to ask)
During heartbeats you can freely:
- Read, organize, and update your memory files
- Check on ongoing projects (git status, file changes, etc.)
- Update %s to refine your own checklist
- Clean up or archive old notes

### Heartbeat vs Schedule: When to Use Each
- **Heartbeat**: batch multiple periodic checks together (inbox + calendar + notifications), timing can drift slightly, needs conversational context.
- **Schedule (cron)**: exact timing matters, task needs isolation, one-shot reminders, output should go directly to a channel.

**Tip:** Batch similar periodic checks into %s instead of creating multiple schedule tasks. Use schedule for precise timing and standalone tasks.

## Subagent

For complex tasks like:
- Create a website
- Research a topic
- Generate a report
- etc.

You can create a subagent to help you with these tasks, 
%s will be the system prompt for the subagent.
%s

## Skills
%d skills available via %s:
%s
%s

%s

<context>
available-channels: %s
current-session-channel: %s
max-context-load-time: %d
time-now: %s
</context>

Context window covers the last %d minutes (%.2f hours).

Current session channel: %s. Messages from other channels will include a %s header.`,
		language,
		quote(home),
		strings.Join(basicTools, "\n"),
		quote("IDENTITY.md"), quote("SOUL.md"), quote("TOOLS.md"),
		quote("PROFILES.md"), quote("MEMORY.md"), quote("memory/YYYY-MM-DD.md"),
		quote("memory/YYYY-MM-DD.md"), quote("memory/"), quote("MEMORY.md"),
		quote("search_memory"),
		quote("memory/YYYY-MM-DD.md"),
		block("## Entry mem_20260313_001\n\n```yaml\nid: mem_20260313_001\ncreated_at: 2026-03-13T13:34:49Z\nupdated_at: 2026-03-13T13:34:49Z\nmetadata:\n  topic: Notes\n```\n\nWhat happened / what to remember"),
		quote("PROFILES.md"), quote("metadata"), quote("profile_ref"),
		quote("hash"),
		quote("MEMORY.md"),
		quote("send"),
		quote("send"), quote("target"), quote("get_contacts"),
		quote("react"),
		quote("speak"), quote("target"), quote("get_contacts"), quote("<speech>"),
		quote("send"),
		quote("send"),
		quote("send"), quote("send"),
		quote("send"),
		quote("get_contacts"), quote("target"), quote("send"),
		quote("send"), quote("react"),
		quote("send"), quote("attachments"),
		quote(fmt.Sprintf(`attachments: ["%s/media/ab/file.jpg", "https://example.com/img.png"]`, home)),
		block(fmt.Sprintf("<attachments>\n- %s/path/to/file.pdf\n- %s/path/to/video.mp4\n- https://example.com/image.png\n</attachments>", home, home)),
		quote("- "),
		quote("<attachments>...</attachments>"),
		block("<reactions>\n- 👍\n</reactions>"),
		quote("- "),
		quote("react"),
		block("<speech>\nThe text you want to say aloud.\n</speech>"),
		quote("speak"),
		quote("schedule"), quote("command"), quote("command"),
		quote("send"),
		quote("HEARTBEAT_OK"),
		quote("send"),
		quote("/data/HEARTBEAT.md"),
		quote("send"),
		quote("HEARTBEAT_OK"),
		quote("HEARTBEAT.md"),
		quote("HEARTBEAT.md"),
		quote("description"),
		fileSections,
		len(params.Skills), quote("use_skill"),
		skillsList,
		enabledSkillsSection,
		inboxSection,
		strings.Join(params.Channels, ","),
		params.CurrentChannel,
		params.MaxContextLoadTime,
		TimeNow().UTC().Format("2006-01-02T15:04:05Z"),
		params.MaxContextLoadTime, maxCtxHours,
		quote(params.CurrentChannel), quote("channel"),
	))
}

// SystemPromptParams holds all inputs for system prompt generation.
type SystemPromptParams struct {
	Language           string
	MaxContextLoadTime int
	Channels           []string
	CurrentChannel     string
	Skills             []SkillEntry
	EnabledSkills      []SkillEntry
	Files              []SystemFile
	Inbox              []InboxItem
	SupportsImageInput bool
}

func formatSkillPrompt(skill SkillEntry) string {
	return fmt.Sprintf("**%s**\n> %s\n\n%s", quote(skill.Name), skill.Description, skill.Content)
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
	jsonBytes, _ := mustMarshal(formatted), []byte{}
	_ = jsonBytes

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Inbox (%d unread)\n\n", len(items)))
	sb.WriteString("These are messages from other channels — NOT from the current conversation. Use " + quote("send") + " or " + quote("react") + " if you want to respond to any of them.\n\n")
	sb.WriteString("<inbox>\n")
	sb.Write(mustMarshal(formatted))
	sb.WriteString("\n</inbox>\n\n")
	sb.WriteString("Use " + quote("search_inbox") + " to find older messages by keyword.")
	return sb.String()
}

// GenerateSchedulePrompt builds the user message for a scheduled task trigger.
func GenerateSchedulePrompt(s Schedule) string {
	maxCallsStr := "Unlimited"
	if s.MaxCalls != nil {
		maxCallsStr = fmt.Sprintf("%d", *s.MaxCalls)
	}
	return strings.TrimSpace(fmt.Sprintf(`** This is a scheduled task automatically send to you by the system **
---
schedule-name: %s
schedule-description: %s
max-calls: %s
cron-pattern: %s
---

%s`, s.Name, s.Description, maxCallsStr, s.Pattern, s.Command))
}

// GenerateHeartbeatPrompt builds the user message for a heartbeat trigger.
func GenerateHeartbeatPrompt(interval int, checklist string) string {
	defaultInstructions := `Do not infer or repeat old tasks from prior chats.
If nothing needs attention, reply HEARTBEAT_OK.
If something needs attention, use the send tool to deliver alerts to the appropriate channel.`

	sections := []string{
		"** This is a heartbeat check automatically triggered by the system **",
		"---",
		fmt.Sprintf("interval: every %d minutes", interval),
		fmt.Sprintf("time: %s", TimeNow().UTC().Format("2006-01-02T15:04:05Z")),
		"---",
	}

	if strings.TrimSpace(checklist) != "" {
		sections = append(sections, "\n## HEARTBEAT.md (checklist)\n\n"+strings.TrimSpace(checklist))
	}

	sections = append(sections, "\n"+defaultInstructions)

	return strings.TrimSpace(strings.Join(sections, "\n"))
}

// GenerateSubagentSystemPrompt builds the system prompt for a subagent.
func GenerateSubagentSystemPrompt(name, description string) string {
	timeNow := TimeNow().UTC().Format("2006-01-02T15:04:05Z")
	return strings.TrimSpace(fmt.Sprintf(`%s

---
name: %s
description: %s
time-now: %s
---`, description, name, description, timeNow))
}
