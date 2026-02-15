import chalk from 'chalk'
import { client } from '@memoh/sdk/client'

// ---------------------------------------------------------------------------
// SSE stream types (aligned with frontend useChat.ts)
// ---------------------------------------------------------------------------

interface StreamEvent {
  type?: string
  delta?: string
  toolName?: string
  input?: unknown
  result?: unknown
  error?: string
  message?: string
  [key: string]: unknown
}

// ---------------------------------------------------------------------------
// SSE parsing (directly from frontend useChat.ts)
// ---------------------------------------------------------------------------

/**
 * Read an SSE stream line-by-line, calling onData for each `data:` payload.
 * Handles standard SSE format (events separated by double newlines).
 */
async function readSSEStream(
  body: ReadableStream<Uint8Array>,
  onData: (payload: string) => void,
): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      const chunks = buffer.split('\n\n')
      buffer = chunks.pop() ?? ''

      for (const chunk of chunks) {
        for (const line of chunk.split('\n')) {
          if (!line.startsWith('data:')) continue
          const payload = line.replace(/^data:\s*/, '').trim()
          if (payload && payload !== '[DONE]') onData(payload)
        }
      }
    }

    // Flush remaining buffer
    if (buffer.trim()) {
      for (const line of buffer.split('\n')) {
        const trimmed = line.trim()
        if (!trimmed.startsWith('data:')) continue
        const payload = trimmed.replace(/^data:\s*/, '').trim()
        if (payload && payload !== '[DONE]') onData(payload)
      }
    }
  } finally {
    reader.releaseLock()
  }
}

/**
 * Parse a raw SSE payload string into a StreamEvent.
 * Handles double-encoded JSON and plain text deltas.
 * (directly from frontend useChat.ts)
 */
function parseStreamPayload(payload: string): StreamEvent | null {
  let current: unknown = payload
  for (let i = 0; i < 2; i += 1) {
    if (typeof current !== 'string') break
    const raw = current.trim()
    if (!raw || raw === '[DONE]') return null
    try {
      current = JSON.parse(raw)
    } catch {
      return { type: 'text_delta', delta: raw } as StreamEvent
    }
  }

  if (typeof current === 'string') {
    return { type: 'text_delta', delta: current.trim() } as StreamEvent
  }
  if (current && typeof current === 'object') {
    return current as StreamEvent
  }
  return null
}

// ---------------------------------------------------------------------------
// Tool display configuration
// ---------------------------------------------------------------------------

type ToolDisplayMode = 'inline' | 'expanded'

interface ToolDisplayConfig {
  mode: ToolDisplayMode
  expandParam?: string
  label?: string
}

const TOOL_DISPLAY: Record<string, ToolDisplayConfig> = {
  exec:  { mode: 'expanded', label: 'exec' },
  write: { mode: 'expanded', expandParam: 'content', label: 'write' },
}

const getToolDisplay = (toolName: string): ToolDisplayConfig => {
  return TOOL_DISPLAY[toolName] ?? { mode: 'inline' }
}

// ---------------------------------------------------------------------------
// Tool call formatting helpers
// ---------------------------------------------------------------------------

const BOX_WIDTH = 60

const extractExecCommand = (toolInput: unknown): string => {
  if (!toolInput || typeof toolInput !== 'object') return ''
  const input = toolInput as Record<string, unknown>
  const command = typeof input.command === 'string' ? input.command : ''
  const args = Array.isArray(input.args) ? input.args.map(String) : []
  if (/^(bash|sh|zsh)$/.test(command) && args.length >= 2) {
    const flag = args[0]
    if (flag === '-c' || flag === '-lc') {
      return args.slice(1).join(' ')
    }
  }
  return [command, ...args].filter(Boolean).join(' ')
}

const formatExecCall = (toolInput: unknown) => {
  const cmd = extractExecCommand(toolInput)
  return chalk.dim('  ▶ ') + chalk.white('$ ') + chalk.bold.white(cmd)
}

