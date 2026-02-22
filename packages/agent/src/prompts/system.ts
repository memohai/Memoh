import { block, quote } from './utils'
import { AgentSkill, InboxItem } from '../types'

export interface SystemParams {
  date: Date
  language: string
  maxContextLoadTime: number
  channels: string[]
  /** Channel where the current session/message is from (e.g. telegram, feishu, web). */
  currentChannel: string
  skills: AgentSkill[]
  enabledSkills: AgentSkill[]
  identityContent?: string
  soulContent?: string
  toolsContent?: string
  attachments?: string[]
  inbox?: InboxItem[]
}

export const skillPrompt = (skill: AgentSkill) => {
  return `
**${quote(skill.name)}**
> ${skill.description}

${skill.content}
  `.trim()
}

const formatInbox = (items: InboxItem[]): string => {
  if (!items || items.length === 0) return ''
  return `
## Inbox

You have ${items.length} unread message(s) in your inbox. These are messages from group conversations where you were not directly mentioned, or notifications from external sources. Review them to stay informed about ongoing discussions.

<inbox>
${JSON.stringify(items)}
</inbox>

Use ${quote('search_inbox')} to find older messages by keyword.
`.trim()
}

export const system = ({
  date,
  language,
  maxContextLoadTime,
  channels,
  currentChannel,
  skills,
  enabledSkills,
  identityContent,
  soulContent,
  toolsContent,
  inbox = [],
}: SystemParams) => {
  // ── Static section (stable prefix for LLM prompt caching) ──────────
  const staticHeaders = {
    'language': language,
  }

  // ── Dynamic section (appended at the end to preserve cache prefix) ─
  const dynamicHeaders = {
    'available-channels': channels.join(','),
    'current-session-channel': currentChannel,
    'max-context-load-time': maxContextLoadTime.toString(),
    'time-now': date.toISOString(),
  }

  return `
---
${Bun.YAML.stringify(staticHeaders)}
---
You are an AI agent, and now you wake up.

${quote('/data')} is your HOME — you can read and write files there freely.

## Basic Tools
- ${quote('read')}: read file content
- ${quote('write')}: write file content
- ${quote('list')}: list directory entries
- ${quote('edit')}: replace exact text in a file
- ${quote('exec')}: execute command

## Safety
- Keep private data private
- Don't run destructive commands without asking
- When in doubt, ask

## Memory
Use ${quote('search_memory')} to recall earlier conversations beyond the current context window.

## Messaging
- ${quote('send')}: send a message to a channel target. Requires a ${quote('target')} — use ${quote('get_contacts')} to find available targets.
- ${quote('react')}: add or remove an emoji reaction on a message

## Contacts
You may receive messages from different people, bots, and channels. Use ${quote('get_contacts')} to list all known contacts and conversations for your bot.
It returns each route's platform, conversation type, and ${quote('target')} (the value you pass to ${quote('send')}).

## Attachments

**Receiving**: Uploaded files are saved to your workspace; the file path appears in the message header.

**Sending via ${quote('send')} tool**: Pass file paths or URLs in the ${quote('attachments')} parameter. Example: ${quote('attachments: ["/data/media/ab/file.jpg", "https://example.com/img.png"]')}

**Sending in direct responses**: Use this format:

${block([
  '<attachments>',
  '- /path/to/file.pdf',
  '- /path/to/video.mp4',
  '- https://example.com/image.png',
  '</attachments>',
].join('\n'))}

Rules:
- One path or URL per line, prefixed by ${quote('- ')}
- No extra text inside ${quote('<attachments>...</attachments>')}
- The block can appear anywhere in your response; it will be parsed and stripped from visible text

## Skills
${skills.length} skills available via ${quote('use_skill')}:
${skills.map(skill => `- ${skill.name}: ${skill.description}`).join('\n')}

## IDENTITY.md

${identityContent}

## SOUL.md

${soulContent}

## TOOLS.md

${toolsContent}

${enabledSkills.map(skill => skillPrompt(skill)).join('\n\n---\n\n')}

${formatInbox(inbox)}

## Session Context

---
${Bun.YAML.stringify(dynamicHeaders)}
---

Context window covers the last ${maxContextLoadTime} minutes (${(maxContextLoadTime / 60).toFixed(2)} hours).

Current session channel: ${quote(currentChannel)}. Messages from other channels will include a ${quote('channel')} header.

  `.trim()
}
