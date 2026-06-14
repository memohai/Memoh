import type { Component } from 'vue'
import {
  Activity,
  AppWindow,
  ArrowLeft,
  ArrowRight,
  AudioLines,
  Braces,
  Brain,
  Cable,
  Calendar,
  CalendarCog,
  CalendarMinus,
  CalendarPlus,
  Camera,
  ChevronDown,
  Code,
  Eye,
  FilePen,
  FilePlus2,
  FileText,
  FolderOpen,
  Focus,
  Globe,
  Heading,
  ImagePlus,
  Inbox,
  Keyboard,
  Link,
  ListChecks,
  Mail,
  MailOpen,
  MailPlus,
  MessagesSquare,
  Monitor,
  MousePointer2,
  MousePointerClick,
  Move,
  MoveVertical,
  Plug,
  Plus,
  RotateCw,
  ScanEye,
  Search,
  SearchCheck,
  Send,
  Smile,
  Sparkles,
  Square,
  SquareCheck,
  SquareTerminal,
  TextCursorInput,
  Timer,
  Unplug,
  Upload,
  Users,
  Volume2,
  Workflow,
  Wrench,
  X,
} from 'lucide-vue-next'
import type { ToolCallBlock } from '@/store/chat-list'
import ToolCallDetailBrowser from './tool-call-detail-browser.vue'
import ToolCallDetailApplyPatch from './tool-call-detail-apply-patch.vue'
import ToolCallDetailComputer from './tool-call-detail-computer.vue'
import ToolCallDetailContacts from './tool-call-detail-contacts.vue'
import ToolCallDetailEdit from './tool-call-detail-edit.vue'
import ToolCallDetailEmailAccounts from './tool-call-detail-email-accounts.vue'
import ToolCallDetailEmailList from './tool-call-detail-email-list.vue'
import ToolCallDetailEmailRead from './tool-call-detail-email-read.vue'
import ToolCallDetailExec from './tool-call-detail-exec.vue'
import ToolCallDetailImage from './tool-call-detail-image.vue'
import ToolCallDetailMemory from './tool-call-detail-memory.vue'
import ToolCallDetailOutput from './tool-call-detail-output.vue'
import ToolCallDetailRemoteSession from './tool-call-detail-remote-session.vue'
import ToolCallDetailSchedule from './tool-call-detail-schedule.vue'
import ToolCallDetailSend from './tool-call-detail-send.vue'
import ToolCallDetailSpawn from './tool-call-detail-spawn.vue'
import ToolCallDetailWebFetch from './tool-call-detail-web-fetch.vue'
import ToolCallDetailWebSearch from './tool-call-detail-web-search.vue'
import ToolCallDetailWrite from './tool-call-detail-write.vue'

export interface ToolDisplay {
  icon: Component
  actionKey: string
  actionParams?: Record<string, unknown>
  target: string
  fullTarget?: string
  detail?: Component
  isError?: boolean
  errorSuffix?: string
  expandable?: boolean
  defaultOpen?: boolean
  diffAdd?: number
  diffRemove?: number
  hideAction?: boolean
  // 'card' = output/diff/file content in a grayscale card; 'inline' = a
  // half-embedded key:value list (params), no card. Defaults to 'card'.
  detailVariant?: 'card' | 'inline'
}

const FILE_PATH_TOOLS = new Set(['read', 'write', 'edit', 'list'])

export function isFilePathTool(toolName: string): boolean {
  return FILE_PATH_TOOLS.has(toolName)
}

export function isDirPathTool(toolName: string): boolean {
  return toolName === 'list'
}

// Read-only / no-side-effect tools form an "explore" segment; everything else
// (write, edit, exec, send, schedule mutations, …) is an "action" segment.
// Consecutive tools of the same category are grouped together; reasoning rides
// along with whichever segment it sits next to.
const READONLY_TOOLS = new Set([
  'read', 'list', 'web_search', 'web_fetch', 'search_memory', 'search_messages',
  'get_contacts', 'list_sessions', 'list_email', 'read_email', 'list_email_accounts',
  'list_schedule', 'get_schedule', 'list_skills', 'bg_status', 'list_background', 'get_background_status',
  'browser_observe', 'computer_observe',
])

export function isReadOnlyTool(toolName: string): boolean {
  return READONLY_TOOLS.has(toolName)
}

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

function pickString(obj: Record<string, unknown>, ...keys: string[]): string {
  for (const k of keys) {
    const v = obj[k]
    if (typeof v === 'string' && v.length > 0) return v
  }
  return ''
}