const extractEditInput = (toolInput: unknown) => {
  if (!toolInput || typeof toolInput !== 'object') {
    return { path: '', oldText: '', newText: '' }
  }
  const input = toolInput as Record<string, unknown>
  const path = typeof input.path === 'string' ? input.path : ''
  const oldText =
    typeof input.old_text === 'string'
      ? input.old_text
      : typeof input.oldText === 'string'
        ? input.oldText
        : ''
  const newText =
    typeof input.new_text === 'string'
      ? input.new_text
      : typeof input.newText === 'string'
        ? input.newText
        : ''
  return { path, oldText, newText }
}

const countLines = (text: string) => (text ? text.split('\n').length : 0)

const pushDetailBlock = (lines: string[], title: string, content: string) => {
  lines.push(chalk.cyan('│ ') + chalk.dim(title))
  const detailLines = content ? content.split('\n') : []
  if (!detailLines.length) {
    lines.push(chalk.cyan('│ ') + chalk.dim('∅'))
    return
  }
  const maxLines = 12
  const shown = detailLines.slice(0, maxLines)
  for (const dl of shown) {
    const truncated = dl.length > BOX_WIDTH - 4 ? dl.slice(0, BOX_WIDTH - 7) + '...' : dl
    lines.push(chalk.cyan('│ ') + chalk.white(truncated))
  }
  if (detailLines.length > maxLines) {
    lines.push(chalk.cyan('│ ') + chalk.dim(`... (${detailLines.length - maxLines} more lines)`))
  }
}

const formatEditCall = (toolInput: unknown) => {
  const { path, oldText, newText } = extractEditInput(toolInput)
  const oldLines = countLines(oldText)
  const newLines = countLines(newText)
  const summary = ` path: ${path || '(unknown)'} · old: ${oldLines} lines · new: ${newLines} lines`

  const topBorder = '┌' + '─'.repeat(BOX_WIDTH - 2) + '┐'
  const botBorder = '└' + '─'.repeat(BOX_WIDTH - 2) + '┘'

  const lines: string[] = []
  lines.push(chalk.cyan(topBorder))
  lines.push(chalk.cyan('│ ') + chalk.bold.white('edit') + chalk.gray(summary))
  lines.push(chalk.cyan('│ ') + chalk.dim('─'.repeat(BOX_WIDTH - 4)))
  pushDetailBlock(lines, 'old_text', oldText)
  lines.push(chalk.cyan('│ ') + chalk.dim('─'.repeat(BOX_WIDTH - 4)))
  pushDetailBlock(lines, 'new_text', newText)
  lines.push(chalk.cyan(botBorder))
  return lines.join('\n')
}

const unwrapToolResult = (result: unknown): Record<string, unknown> | null => {
  if (!result) return null
  const extractFromContentBlocks = (arr: unknown[]): Record<string, unknown> | null => {
    for (const block of arr) {
      if (block && typeof block === 'object') {
        const b = block as Record<string, unknown>
        if (b.type === 'text' && typeof b.text === 'string') {
          try { return JSON.parse(b.text) } catch { /* ignore */ }
        }
      }
    }
    return null
  }
  if (Array.isArray(result)) return extractFromContentBlocks(result)
  if (typeof result === 'object') {
    const obj = result as Record<string, unknown>
    if (Array.isArray(obj.content)) {
      const extracted = extractFromContentBlocks(obj.content)
      if (extracted) return extracted
    }
    return obj
  }
  if (typeof result === 'string') {
    try { return JSON.parse(result) } catch { /* ignore */ }
  }
  return null
}

const formatExecResult = (result: unknown) => {
  const r = unwrapToolResult(result)
  if (!r) return chalk.dim('  ╰─ done')
  const exitCode = typeof r.exit_code === 'number' ? r.exit_code : (r.ok ? 0 : 1)
  const ok = exitCode === 0
  const stdout = typeof r.stdout === 'string' ? r.stdout.trim() : ''
  const stderr = typeof r.stderr === 'string' ? r.stderr.trim() : ''
  const lines: string[] = []
  lines.push(chalk.dim('  ╰─ ') + (ok ? chalk.green(`✓ exit ${exitCode}`) : chalk.red(`✗ exit ${exitCode}`)))
  const output = ok ? stdout : (stderr || stdout)
  if (output) {
    const outputLines = output.split('\n')
    const maxLines = 8
    const shown = outputLines.slice(0, maxLines)
    for (const ol of shown) {
      const truncated = ol.length > 72 ? ol.slice(0, 69) + '...' : ol
      lines.push(chalk.dim('    ') + (ok ? chalk.white(truncated) : chalk.yellow(truncated)))
    }
    if (outputLines.length > maxLines) {
      lines.push(chalk.dim(`    ... (${outputLines.length - maxLines} more lines)`))
    }
  }
  return lines.join('\n')
}

