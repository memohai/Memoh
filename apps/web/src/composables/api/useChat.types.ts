import type { BotsBot } from '@memohai/sdk'

export type Bot = BotsBot

export interface SessionSummary {
  id: string
  bot_id: string
  route_id?: string
  channel_type?: string
  type?: string
  title: string
  metadata?: Record<string, unknown>
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

export interface StreamEvent {
  type?:
    | 'text_start' | 'text_delta' | 'text_end'
    | 'reasoning_start' | 'reasoning_delta' | 'reasoning_end'
    | 'tool_call_start' | 'tool_call_progress' | 'tool_call_end'
    | 'attachment_delta' | 'reaction_delta'
    | 'agent_start' | 'agent_end' | 'agent_abort'
    | 'processing_started' | 'processing_completed' | 'processing_failed'
    | 'error'
  delta?: string
  toolCallId?: string
  toolName?: string
  input?: unknown
  progress?: unknown
  result?: unknown
  attachments?: Array<Record<string, unknown>>
  error?: string
  message?: string
  [key: string]: unknown
}

export type StreamEventHandler = (event: StreamEvent) => void

export interface MessageStreamEvent {
  type: string
  bot_id?: string
  message?: Message
  session_id?: string
  title?: string
}

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
}

export interface UIToolApproval {
  approval_id: string
  short_id?: number
  status: string
  decision_reason?: string
  can_approve?: boolean
}

export interface UIAttachmentsMessage {
  id: number
  type: 'attachments'
  attachments: UIAttachment[]
}

export type UIMessage = UITextMessage | UIReasoningMessage | UIToolMessage | UIAttachmentsMessage

export interface UIUserTurn {
  role: 'user'
  text: string
  attachments?: UIAttachment[]
  timestamp: string
  platform?: string
  sender_display_name?: string
  sender_avatar_url?: string
  sender_user_id?: string
  id?: string
}

export interface UIAssistantTurn {
  role: 'assistant'
  messages: UIMessage[]
  timestamp: string
  platform?: string
  id?: string
}

export type UITurn = UIUserTurn | UIAssistantTurn

export interface UIStreamStartEvent {
  type: 'start'
}

export interface UIStreamMessageEvent {
  type: 'message'
  data: UIMessage
}

export interface UIStreamEndEvent {
  type: 'end'
}

export interface UIStreamErrorEvent {
  type: 'error'
  message: string
}

export type UIStreamEvent =
  | UIStreamStartEvent
  | UIStreamMessageEvent
  | UIStreamEndEvent
  | UIStreamErrorEvent

export type UIStreamEventHandler = (event: UIStreamEvent) => void
