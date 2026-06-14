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
// (write, edit, exec, message sends, schedule mutations, …) is an "action" segment.
// Consecutive tools of the same category are grouped together; reasoning rides
// along with whichever segment it sits next to.
const READONLY_TOOLS = new Set([
  'read', 'list', 'web_search', 'web_fetch', 'memory.search', 'search_messages',
  'list_contacts', 'list_sessions', 'email.list', 'email.read', 'email.accounts_list',
  'schedule.list', 'schedule.get', 'background.status', 'browser.observe', 'computer.observe',
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
    case 'background.status': {
      const action = pickString(input, 'action') || 'list'
      return { icon: ListChecks, actionKey: 'background.status', target: action }
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
    case 'memory.search': {
      const query = pickString(input, 'query')
      return {
        icon: Brain,
        actionKey: 'memory.search',
        target: query ? `"${query}"` : '',
        fullTarget: query,
        detail: ToolCallDetailMemory,
      }
    }
    case 'message.send': {
      const target = pickString(input, 'target')
      const text = pickString(input, 'text', 'message')
      const display = target || truncate(text, 60)
      return {
        icon: Send,
        actionKey: 'message.send',
        target: display,
        fullTarget: text || target,
        detail: ToolCallDetailSend,
      }
    }
    case 'message.react': {
      const emoji = pickString(input, 'emoji')
      const remove = input.remove === true
      if (remove) {
        return {
          icon: Smile,
          actionKey: 'message.react_remove',
          target: pickString(input, 'message_id'),
        }
      }
      return { icon: Smile, actionKey: 'message.react', target: emoji }
    }
    case 'list_contacts': {
      return {
        icon: Users,
        actionKey: 'list_contacts',
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
    case 'schedule.list':
      return { icon: Calendar, actionKey: 'schedule.list', target: '', detail: ToolCallDetailSchedule }
    case 'schedule.get':
      return { icon: Calendar, actionKey: 'schedule.get', target: pickString(input, 'id') }
    case 'schedule.create':
      return {
        icon: CalendarPlus,
        actionKey: 'schedule.create',
        target: pickString(input, 'name'),
      }
    case 'schedule.update':
      return {
        icon: CalendarCog,
        actionKey: 'schedule.update',
        target: pickString(input, 'name', 'id'),
      }
    case 'schedule.delete':
      return {
        icon: CalendarMinus,
        actionKey: 'schedule.delete',
        target: pickString(input, 'id'),
      }
    case 'email.accounts_list':
      return {
        icon: Mail,
        actionKey: 'email.accounts_list',
        target: '',
        detail: ToolCallDetailEmailAccounts,
      }
    case 'email.send': {
      const subject = pickString(input, 'subject')
      const to = pickString(input, 'to')
      return {
        icon: MailPlus,
        actionKey: 'email.send',
        target: subject || to,
        fullTarget: subject ? `${to} — ${subject}` : to,
      }
    }
    case 'email.list':
      return {
        icon: Inbox,
        actionKey: 'email.list',
        target: '',
        detail: ToolCallDetailEmailList,
      }
    case 'email.read': {
      const uid = input.uid
      const target = uid != null ? `#${String(uid)}` : ''
      return {
        icon: MailOpen,
        actionKey: 'email.read',
        target,
        detail: ToolCallDetailEmailRead,
      }
    }
    case 'audio.speak': {
      const text = pickString(input, 'text')
      return {
        icon: Volume2,
        actionKey: 'audio.speak',
        target: truncate(text, 60),
        fullTarget: text,
      }
    }
    case 'audio.transcribe': {
      const target = pickString(
        input,
        'path',
        'audio_path',
        'file_path',
        'url',
        'audio_url',
      )
      return { icon: AudioLines, actionKey: 'audio.transcribe', target }
    }
    case 'image.generate': {
      const prompt = pickString(input, 'prompt')
      return {
        icon: ImagePlus,
        actionKey: 'image.generate',
        target: truncate(prompt, 60),
        fullTarget: prompt,
        detail: ToolCallDetailImage,
      }
    }
    case 'agent.spawn': {
      const tasks = Array.isArray(input.tasks) ? (input.tasks as unknown[]).length : 0
      return {
        icon: Workflow,
        actionKey: 'agent.spawn',
        actionParams: { count: tasks },
        target: '',
        detail: ToolCallDetailSpawn,
      }
    }
    case 'skill.use':
      return {
        icon: Sparkles,
        actionKey: 'skill.use',
        target: pickString(input, 'skillName'),
      }
    case 'browser.action': {
      const resolved = resolveGuiAction(BROWSER_ACTION_ICONS, 'browserAction', MousePointerClick, 'browser.action', pickString(input, 'action'))
      const target = pickString(input, 'url', 'ref', 'selector')
      return {
        ...resolved,
        target,
        fullTarget: pickString(input, 'url') || target,
        detail: ToolCallDetailBrowser,
      }
    }
    case 'browser.observe': {
      const resolved = resolveGuiAction(BROWSER_OBSERVE_ICONS, 'browserObserve', Eye, 'browser.observe', pickString(input, 'observe'))
      return {
        ...resolved,
        target: pickString(input, 'ref', 'selector'),
        detail: ToolCallDetailBrowser,
      }
    }
    case 'computer.observe': {
      const resolved = resolveGuiAction(COMPUTER_OBSERVE_ICONS, 'computerObserve', Monitor, 'computer.observe', pickString(input, 'observe'))
      return {
        ...resolved,
        target: '',
        detail: ToolCallDetailComputer,
      }
    }
    case 'computer.action': {
      const resolved = resolveGuiAction(COMPUTER_ACTION_ICONS, 'computerAction', MousePointer2, 'computer.action', pickString(input, 'action'))
      const x = input.x
      const y = input.y
      const coords = typeof x === 'number' && typeof y === 'number' ? `${x}, ${y}` : ''
      return {
        ...resolved,
        target: pickString(input, 'ref') || coords,
        detail: ToolCallDetailComputer,
      }
    }
    case 'browser.remote_session': {
      const resolved = resolveGuiAction(REMOTE_SESSION_ICONS, 'remoteSession', Cable, 'browser.remote_session', pickString(input, 'action'))
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