const formatToolCallInline = (toolName: string, toolInput: unknown) => {
  let params = ''
  if (toolInput && typeof toolInput === 'object') {
    const entries = Object.entries(toolInput as Record<string, unknown>)
    params = entries
      .map(([k, v]) => {
        const s = typeof v === 'string' ? v : JSON.stringify(v)
        const short = s.length > 40 ? s.slice(0, 37) + '...' : s
        return `${k}=${short}`
      })
      .join(', ')
  }
  return chalk.dim(`  ◆ ${toolName}`) + (params ? chalk.dim(`(${params})`) : '')
}

const formatToolCallExpanded = (config: ToolDisplayConfig, toolName: string, toolInput: unknown) => {
  const label = config.label ?? toolName
  const inputObj = (toolInput && typeof toolInput === 'object' ? toolInput : {}) as Record<string, unknown>
  const summaryParts: string[] = []
  for (const [k, v] of Object.entries(inputObj)) {
    if (k === config.expandParam) continue
    const s = typeof v === 'string' ? v : JSON.stringify(v)
    summaryParts.push(`${k}: ${s.length > 50 ? s.slice(0, 47) + '...' : s}`)
  }
  const summary = summaryParts.length ? ' ' + summaryParts.join(', ') : ''
  let detail = ''
  if (config.expandParam && config.expandParam in inputObj) {
    const raw = inputObj[config.expandParam]
    if (typeof raw === 'string') detail = raw
    else if (Array.isArray(raw)) detail = raw.join(' ')
    else detail = JSON.stringify(raw, null, 2)
  }
  const topBorder = '┌' + '─'.repeat(BOX_WIDTH - 2) + '┐'
  const botBorder = '└' + '─'.repeat(BOX_WIDTH - 2) + '┘'
  const lines: string[] = []
  lines.push(chalk.cyan(topBorder))
  lines.push(chalk.cyan('│ ') + chalk.bold.white(label) + chalk.gray(summary))
  if (detail) {
    lines.push(chalk.cyan('│ ') + chalk.dim('─'.repeat(BOX_WIDTH - 4)))
    const detailLines = detail.split('\n')
    const maxLines = 20
    const shown = detailLines.slice(0, maxLines)
    for (const dl of shown) {
      const truncated = dl.length > BOX_WIDTH - 4 ? dl.slice(0, BOX_WIDTH - 7) + '...' : dl
      lines.push(chalk.cyan('│ ') + chalk.white(truncated))
    }
    if (detailLines.length > maxLines) {
      lines.push(chalk.cyan('│ ') + chalk.dim(`... (${detailLines.length - maxLines} more lines)`))
    }
  }
  lines.push(chalk.cyan(botBorder))
  return lines.join('\n')
}

const formatToolResult = (toolName: string, result: unknown) => {
  if (toolName === 'exec') return formatExecResult(result)
  const config = getToolDisplay(toolName)
  if (config.mode === 'expanded' || toolName === 'edit') {
    const r = unwrapToolResult(result)
    if (r) {
      if ('ok' in r) {
        return chalk.dim('  ╰─ ') + (r.ok ? chalk.green('✓ ok') : chalk.red('✗ failed'))
      }
    }
    return chalk.dim('  ╰─ done')
  }
  return null
}

// ---------------------------------------------------------------------------
// Event handler for terminal display
// ---------------------------------------------------------------------------

