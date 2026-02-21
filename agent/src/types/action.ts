import { LanguageModelUsage, ModelMessage } from 'ai'
import { AgentInput } from './agent'
import { AgentAttachment } from './attachment'

export interface BaseAction {
  type: string
  metadata?: Record<string, unknown>
}

export interface AgentStartAction extends BaseAction {
  type: 'agent_start'
  input: AgentInput
}

export interface ReasoningStartAction extends BaseAction {
  type: 'reasoning_start'
}

export interface ReasoningDeltaAction extends BaseAction {
  type: 'reasoning_delta'
  delta: string
}

export interface ReasoningEndAction extends BaseAction {
  type: 'reasoning_end'
}

export interface TextStartAction extends BaseAction {
  type: 'text_start'
}

export interface TextDeltaAction extends BaseAction {
  type: 'text_delta'
  delta: string
}

export interface AttachmentDeltaAction extends BaseAction {
  type: 'attachment_delta'
  attachments: AgentAttachment[]
}

export interface TextEndAction extends BaseAction {
  type: 'text_end'
}

export interface ToolCallStartAction extends BaseAction {
  type: 'tool_call_start'
  toolName: string
  toolCallId: string
  input: unknown
}

export interface ToolCallEndAction extends BaseAction {
  type: 'tool_call_end'
  toolName: string
  toolCallId: string
  input: unknown
  result: unknown
}

export interface AgentEndAction extends BaseAction {
  type: 'agent_end'
  messages: ModelMessage[]
  skills: string[]
  reasoning: string[]
  usage: LanguageModelUsage
  usages: (LanguageModelUsage | null)[]
}

export type AgentAction = 
  | AgentStartAction
  | ReasoningStartAction
  | ReasoningDeltaAction
  | ReasoningEndAction
  | TextStartAction
  | TextDeltaAction
  | AttachmentDeltaAction
  | TextEndAction
  | ToolCallStartAction
  | ToolCallEndAction
  | AgentEndAction
