import type {
  ChatAttachment,
  RequestedSkillRequest,
  RequestedSkillSelection,
} from '@/composables/api/useChat.types'

export type ChatIntentKind = 'send' | 'retry' | 'edit'
export type ChatIntentTerminalStatus = 'completed' | 'aborted' | 'errored' | 'interrupted' | 'lost'

export interface ChatIntentTarget {
  botId: string
  sessionId: string
  viewId: string
  composerScope?: string
}

export interface ChatRunHandle {
  botId: string
  sessionId: string
  streamId: string
  generation: string
}

interface IntentModelOptions {
  modelId?: string
  reasoningEffort?: string
  workspaceTargetId?: string
}

export interface SendChatIntentInput extends ChatIntentTarget, IntentModelOptions {
  text: string
  attachments?: ChatAttachment[]
  requestedSkills?: RequestedSkillSelection[]
}

export interface RetryChatIntentInput extends ChatIntentTarget, IntentModelOptions {
  messageId: string
}

export interface EditChatIntentInput extends ChatIntentTarget, IntentModelOptions {
  messageId: string
  text: string
}

export type ChatIntentInput = SendChatIntentInput | RetryChatIntentInput | EditChatIntentInput

export interface IntentPlaceholder {
  userTurnKey?: string
  assistantTurnKey: string
}

export interface PendingChatIntent {
  kind: ChatIntentKind
  botId: string
  sessionId: string
  streamId: string
  generation: string
  viewId: string
  composerScope: string
  text: string
  attachments: ChatAttachment[]
  requestedSkills: RequestedSkillSelection[]
  replaceFromMessageId: string
  userTurnKey?: string
  assistantTurnKey: string
  abortRequested: boolean
  cancelSent: boolean
  abortSentGeneration: string
}

export interface IntentPlaceholderRequest {
  kind: ChatIntentKind
  botId: string
  sessionId: string
  streamId: string
  viewId: string
  composerScope: string
  text: string
  attachments: ChatAttachment[]
  requestedSkills: RequestedSkillSelection[]
  replaceFromMessageId: string
}

export interface RestoreComposerDraftRequest {
  botId: string
  sessionId: string
  viewId: string
  composerScope: string
  text: string
  attachments: ChatAttachment[]
  requestedSkills: RequestedSkillSelection[]
}

export interface IntentTerminalResult extends ChatRunHandle {
  status: ChatIntentTerminalStatus
  persistence: 'unknown' | 'persisted' | 'vanished'
  stableIds?: string[]
}

export type ChatIntentWireMessage =
  | {
      type: 'message'
      stream_id: string
      invocation_id: string
      composer_scope: string
      text: string
      session_id?: string
      attachments?: ChatAttachment[]
      requested_skills?: RequestedSkillRequest[]
      model_id?: string
      reasoning_effort?: string
      workspace_target_id?: string
    }
  | {
      type: 'retry_message'
      stream_id: string
      session_id: string
      message_id: string
      model_id?: string
      reasoning_effort?: string
      workspace_target_id?: string
    }
  | {
      type: 'edit_message'
      stream_id: string
      session_id: string
      message_id: string
      text: string
      model_id?: string
      reasoning_effort?: string
      workspace_target_id?: string
    }
  | {
      type: 'abort'
      stream_id: string
      session_id: string
      generation?: string
    }
  | {
      type: 'steer_current_run'
      stream_id: string
      session_id: string
      generation: string
      text: string
    }

export interface ChatIntentsDeps {
  createStreamId: () => string
  dispatch: (botId: string, message: ChatIntentWireMessage) => boolean
  createPlaceholder: (request: IntentPlaceholderRequest) => IntentPlaceholder
  rollbackPlaceholder: (intent: PendingChatIntent) => void
  removeVanished: (intent: PendingChatIntent) => void
  restoreReplacement: (intent: PendingChatIntent) => void
  restoreComposerDraft: (request: RestoreComposerDraftRequest) => void
  onClaim?: (intent: PendingChatIntent, handle: ChatRunHandle) => void
  onRebind?: (intent: PendingChatIntent, previousSessionId: string) => void
  onPersisted?: (intent: PendingChatIntent, result: IntentTerminalResult) => void
}