function handleStreamEvent(event: StreamEvent): boolean {
  const type = (event.type ?? '').toLowerCase()
  // Track whether text has been written without a trailing newline
  return handleStreamEventInner(type, event)
}

let _printedText = false

function handleStreamEventInner(type: string, event: StreamEvent): boolean {
  switch (type) {
    case 'text_start':
      break

    case 'text_delta':
      if (typeof event.delta === 'string') {
        process.stdout.write(event.delta)
        _printedText = true
      }
      break

    case 'text_end':
      if (_printedText) {
        process.stdout.write('\n')
        _printedText = false
      }
      break

    case 'tool_call_start': {
      if (_printedText) {
        process.stdout.write('\n')
        _printedText = false
      }
      const toolName = event.toolName as string
      const toolInput = event.input
      if (toolName === 'exec') {
        console.log(formatExecCall(toolInput))
      } else if (toolName === 'edit') {
        console.log(formatEditCall(toolInput))
      } else {
        const displayConfig = getToolDisplay(toolName)
        if (displayConfig.mode === 'expanded') {
          console.log(formatToolCallExpanded(displayConfig, toolName, toolInput))
        } else {
          console.log(formatToolCallInline(toolName, toolInput))
        }
      }
      break
    }

    case 'tool_call_end': {
      const toolName = event.toolName as string
      const result = event.result
      const resultLine = formatToolResult(toolName, result)
      if (resultLine) console.log(resultLine)
      break
    }

    case 'reasoning_start':
      if (_printedText) {
        process.stdout.write('\n')
        _printedText = false
      }
      process.stdout.write(chalk.dim('  💭 '))
      break

    case 'reasoning_delta':
      if (typeof event.delta === 'string') {
        process.stdout.write(chalk.dim(event.delta))
        _printedText = true
      }
      break

    case 'reasoning_end':
      if (_printedText) {
        process.stdout.write('\n')
        _printedText = false
      }
      break

    case 'error': {
      const errMsg = typeof event.message === 'string'
        ? event.message
        : typeof event.error === 'string'
          ? event.error
          : 'Stream error'
      console.log(chalk.red(`Error: ${errMsg}`))
      break
    }

    case 'processing_started':
    case 'processing_completed':
    case 'processing_failed':
    case 'agent_start':
    case 'agent_end':
      break

    default: {
      // Fallback: try to extract text (aligned with frontend extractFallbackText)
      if (typeof event.delta === 'string') {
        process.stdout.write(event.delta)
        _printedText = true
      } else if (typeof (event as Record<string, unknown>).text === 'string') {
        process.stdout.write((event as Record<string, unknown>).text as string)
        _printedText = true
      } else if (typeof (event as Record<string, unknown>).content === 'string') {
        process.stdout.write((event as Record<string, unknown>).content as string)
        _printedText = true
      }
      break
    }
  }
  return true
}

// ---------------------------------------------------------------------------
// Stream chat
// Strictly follows frontend streamMessage() in useChat.ts:
//   client.post({ url: '/bots/{bot_id}/messages/stream', path: { bot_id }, ... })
// ---------------------------------------------------------------------------

export const streamChat = async (query: string, botId: string) => {
  _printedText = false

  try {
    // Exactly matches frontend: client.post() with parseAs: 'stream'
    const { data: body } = await client.post({
      url: '/bots/{bot_id}/messages/stream',
      path: { bot_id: botId },
      body: { query, current_channel: 'cli', channels: ['cli'] },
      parseAs: 'stream',
      throwOnError: true,
    }) as { data: ReadableStream<Uint8Array> }

    if (!body) {
      console.log(chalk.red('No response body'))
      return false
    }

    // Use the same readSSEStream + parseStreamPayload as frontend
    await readSSEStream(body, (payload) => {
      const event = parseStreamPayload(payload)
      if (event) handleStreamEvent(event)
    })

    if (_printedText) {
      process.stdout.write('\n')
    }
    return true
  } catch (err) {
    if (_printedText) {
      process.stdout.write('\n')
    }
    const msg = err instanceof Error ? err.message : String(err)
    console.log(chalk.red(`Stream error: ${msg}`))
    return false
  }
}