function firstQuestionText(input: Record<string, unknown>): string {
  const questions = input.questions
  if (!Array.isArray(questions) || questions.length === 0) return ''
  return pickString(asObject(questions[0]), 'text')
}

function pickNumber(obj: Record<string, unknown>, ...keys: string[]): number {
  for (const k of keys) {
    const v = obj[k]
    if (typeof v === 'number' && Number.isFinite(v)) return v
  }
  return 0
}

function truncate(s: string, max = 60): string {
  if (!s) return ''
  if (s.length <= max) return s
  return `${s.slice(0, max)}…`
}

// File-path tools show just the filename in the row; the absolute path becomes
// the tooltip via fullTarget.
function basename(path: string): string {
  if (!path) return ''
  const parts = path.split('/').filter(Boolean)
  return parts[parts.length - 1] ?? path
}

function firstLine(s: string, max = 80): string {
  if (!s) return ''
  const idx = s.indexOf('\n')
  const line = idx === -1 ? s : `${s.slice(0, idx)} …`
  return truncate(line, max)
}

function lineCount(s: string): number {
  if (!s) return 0
  return s.split('\n').length
}

function resultObject(block: ToolCallBlock): Record<string, unknown> {
  const result = asObject(block.result)
  const sc = asObject(result.structuredContent)
  return Object.keys(sc).length > 0 ? sc : result
}

interface PatchFileTarget {
  operation: 'add' | 'modify' | 'delete'
  path: string
}

function normalizePatchOperation(value: unknown): PatchFileTarget['operation'] | '' {
  if (value === 'add' || value === 'added') return 'add'
  if (value === 'modify' || value === 'modified' || value === 'update') return 'modify'
  if (value === 'delete' || value === 'deleted') return 'delete'
  return ''
}

function patchFilesFromResult(block: ToolCallBlock): PatchFileTarget[] {
  const result = resultObject(block)
  const rawFiles = result.files
  if (Array.isArray(rawFiles)) {
    return rawFiles
      .map((item) => {
        const obj = asObject(item)
        const path = pickString(obj, 'path')
        const operation = normalizePatchOperation(obj.operation)
        return path && operation ? { operation, path } : null
      })
      .filter((item): item is PatchFileTarget => Boolean(item))
  }

  const out: PatchFileTarget[] = []
  for (const [key, operation] of [
    ['added', 'add'],
    ['modified', 'modify'],
    ['deleted', 'delete'],
  ] as const) {
    const paths = result[key]
    if (!Array.isArray(paths)) continue
    for (const path of paths) {
      if (typeof path === 'string' && path) out.push({ operation, path })
    }
  }
  return out
}

function patchFilesFromInput(patch: string): PatchFileTarget[] {
  if (!patch) return []
  const out: PatchFileTarget[] = []
  for (const rawLine of patch.split('\n')) {
    const line = rawLine.trim()
    if (line.startsWith('*** Add File: ')) {
      const path = line.slice('*** Add File: '.length).trim()
      if (path) out.push({ operation: 'add', path })
    }
    else if (line.startsWith('*** Delete File: ')) {
      const path = line.slice('*** Delete File: '.length).trim()
      if (path) out.push({ operation: 'delete', path })
    }
    else if (line.startsWith('*** Update File: ')) {
      const path = line.slice('*** Update File: '.length).trim()
      if (path) out.push({ operation: 'modify', path })
    }
  }
  return out
}

function patchLineCounts(patch: string): { add: number; remove: number } {
  let add = 0
  let remove = 0
  for (const line of patch.split('\n')) {
    if (line.startsWith('+')) add++
    else if (line.startsWith('-') && !line.startsWith('***')) remove++
  }
  return { add, remove }
}

function hostnameOrUrl(url: string): string {
  if (!url) return ''
  try {
    const parsed = new URL(url)
    return parsed.hostname || url
  }
  catch {
    return url
  }
}

// Compatibility aliases accepted by the backend browser/computer tools.
const GUI_ACTION_ALIASES: Record<string, string> = {
  dblclick: 'double_click',
  scrollintoview: 'scroll_into_view',
}

function normalizeGuiAction(raw: string): string {
  const key = raw.trim().toLowerCase()
  return GUI_ACTION_ALIASES[key] ?? key
}