export type StartIntentResult =
  | { ok: true; intent: PendingChatIntent }
  | { ok: false; error: Error; intent?: PendingChatIntent }

interface StartRequest {
  kind: ChatIntentKind
  target: ChatIntentTarget
  text: string
  attachments: ChatAttachment[]
  requestedSkills: RequestedSkillSelection[]
  replaceFromMessageId: string
  modelId: string
  reasoningEffort: string
  workspaceTargetId: string
}

const TERMINAL_GENERATION_LIMIT = 512

function normalize(value: string | null | undefined): string {
  return value?.trim() ?? ''
}

function copyAttachments(attachments: ChatAttachment[] | undefined): ChatAttachment[] {
  return (attachments ?? []).map(attachment => ({ ...attachment }))
}

function copySkills(skills: RequestedSkillSelection[] | undefined): RequestedSkillSelection[] {
  return (skills ?? []).map(skill => ({ ...skill }))
}

function scopedStreamKey(botId: string, sessionId: string, streamId: string): string {
  return `${normalize(botId)}\u0000${normalize(sessionId)}\u0000${normalize(streamId)}`
}

function intentSnapshot(intent: PendingChatIntent): PendingChatIntent {
  return {
    ...intent,
    attachments: copyAttachments(intent.attachments),
    requestedSkills: copySkills(intent.requestedSkills),
  }
}

function optionalWireValue(value: string): string | undefined {
  return value || undefined
}

