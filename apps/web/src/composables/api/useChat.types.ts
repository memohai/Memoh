import type { BotsBot } from '@memohai/sdk'

export type Bot = BotsBot

export interface SessionSummary {
  id: string
  bot_id: string
  route_id?: string
  channel_type?: string
  type?: string
  session_mode?: string
  runtime_type?: string
  title: string
  metadata?: Record<string, unknown>
  runtime_metadata?: Record<string, unknown>
  parent_session_id?: string
  created_at?: string
  updated_at?: string
  route_metadata?: Record<string, unknown>
  route_conversation_type?: string
}

export interface MessageAsset {
  content_hash: string
  role: string
  ordinal: number
  mime: string
  size_bytes: number
  storage_key: string
  name?: string
  metadata?: Record<string, unknown>
}

export interface Message {
  id: string
  bot_id: string
  session_id?: string
  sender_channel_identity_id?: string
  sender_user_id?: string
  sender_display_name?: string
  sender_avatar_url?: string
  platform?: string
  external_message_id?: string
  source_reply_to_message_id?: string
  role: string
  content?: unknown
  metadata?: Record<string, unknown>
  assets?: MessageAsset[]
  display_content?: string
  created_at?: string
}

// Per-session SSE: `/bots/{bot_id}/sessions/{session_id}/messages/events`.
// Server pushes a small backlog of `message_created` events followed by live
// `message_created` / `session_title_updated` events scoped to this session
// only. `ping` is a server heartbeat; `dropped` asks the client to refresh
// because the server-side subscriber buffer overflowed.
export interface SessionMessageCreatedEvent {
  type: 'message_created'
  bot_id?: string
  message: Message
}

export interface SessionTitleUpdatedEvent {
  type: 'session_title_updated'
  bot_id?: string
  session_id: string
  title: string
}

export interface SessionPingEvent {
  type: 'ping'
}

export interface SessionDroppedEvent {
  type: 'dropped'
  count?: number
}

export interface SessionBackgroundTaskEvent extends UIBackgroundTask {
  type: 'background_task'
  task?: UIBackgroundTask
}

export type SessionMessageStreamEvent =
  | SessionMessageCreatedEvent
  | SessionTitleUpdatedEvent
  | SessionBackgroundTaskEvent
  | SessionDroppedEvent
  | SessionPingEvent

// Bot-wide activity SSE: `/bots/{bot_id}/sessions/events`. Carries identifier
// + minimal metadata for sidebar live-sort; never includes message bodies.
export interface SessionTouchedEvent {
  type: 'session_touched'
  session_id: string
  updated_at?: string
}

export interface SessionTitleChangedEvent {
  type: 'session_title_changed'
  session_id: string
  title: string
}

export interface SessionCreatedEvent {
  type: 'session_created'
  session_id: string
  // `type` here is the session kind (chat | discuss | acp_agent), already
  // filtered server-side to user-facing types.
  session_type?: string
  title?: string
}

export type BotSessionActivityEvent =
  | SessionTouchedEvent
  | SessionTitleChangedEvent
  | SessionCreatedEvent
  | SessionDroppedEvent
  | SessionPingEvent

export interface FetchMessagesOptions {
  limit?: number
  before?: string
  session_id?: string
}

export interface ChatAttachment {
  type: string
  base64: string
  mime?: string
  name?: string
}

export interface UIAttachment {
  id?: string
  type: string
  path?: string
  url?: string
  base64?: string
  name?: string
  content_hash?: string
  bot_id?: string
  mime?: string
  size?: number
  storage_key?: string
  metadata?: Record<string, unknown>
}

export interface UIReplyRef {
  message_id?: string
  sender?: string
  preview?: string
  attachments?: UIAttachment[]
}

export interface UIForwardRef {
  message_id?: string
  from_user_id?: string
  from_conversation_id?: string
  sender?: string
  date?: number
}

export interface UITextMessage {
  id: number
  type: 'text'
  content: string
}

export interface UIReasoningMessage {
  id: number
  type: 'reasoning'
  content: string
}

export interface UIToolMessage {
  id: number
  type: 'tool'
  name: string
  input: unknown
  output?: unknown
  tool_call_id: string
  running: boolean
  progress?: unknown[]
  approval?: UIToolApproval
  user_input?: UIUserInput
  background_task?: UIBackgroundTask
}

export interface UIBackgroundTask {
  event?: string
  task_id?: string
  bot_id?: string
  session_id?: string
  command?: string
  agent_id?: string
  agent_session_id?: string
  status?: string
  stream?: string
  chunk?: string
  tail?: string
  output_file?: string
  output_tail?: string
  exit_code?: number
  duration?: string
  stalled?: boolean
}

export interface UIToolApproval {
  approval_id: string
  short_id?: number
  status: string
  decision_reason?: string
  can_approve?: boolean
}

export interface UIUserInput {
  user_input_id: string
  short_id?: number
  status: string
  questions?: UIUserInputQuestion[]
  can_respond?: boolean
}

export interface UIUserInputQuestion {
  id: string
  text: string
  kind: 'single_select' | 'multi_select' | 'text'
  options?: UIUserInputOption[]
  allow_custom?: boolean
  placeholder?: string
}

export interface UIUserInputOption {
  id: string
  label: string
  description?: string
}

export interface UIAttachmentsMessage {
  id: number
  type: 'attachments'
  attachments: UIAttachment[]
}

export interface UIErrorMessage {
  id: number
  type: 'error'
  content: string
}

export type UIMessage = UITextMessage | UIReasoningMessage | UIToolMessage | UIAttachmentsMessage | UIErrorMessage

export interface UIUserTurn {
  role: 'user'
  text: string
  attachments?: UIAttachment[]
  reply?: UIReplyRef
  forward?: UIForwardRef
  timestamp: string
  platform?: string
  sender_display_name?: string
  sender_avatar_url?: string
  sender_user_id?: string
  external_message_id?: string
  id?: string
}

export interface UIAssistantTurn {
  role: 'assistant'
  messages: UIMessage[]
  timestamp: string
  platform?: string
  external_message_id?: string
  id?: string
}

export interface UISystemTurn {
  role: 'system'
  kind?: 'background_task' | string
  background_task?: UIBackgroundTask
  timestamp: string
  platform?: string
  id?: string
}

export type UITurn = UIUserTurn | UIAssistantTurn | UISystemTurn

export interface UIStreamStartEvent {
  type: 'start'
  stream_id?: string
  session_id?: string
}

export interface UIStreamMessageEvent {
  type: 'message'
  stream_id?: string
  session_id?: string
  data: UIMessage
}

export interface UIStreamEndEvent {
  type: 'end'
  stream_id?: string
  session_id?: string
}

export interface UIStreamErrorEvent {
  type: 'error'
  stream_id?: string
  session_id?: string
  message: string
  feedback?: unknown
}

export type UIStreamEvent =
  | UIStreamStartEvent
  | UIStreamMessageEvent
  | UIStreamEndEvent
  | UIStreamErrorEvent

export type UIStreamEventHandler = (event: UIStreamEvent) => void