const BROWSER_ACTION_ICONS: Record<string, Component> = {
  navigate: Globe,
  click: MousePointerClick,
  double_click: MousePointerClick,
  focus: Focus,
  type: Keyboard,
  fill: TextCursorInput,
  press: Keyboard,
  hover: MousePointer2,
  select: ChevronDown,
  check: SquareCheck,
  uncheck: Square,
  scroll: MoveVertical,
  scroll_into_view: MoveVertical,
  drag: Move,
  upload: Upload,
  wait: Timer,
  go_back: ArrowLeft,
  go_forward: ArrowRight,
  reload: RotateCw,
  tab_new: Plus,
  tab_select: AppWindow,
  tab_close: X,
}

const BROWSER_OBSERVE_ICONS: Record<string, Component> = {
  snapshot: ScanEye,
  get_content: FileText,
  screenshot_annotate: Camera,
  screenshot: Camera,
  get_html: Code,
  evaluate: Braces,
  get_url: Link,
  get_title: Heading,
  pdf: FileText,
  tab_list: AppWindow,
}

const COMPUTER_OBSERVE_ICONS: Record<string, Component> = {
  snapshot: ScanEye,
  screenshot: Camera,
}

const COMPUTER_ACTION_ICONS: Record<string, Component> = {
  click: MousePointerClick,
  double_click: MousePointerClick,
  type: Keyboard,
  fill: TextCursorInput,
  key: Keyboard,
  scroll: MoveVertical,
  drag: Move,
  wait: Timer,
  mouse_move: MousePointer2,
  pointer: MousePointer2,
}

const REMOTE_SESSION_ICONS: Record<string, Component> = {
  create: Plug,
  close: Unplug,
  status: Activity,
}

// Resolves a per-action icon and i18n action key. When the action is known the
// label comes from a nested namespace key (e.g. chat.tools.browserAction.click);
// unknown actions fall back to the tool's generic label with the raw action as
// a parameter.
function resolveGuiAction(
  icons: Record<string, Component>,
  namespace: string,
  fallbackIcon: Component,
  fallbackKey: string,
  rawAction: string,
): { icon: Component; actionKey: string; actionParams?: Record<string, unknown> } {
  const action = normalizeGuiAction(rawAction)
  const icon = icons[action]
  if (icon) {
    return { icon, actionKey: `${namespace}.${action}` }
  }
  return { icon: fallbackIcon, actionKey: fallbackKey, actionParams: { action: rawAction } }
}

