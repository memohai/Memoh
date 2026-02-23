import type { FSClient } from '../utils/fs'

export interface HeartbeatParams {
  interval: number
  date: Date
  fs: FSClient
}

const defaultInstructions = `Do not infer or repeat old tasks from prior chats.
If nothing needs attention, reply HEARTBEAT_OK.
If something needs attention, use the send tool to deliver alerts to the appropriate channel.`

export const heartbeat = async (params: HeartbeatParams) => {
  let checklist = ''
  try {
    checklist = await params.fs.readText('/data/HEARTBEAT.md')
  } catch {
    // HEARTBEAT.md does not exist — not an error
  }

  const sections: string[] = [
    '** This is a heartbeat check automatically triggered by the system **',
    '---',
    `interval: every ${params.interval} minutes`,
    `time: ${params.date.toISOString()}`,
    '---',
  ]

  if (checklist.trim()) {
    sections.push(`\n## HEARTBEAT.md (checklist)\n\n${checklist.trim()}`)
  }

  sections.push(`\n${defaultInstructions}`)

  return sections.join('\n').trim()
}