// Owns outgoing chat operations until a runtime generation claims them. It does
// not interpret runtime snapshots or fetch history; projection and settle call
// back into this registry with facts produced by their respective layers.
export function createChatIntents(deps: ChatIntentsDeps) {
  const pending = new Map<string, PendingChatIntent>()
  const terminalGenerations = new Map<string, Set<string>>()

  function pendingFor(botId: string, sessionId: string, streamId: string): PendingChatIntent | undefined {
    return pending.get(scopedStreamKey(botId, sessionId, streamId))
  }

  function rememberTerminalGeneration(handle: ChatRunHandle) {
    const generation = normalize(handle.generation)
    if (!generation) return
    const key = scopedStreamKey(handle.botId, handle.sessionId, handle.streamId)
    const generations = terminalGenerations.get(key) ?? new Set<string>()
    terminalGenerations.delete(key)
    generations.add(generation)
    terminalGenerations.set(key, generations)
    while (terminalGenerations.size > TERMINAL_GENERATION_LIMIT) {
      const oldest = terminalGenerations.keys().next().value
      if (oldest) terminalGenerations.delete(oldest)
      else break
    }
  }

  function wasTerminal(handle: ChatRunHandle): boolean {
    return terminalGenerations
      .get(scopedStreamKey(handle.botId, handle.sessionId, handle.streamId))
      ?.has(normalize(handle.generation)) === true
  }

  function wireForStart(request: StartRequest, streamId: string): ChatIntentWireMessage {
    const common = {
      model_id: optionalWireValue(request.modelId),
      reasoning_effort: optionalWireValue(request.reasoningEffort),
      workspace_target_id: optionalWireValue(request.workspaceTargetId),
    }
    if (request.kind === 'send') {
      return {
        type: 'message',
        stream_id: streamId,
        invocation_id: streamId,
        composer_scope: normalize(request.target.composerScope)
          || `${normalize(request.target.botId)}:${normalize(request.target.viewId)}`,
        text: request.text,
        session_id: optionalWireValue(normalize(request.target.sessionId)),
        attachments: request.attachments.length ? copyAttachments(request.attachments) : undefined,
        requested_skills: request.requestedSkills.length
          ? request.requestedSkills.map(skill => ({ name: normalize(skill.name) })).filter(skill => skill.name)
          : undefined,
        ...common,
      }
    }
    if (request.kind === 'retry') {
      return {
        type: 'retry_message',
        stream_id: streamId,
        session_id: normalize(request.target.sessionId),
        message_id: request.replaceFromMessageId,
        ...common,
      }
    }
    return {
      type: 'edit_message',
      stream_id: streamId,
      session_id: normalize(request.target.sessionId),
      message_id: request.replaceFromMessageId,
      text: request.text,
      ...common,
    }
  }

  function validateStart(request: StartRequest): Error | null {
    if (!normalize(request.target.botId)) return new Error('bot id is required')
    if (!normalize(request.target.viewId)) return new Error('view id is required')
    if (request.kind !== 'send' && !normalize(request.target.sessionId)) return new Error('session id is required')
    if (request.kind !== 'send' && !request.replaceFromMessageId) return new Error('message id is required')
    if (request.kind === 'edit' && !request.text) return new Error('edit text is required')
    if (request.kind === 'send' && !request.text && request.attachments.length === 0 && request.requestedSkills.length === 0) {
      return new Error('message content is required')
    }
    return null
  }

  function start(request: StartRequest): StartIntentResult {
    const validationError = validateStart(request)
    if (validationError) return { ok: false, error: validationError }
    const streamId = normalize(deps.createStreamId())
    if (!streamId) return { ok: false, error: new Error('stream id is required') }

    const botId = normalize(request.target.botId)
    const sessionId = normalize(request.target.sessionId)
    const viewId = normalize(request.target.viewId)
    const composerScope = normalize(request.target.composerScope) || `${botId}:${viewId}`
    const key = scopedStreamKey(botId, sessionId, streamId)
    if (pending.has(key)) return { ok: false, error: new Error('stream id is already pending') }

    let placeholder: IntentPlaceholder
    try {
      placeholder = deps.createPlaceholder({
        kind: request.kind,
        botId,
        sessionId,
        streamId,
        viewId,
        composerScope,
        text: request.text,
        attachments: copyAttachments(request.attachments),
        requestedSkills: copySkills(request.requestedSkills),
        replaceFromMessageId: request.replaceFromMessageId,
      })
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error('failed to create optimistic placeholder') }
    }
    if (!normalize(placeholder.assistantTurnKey)) {
      return { ok: false, error: new Error('assistant placeholder key is required') }
    }

    const intent: PendingChatIntent = {
      kind: request.kind,
      botId,
      sessionId,
      streamId,
      generation: '',
      viewId,
      composerScope,
      text: request.text,
      attachments: copyAttachments(request.attachments),
      requestedSkills: copySkills(request.requestedSkills),
      replaceFromMessageId: request.replaceFromMessageId,
      userTurnKey: normalize(placeholder.userTurnKey) || undefined,
      assistantTurnKey: normalize(placeholder.assistantTurnKey),
      abortRequested: false,
      cancelSent: false,
      abortSentGeneration: '',
    }
    pending.set(key, intent)

    try {
      if (deps.dispatch(botId, wireForStart(request, streamId))) return { ok: true, intent: intentSnapshot(intent) }
      throw new Error('chat transport is not connected')
    } catch (error) {
      pending.delete(key)
      deps.rollbackPlaceholder(intentSnapshot(intent))
      if (intent.kind === 'send') deps.restoreComposerDraft(draftRestoreRequest(intent))
      return {
        ok: false,
        error: error instanceof Error ? error : new Error('failed to dispatch chat intent'),
        intent: intentSnapshot(intent),
      }
    }
  }

  function send(input: SendChatIntentInput): StartIntentResult {
    return start({
      kind: 'send',
      target: input,
      text: normalize(input.text),
      attachments: copyAttachments(input.attachments),
      requestedSkills: copySkills(input.requestedSkills),
      replaceFromMessageId: '',
      modelId: normalize(input.modelId),
      reasoningEffort: normalize(input.reasoningEffort),
      workspaceTargetId: normalize(input.workspaceTargetId),
    })
  }

  function retry(input: RetryChatIntentInput): StartIntentResult {
    return start({
      kind: 'retry',
      target: input,
      text: '',
      attachments: [],
      requestedSkills: [],
      replaceFromMessageId: normalize(input.messageId),
      modelId: normalize(input.modelId),
      reasoningEffort: normalize(input.reasoningEffort),
      workspaceTargetId: normalize(input.workspaceTargetId),
    })
  }

  function edit(input: EditChatIntentInput): StartIntentResult {
    return start({
      kind: 'edit',
      target: input,
      text: normalize(input.text),
      attachments: [],
      requestedSkills: [],
      replaceFromMessageId: normalize(input.messageId),
      modelId: normalize(input.modelId),
      reasoningEffort: normalize(input.reasoningEffort),
      workspaceTargetId: normalize(input.workspaceTargetId),
    })
  }

  function abortRun(handle: ChatRunHandle): boolean {
    const normalized: ChatRunHandle = {
      botId: normalize(handle.botId),
      sessionId: normalize(handle.sessionId),
      streamId: normalize(handle.streamId),
      generation: normalize(handle.generation),
    }
    if (!normalized.botId || !normalized.sessionId || !normalized.streamId || !normalized.generation) return false
    const intent = pendingFor(normalized.botId, normalized.sessionId, normalized.streamId)
    if (intent?.generation && intent.generation !== normalized.generation) return false
    if (intent?.abortSentGeneration === normalized.generation) return true
    try {
      if (!deps.dispatch(normalized.botId, {
        type: 'abort',
        stream_id: normalized.streamId,
        session_id: normalized.sessionId,
        generation: normalized.generation,
      })) return false
    } catch {
      return false
    }
    if (intent) {
      intent.abortRequested = true
      intent.abortSentGeneration = normalized.generation
    }
    return true
  }

  // The server permits this only during the short pre-admission window. Keeping
  // it separate prevents generationless cancellation from becoming the normal
  // abort API once a reusable stream id has a concrete runtime generation.
  function cancelPendingSend(botId: string, sessionId: string, streamId: string): boolean {
    const intent = pendingFor(botId, sessionId, streamId)
    if (!intent || intent.generation) return false
    if (!intent.sessionId) {
      intent.abortRequested = true
      return true
    }
    if (intent.cancelSent) return true
    try {
      if (!deps.dispatch(intent.botId, {
        type: 'abort',
        stream_id: intent.streamId,
        session_id: intent.sessionId,
      })) return false
    } catch {
      return false
    }
    intent.abortRequested = true
    intent.cancelSent = true
    return true
  }

  function steer(handle: ChatRunHandle, text: string): boolean {
    const input = normalize(text)
    const botId = normalize(handle.botId)
    const sessionId = normalize(handle.sessionId)
    const streamId = normalize(handle.streamId)
    const generation = normalize(handle.generation)
    if (!botId || !sessionId || !streamId || !generation || !input) return false
    try {
      return deps.dispatch(botId, {
        type: 'steer_current_run',
        stream_id: streamId,
        session_id: sessionId,
        generation,
        text: input,
      })
    } catch {
      return false
    }
  }

  function claimRun(handle: ChatRunHandle, dispatchRequestedAbort = true): PendingChatIntent | null {
    const normalized: ChatRunHandle = {
      botId: normalize(handle.botId),
      sessionId: normalize(handle.sessionId),
      streamId: normalize(handle.streamId),
      generation: normalize(handle.generation),
    }
    if (!normalized.botId || !normalized.sessionId || !normalized.streamId || !normalized.generation) return null
    if (wasTerminal(normalized)) return null
    const intent = pendingFor(normalized.botId, normalized.sessionId, normalized.streamId)
    if (!intent) return null
    if (intent.generation && intent.generation !== normalized.generation) return null
    if (!intent.generation) {
      intent.generation = normalized.generation
      deps.onClaim?.(intentSnapshot(intent), normalized)
    }
    if (dispatchRequestedAbort && intent.abortRequested && intent.abortSentGeneration !== normalized.generation) abortRun(normalized)
    return intentSnapshot(intent)
  }

  function handleTransportClose(botId: string) {
    const bid = normalize(botId)
    if (!bid) return
    for (const intent of pending.values()) {
      if (intent.botId !== bid || !intent.abortRequested || !intent.generation) continue
      intent.abortSentGeneration = ''
    }
  }

  function rejectAbort(handle: ChatRunHandle) {
    const intent = pendingFor(handle.botId, handle.sessionId, handle.streamId)
    if (!intent) return
    const generation = normalize(handle.generation)
    if (generation && intent.generation && intent.generation !== generation) return
    intent.abortRequested = false
    intent.abortSentGeneration = ''
  }

  function rebindSession(botId: string, streamId: string, sessionId: string): PendingChatIntent | null {
    const bid = normalize(botId)
    const id = normalize(streamId)
    const sid = normalize(sessionId)
    if (!bid || !id || !sid) return null
    const unboundKey = scopedStreamKey(bid, '', id)
    const intent = pending.get(unboundKey)
    if (!intent) return null
    const nextKey = scopedStreamKey(bid, sid, id)
    if (pending.has(nextKey)) return null
    pending.delete(unboundKey)
    const previousSessionId = intent.sessionId
    intent.sessionId = sid
    pending.set(nextKey, intent)
    deps.onRebind?.(intentSnapshot(intent), previousSessionId)
    if (intent.abortRequested && !intent.cancelSent) cancelPendingSend(bid, sid, id)
    return intentSnapshot(intent)
  }

  function draftRestoreRequest(intent: PendingChatIntent): RestoreComposerDraftRequest {
    return {
      botId: intent.botId,
      sessionId: intent.sessionId,
      viewId: intent.viewId,
      composerScope: intent.composerScope,
      text: intent.text,
      attachments: copyAttachments(intent.attachments),
      requestedSkills: copySkills(intent.requestedSkills),
    }
  }

  function terminal(
    result: IntentTerminalResult,
    options: { effectsApplied?: boolean } = {},
  ): PendingChatIntent | null {
    const handle: ChatRunHandle = {
      botId: normalize(result.botId),
      sessionId: normalize(result.sessionId),
      streamId: normalize(result.streamId),
      generation: normalize(result.generation),
    }
    // A terminal checkpoint may be the first event carrying the generation.
    // Bind it without replaying a pending abort against a run that already ended.
    const intent = claimRun(handle, false)
    rememberTerminalGeneration(handle)
    if (!intent) return null
    pending.delete(scopedStreamKey(handle.botId, handle.sessionId, handle.streamId))
    const settledIntent = intentSnapshot(intent)
    if (result.persistence === 'persisted') {
      deps.onPersisted?.(settledIntent, { ...result, ...handle, stableIds: [...(result.stableIds ?? [])] })
      return settledIntent
    }

    if (result.persistence === 'vanished' && !options.effectsApplied) {
      deps.removeVanished(settledIntent)
      if (settledIntent.kind === 'send') {
        deps.restoreComposerDraft(draftRestoreRequest(settledIntent))
      } else {
        deps.restoreReplacement(settledIntent)
      }
    }
    return settledIntent
  }

  function failPending(botId: string, sessionId: string, streamId: string): PendingChatIntent | null {
    const intent = pendingFor(botId, sessionId, streamId)
    if (!intent || intent.generation) return null
    pending.delete(scopedStreamKey(intent.botId, intent.sessionId, intent.streamId))
    const failed = intentSnapshot(intent)
    deps.rollbackPlaceholder(failed)
    if (failed.kind === 'send') {
      deps.restoreComposerDraft(draftRestoreRequest(failed))
    } else {
      deps.restoreReplacement(failed)
    }
    return failed
  }

  function get(botId: string, sessionId: string, streamId: string): PendingChatIntent | null {
    const intent = pendingFor(botId, sessionId, streamId)
    return intent ? intentSnapshot(intent) : null
  }

  function activeForTarget(target: Pick<ChatIntentTarget, 'botId' | 'sessionId'>): PendingChatIntent[] {
    const botId = normalize(target.botId)
    const sessionId = normalize(target.sessionId)
    return [...pending.values()]
      .filter(intent => intent.botId === botId && intent.sessionId === sessionId)
      .map(intentSnapshot)
  }

  function discardTarget(target: Pick<ChatIntentTarget, 'botId' | 'sessionId'>): PendingChatIntent[] {
    const intents = activeForTarget(target)
    for (const intent of intents) {
      pending.delete(scopedStreamKey(intent.botId, intent.sessionId, intent.streamId))
    }
    return intents
  }

  function clear() {
    pending.clear()
    terminalGenerations.clear()
  }

  return {
    send,
    retry,
    edit,
    abortRun,
    cancelPendingSend,
    steer,
    claimRun,
    handleTransportClose,
    rejectAbort,
    rebindSession,
    terminal,
    failPending,
    get,
    activeForTarget,
    discardTarget,
    clear,
  }
}