export function getToolDisplay(block: ToolCallBlock): ToolDisplay {
  const input = asObject(block.input)

  switch (block.toolName) {
    case 'ask_user': {
      // pickString covers pre-v2 history where input was { question: "..." }.
      const question = block.userInput?.questions?.[0]?.text || firstQuestionText(input) || pickString(input, 'question')
      const showQuestionInBody = block.userInput?.status === 'pending'
      return {
        icon: TextCursorInput,
        actionKey: 'ask_user',
        target: showQuestionInBody ? '' : truncate(question, 80),
        fullTarget: showQuestionInBody ? '' : question,
        expandable: true,
      }
    }
    case 'read': {
      const path = pickString(input, 'path')
      return { icon: FileText, actionKey: 'read', target: basename(path), fullTarget: path, detail: ToolCallDetailOutput }
    }
    case 'write': {
      const path = pickString(input, 'path')
      const content = pickString(input, 'content')
      const contentLineCount = pickNumber(input, 'content_line_count')
      return {
        icon: FilePlus2,
        actionKey: 'write',
        target: basename(path),
        fullTarget: path,
        detail: ToolCallDetailWrite,
        defaultOpen: true,
        diffAdd: contentLineCount || lineCount(content),
        hideAction: true,
      }
    }
    case 'edit': {
      const path = pickString(input, 'path')
      const oldText = pickString(input, 'old_text')
      const newText = pickString(input, 'new_text')
      return {
        icon: FilePen,
        actionKey: 'edit',
        target: basename(path),
        fullTarget: path,
        detail: ToolCallDetailEdit,
        diffAdd: lineCount(newText),
        diffRemove: lineCount(oldText),
      }
    }
    case 'apply_patch': {
      const patch = pickString(input, 'patch')
      const files = patchFilesFromResult(block)
      const fileTargets = files.length > 0 ? files : patchFilesFromInput(patch)
      const target = fileTargets.length === 1
        ? basename(fileTargets[0]!.path)
        : fileTargets.length > 1
          ? `${fileTargets.length} files`
          : ''
      const fullTarget = fileTargets
        .map(file => `${file.operation} ${file.path}`)
        .join('\n')
      const counts = patchLineCounts(patch)
      return {
        icon: FilePen,
        actionKey: 'apply_patch',
        target,
        fullTarget,
        detail: ToolCallDetailApplyPatch,
        defaultOpen: true,
        diffAdd: counts.add,
        diffRemove: counts.remove,
      }
    }
    case 'list': {
      const path = pickString(input, 'path')
      return { icon: FolderOpen, actionKey: 'list', target: basename(path), fullTarget: path, detail: ToolCallDetailOutput }
    }
    case 'exec': {
      const cmd = pickString(input, 'command')
      return {
        icon: SquareTerminal,
        actionKey: 'exec',
        target: firstLine(cmd, 80),
        fullTarget: cmd,
        detail: ToolCallDetailExec,
      }
    }
    case 'bg_status': {
      const action = pickString(input, 'action') || 'list'
      return { icon: ListChecks, actionKey: 'bg_status', target: action }
    }
    case 'list_background':
      return { icon: ListChecks, actionKey: 'list_background' }
    case 'get_background_status': {
      const taskId = pickString(input, 'task_id', 'taskId')
      return { icon: SearchCheck, actionKey: 'get_background_status', target: taskId }
    }
    case 'kill_background': {
      const taskId = pickString(input, 'task_id', 'taskId')
      return { icon: X, actionKey: 'kill_background', target: taskId }
    }
    case 'web_search': {
      const query = pickString(input, 'query')
      return {
        icon: Search,
        actionKey: 'web_search',
        target: query ? `"${query}"` : '',
        fullTarget: query,
        detail: ToolCallDetailWebSearch,
      }
    }
    case 'web_fetch': {
      const url = pickString(input, 'url')
      return {
        icon: Globe,
        actionKey: 'web_fetch',
        target: hostnameOrUrl(url),
        fullTarget: url,
        detail: ToolCallDetailWebFetch,
      }
    }
    case 'search_memory': {
      const query = pickString(input, 'query')
      return {
        icon: Brain,
        actionKey: 'search_memory',
        target: query ? `"${query}"` : '',
        fullTarget: query,
        detail: ToolCallDetailMemory,
      }
    }
    case 'send': {
      const target = pickString(input, 'target')
      const text = pickString(input, 'text', 'message')
      const display = target || truncate(text, 60)
      return {
        icon: Send,
        actionKey: 'send',
        target: display,
        fullTarget: text || target,
        detail: ToolCallDetailSend,
      }
    }
    case 'react': {
      const emoji = pickString(input, 'emoji')
      const remove = input.remove === true
      if (remove) {
        return {
          icon: Smile,
          actionKey: 'react_remove',
          target: pickString(input, 'message_id'),
        }
      }
      return { icon: Smile, actionKey: 'react', target: emoji }
    }
    case 'get_contacts': {
      return {
        icon: Users,
        actionKey: 'get_contacts',
        target: pickString(input, 'platform'),
        detail: ToolCallDetailContacts,
      }
    }
    case 'list_sessions': {
      const target = pickString(input, 'platform') || pickString(input, 'type')
      return { icon: MessagesSquare, actionKey: 'list_sessions', target }
    }
    case 'search_messages': {
      const keyword = pickString(input, 'keyword')
      return {
        icon: SearchCheck,
        actionKey: 'search_messages',
        target: keyword ? `"${keyword}"` : '',
        fullTarget: keyword,
      }
    }
    case 'list_schedule':
      return { icon: Calendar, actionKey: 'list_schedule', target: '', detail: ToolCallDetailSchedule }
    case 'get_schedule':
      return { icon: Calendar, actionKey: 'get_schedule', target: pickString(input, 'id') }
    case 'create_schedule':
      return {
        icon: CalendarPlus,
        actionKey: 'create_schedule',
        target: pickString(input, 'name'),
      }
    case 'update_schedule':
      return {
        icon: CalendarCog,
        actionKey: 'update_schedule',
        target: pickString(input, 'name', 'id'),
      }
    case 'delete_schedule':
      return {
        icon: CalendarMinus,
        actionKey: 'delete_schedule',
        target: pickString(input, 'id'),
      }
    case 'list_email_accounts':
      return {
        icon: Mail,
        actionKey: 'list_email_accounts',
        target: '',
        detail: ToolCallDetailEmailAccounts,
      }
    case 'send_email': {
      const subject = pickString(input, 'subject')
      const to = pickString(input, 'to')
      return {
        icon: MailPlus,
        actionKey: 'send_email',
        target: subject || to,
        fullTarget: subject ? `${to} — ${subject}` : to,
      }
    }
    case 'list_email':
      return {
        icon: Inbox,
        actionKey: 'list_email',
        target: '',
        detail: ToolCallDetailEmailList,
      }
    case 'read_email': {
      const uid = input.uid
      const target = uid != null ? `#${String(uid)}` : ''
      return {
        icon: MailOpen,
        actionKey: 'read_email',
        target,
        detail: ToolCallDetailEmailRead,
      }
    }
    case 'speak': {
      const text = pickString(input, 'text')
      return {
        icon: Volume2,
        actionKey: 'speak',
        target: truncate(text, 60),
        fullTarget: text,
      }
    }
    case 'transcribe_audio': {
      const target = pickString(
        input,
        'path',
        'audio_path',
        'file_path',
        'url',
        'audio_url',
      )
      return { icon: AudioLines, actionKey: 'transcribe_audio', target }
    }
    case 'generate_image': {
      const prompt = pickString(input, 'prompt')
      return {
        icon: ImagePlus,
        actionKey: 'generate_image',
        target: truncate(prompt, 60),
        fullTarget: prompt,
        detail: ToolCallDetailImage,
      }
    }
    case 'spawn_agent': {
      const task = pickString(input, 'task')
      return {
        icon: Workflow,
        actionKey: 'spawn_agent',
        target: pickString(input, 'id') || truncate(task, 60),
        fullTarget: task,
        detail: ToolCallDetailSpawn,
      }
    }
    case 'send_message': {
      const message = pickString(input, 'message')
      return {
        icon: MessagesSquare,
        actionKey: 'send_message',
        target: pickString(input, 'id'),
        fullTarget: message,
        detail: ToolCallDetailSpawn,
      }
    }
    case 'wait_agent':
      return {
        icon: Timer,
        actionKey: 'wait_agent',
        target: pickString(input, 'id', 'task_id'),
        detail: ToolCallDetailSpawn,
      }
    case 'list_agents':
      return {
        icon: ListChecks,
        actionKey: 'list_agents',
        target: '',
        detail: ToolCallDetailSpawn,
      }
    case 'use_skill':
      return {
        icon: Sparkles,
        actionKey: 'use_skill',
        target: pickString(input, 'skillName'),
      }
    case 'list_skills':
      return {
        icon: Sparkles,
        actionKey: 'list_skills',
        target: '',
      }
    case 'browser_action': {
      const resolved = resolveGuiAction(BROWSER_ACTION_ICONS, 'browserAction', MousePointerClick, 'browser_action', pickString(input, 'action'))
      const target = pickString(input, 'url', 'ref', 'selector')
      return {
        ...resolved,
        target,
        fullTarget: pickString(input, 'url') || target,
        detail: ToolCallDetailBrowser,
      }
    }
    case 'browser_observe': {
      const resolved = resolveGuiAction(BROWSER_OBSERVE_ICONS, 'browserObserve', Eye, 'browser_observe', pickString(input, 'observe'))
      return {
        ...resolved,
        target: pickString(input, 'ref', 'selector'),
        detail: ToolCallDetailBrowser,
      }
    }
    case 'computer_observe': {
      const resolved = resolveGuiAction(COMPUTER_OBSERVE_ICONS, 'computerObserve', Monitor, 'computer_observe', pickString(input, 'observe'))
      return {
        ...resolved,
        target: '',
        detail: ToolCallDetailComputer,
      }
    }
    case 'computer_action': {
      const resolved = resolveGuiAction(COMPUTER_ACTION_ICONS, 'computerAction', MousePointer2, 'computer_action', pickString(input, 'action'))
      const x = input.x
      const y = input.y
      const coords = typeof x === 'number' && typeof y === 'number' ? `${x}, ${y}` : ''
      return {
        ...resolved,
        target: pickString(input, 'ref') || coords,
        detail: ToolCallDetailComputer,
      }
    }
    case 'browser_remote_session': {
      const resolved = resolveGuiAction(REMOTE_SESSION_ICONS, 'remoteSession', Cable, 'browser_remote_session', pickString(input, 'action'))
      return {
        ...resolved,
        target: pickString(input, 'session_id'),
        detail: ToolCallDetailRemoteSession,
      }
    }
    default:
      return {
        icon: Wrench,
        actionKey: 'generic',
        target: block.toolName,
        expandable: true,
        detailVariant: 'inline',
      }
  }
}
