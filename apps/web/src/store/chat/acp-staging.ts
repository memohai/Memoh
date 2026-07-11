import { computed, ref, type Ref } from 'vue'
import type { AcpagentRuntimeStatus } from '@memohai/sdk'
import {
  createACPRuntime as requestCreateACPRuntime,
  closeACPRuntime as requestCloseACPRuntime,
  setACPRuntimeModelByID as requestSetACPRuntimeModelByID,
} from '@/composables/api/useChat'
import { ACP_DEFAULT_PROJECT_MODE, ACP_DEFAULT_PROJECT_PATH } from '@/utils/acp'
import type { ACPRuntimeStatusRegistry } from './acp-runtime-registry'
import type { ACPAgentSessionInput } from './types'

// Pending-ACP session staging — the state machine behind the "draft composer
// pointed at an ACP agent" flow: staging an agent before any session exists,
// warming a runtime for it, switching its model, and handing the warm runtime
// over to the real session on first send.
//
// This factory calls transports directly (createACPRuntime / closeACPRuntime /
// setACPRuntimeModelByID) — an exception to the "factories don't touch
// transports" rule, acceptable because tests mock the API module by path and
// this file imports the exact same module. Everything that mutates the
// session list or transcript is injected as a callback instead.

interface PendingACPStageSnapshot {
  botId: string
  generation: number
  identityKey: string
  runtimeId: string
  modelId: string
}

export function acpSessionMetadata(input: ACPAgentSessionInput): Record<string, unknown> {
  const agentId = input.agentId.trim()
  const projectMode = input.projectMode?.trim() || ACP_DEFAULT_PROJECT_MODE
  const projectPath = input.projectPath?.trim() || ACP_DEFAULT_PROJECT_PATH
  return {
    acp_agent_id: agentId,
    project_path: projectPath,
    acp_project_mode: projectMode,
  }
}

export interface ACPStagingDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  draftIntent: Ref<boolean>
  explicitSessionSelection: Ref<boolean>
  // Shared by staged runtimes and real session runtimes, but owned by one
  // registry so neither flow can mutate its lookup containers directly.
  runtimeRegistry: ACPRuntimeStatusRegistry
  // Invalidates any in-flight selectSession/hydration work in the store.
  bumpSelectSessionRequest: () => void
  // Stops the per-session SSE stream and empties the transcript, resetting
  // pagination to the "no history" draft posture.
  clearTranscriptForDraft: () => void
}

export function createACPStaging(deps: ACPStagingDeps) {
  const {
    currentBotId,
    sessionId,
    draftIntent,
    explicitSessionSelection,
    runtimeRegistry,
    bumpSelectSessionRequest,
    clearTranscriptForDraft,
  } = deps
  const {
    acpRuntimeStatuses,
    acpRuntimeKey,
    setACPRuntimeStatus,
    clearACPRuntimeStatus,
  } = runtimeRegistry

  const pendingACPSessionInput = ref<ACPAgentSessionInput | null>(null)
  const defaultACPInputsByBot = new Map<string, ACPAgentSessionInput | null>()
  // Server-generated ID of the staged runtime; the client never invents
  // runtime identifiers.
  const pendingACPRuntimeId = ref('')
  const pendingACPBotId = ref('')
  const pendingACPCreating = ref(false)
  let pendingACPCreateRequest: Promise<AcpagentRuntimeStatus | undefined> | null = null
  let pendingACPCreateKey = ''
  let pendingACPGeneration = 0
  let pendingACPModelRequestVersion = 0

  const pendingACPSessionMetadata = computed<Record<string, unknown> | null>(() =>
    pendingACPSessionInput.value ? acpSessionMetadata(pendingACPSessionInput.value) : null,
  )
  const pendingACPModelId = computed(() => pendingACPSessionInput.value?.modelId?.trim() ?? '')
  const pendingACPRuntimeStatus = computed(() => {
    const bid = pendingACPBotId.value
    const rid = pendingACPRuntimeId.value
    const key = acpRuntimeKey(bid, rid)
    return key ? acpRuntimeStatuses.value[key] : undefined
  })
  const pendingACPRuntimeEnsuring = computed(() => pendingACPCreating.value)

  function cloneACPInput(input: ACPAgentSessionInput): ACPAgentSessionInput {
    return { ...input }
  }

  function rememberDefaultACPInput(botId: string, input: ACPAgentSessionInput | null) {
    const bid = botId.trim()
    if (!bid) return
    defaultACPInputsByBot.set(bid, input ? cloneACPInput(input) : null)
  }

  function cachedDefaultACPInput(botId: string): { loaded: boolean, input: ACPAgentSessionInput | null } {
    const bid = botId.trim()
    if (!defaultACPInputsByBot.has(bid)) return { loaded: false, input: null }
    const input = defaultACPInputsByBot.get(bid) ?? null
    return { loaded: true, input: input ? cloneACPInput(input) : null }
  }

  function cacheDefaultACPSession(input: ACPAgentSessionInput | null) {
    rememberDefaultACPInput(currentBotId.value ?? '', input)
  }

  function pendingACPIdentityKey(botId: string, input: ACPAgentSessionInput): string {
    return [botId, input.sessionMode ?? 'chat', input.agentId, input.projectPath ?? '', input.projectMode ?? ''].join('\u0000')
  }

  function pendingACPStagingKey(snapshot: Pick<PendingACPStageSnapshot, 'identityKey' | 'generation'>): string {
    return `${snapshot.generation}\u0000${snapshot.identityKey}`
  }

  function nextPendingACPGeneration() {
    pendingACPGeneration += 1
  }

  function clearPendingACPCreateTracking() {
    pendingACPCreateRequest = null
    pendingACPCreateKey = ''
    pendingACPCreating.value = false
  }

  function closeStagedRuntime(botId: string, runtimeId: string) {
    const bid = botId.trim()
    const rid = runtimeId.trim()
    if (!bid || !rid) return
    void requestCloseACPRuntime(bid, rid).catch(() => {})
    clearACPRuntimeStatus(bid, rid)
  }

  function capturePendingACPStage(): PendingACPStageSnapshot | null {
    const botId = pendingACPBotId.value
    const pending = pendingACPSessionInput.value
    if (!botId || !pending) return null
    return {
      botId,
      generation: pendingACPGeneration,
      identityKey: pendingACPIdentityKey(botId, pending),
      runtimeId: pendingACPRuntimeId.value,
      modelId: pending.modelId?.trim() ?? '',
    }
  }

  function isPendingACPStageCurrent(snapshot: PendingACPStageSnapshot, modelId?: string, modelRequestVersion?: number): boolean {
    const current = capturePendingACPStage()
    if (!current) return false
    return current.botId === snapshot.botId
      && current.generation === snapshot.generation
      && current.identityKey === snapshot.identityKey
      && (modelId === undefined || current.modelId === modelId)
      && (modelRequestVersion === undefined || pendingACPModelRequestVersion === modelRequestVersion)
  }

  function stageACPSession(input: ACPAgentSessionInput, options: { explicitSelection?: boolean } = {}) {
    const ownerBotId = (currentBotId.value ?? '').trim()
    const metadata = acpSessionMetadata(input)
    const existing = pendingACPSessionInput.value
    const samePendingAgent = Boolean(existing
      && pendingACPBotId.value === ownerBotId
      && existing.agentId === metadata.acp_agent_id
      && (existing.sessionMode || 'chat') === (input.sessionMode || 'chat')
      && (existing.projectPath || ACP_DEFAULT_PROJECT_PATH) === metadata.project_path
      && (existing.projectMode || ACP_DEFAULT_PROJECT_MODE) === metadata.acp_project_mode)
    if (!samePendingAgent) {
      nextPendingACPGeneration()
      pendingACPModelRequestVersion += 1
      clearPendingACPCreateTracking()
    }
    const previousOwnerBotId = pendingACPBotId.value
    pendingACPBotId.value = ownerBotId
    pendingACPSessionInput.value = {
      ...input,
      agentId: String(metadata.acp_agent_id ?? ''),
      projectPath: String(metadata.project_path ?? ''),
      projectMode: String(metadata.acp_project_mode ?? ''),
      modelId: input.modelId?.trim() || (samePendingAgent ? existing?.modelId : '') || '',
    }
    if (!samePendingAgent && pendingACPRuntimeId.value) {
      const bid = previousOwnerBotId
      const runtimeId = pendingACPRuntimeId.value
      pendingACPRuntimeId.value = ''
      closeStagedRuntime(bid, runtimeId)
    }
    explicitSessionSelection.value = options.explicitSelection !== false
  }

  function stageDefaultACPSession(input: ACPAgentSessionInput) {
    rememberDefaultACPInput(currentBotId.value ?? '', input)
    bumpSelectSessionRequest()
    explicitSessionSelection.value = false
    draftIntent.value = false
    sessionId.value = null
    clearTranscriptForDraft()
    stageACPSession(input, { explicitSelection: false })
  }

  function stageNewACPSession(input: ACPAgentSessionInput) {
    bumpSelectSessionRequest()
    clearPendingACPSession()
    sessionId.value = null
    draftIntent.value = true
    clearTranscriptForDraft()
    stageACPSession(input, { explicitSelection: true })
  }

  function resetToEmptyComposer(options: { clearPendingACP?: boolean; explicitSelection?: boolean; draftIntent?: boolean } = {}) {
    bumpSelectSessionRequest()
    if (options.clearPendingACP !== false) {
      clearPendingACPSession()
    }
    sessionId.value = null
    explicitSessionSelection.value = options.explicitSelection === true
    draftIntent.value = options.draftIntent ?? options.explicitSelection === true
    clearTranscriptForDraft()
  }

  async function ensurePendingACPRuntime(): Promise<AcpagentRuntimeStatus | undefined> {
    const snapshot = capturePendingACPStage()
    const pending = pendingACPSessionInput.value
    if (!snapshot || !pending) return undefined
    if (snapshot.runtimeId) {
      const key = acpRuntimeKey(snapshot.botId, snapshot.runtimeId)
      return acpRuntimeStatuses.value[key]
    }
    const stagingKey = pendingACPStagingKey(snapshot)
    if (pendingACPCreateRequest && pendingACPCreateKey === stagingKey) return pendingACPCreateRequest

    pendingACPCreating.value = true
    const request = requestCreateACPRuntime(snapshot.botId, {
      agentId: pending.agentId,
      projectPath: pending.projectPath,
    })
      .then((runtime) => {
        const rid = runtime?.runtime_id?.trim() ?? ''
        const current = capturePendingACPStage()
        const stillStaged = !!current
          && pendingACPStagingKey(current) === stagingKey
          && !current.runtimeId
        if (stillStaged && rid) {
          pendingACPRuntimeId.value = rid
          setACPRuntimeStatus(snapshot.botId, rid, runtime)
        } else if (rid) {
          // Staging changed while the runtime was starting: discard it.
          closeStagedRuntime(snapshot.botId, rid)
        }
        return runtime
      })
      .catch((error) => {
        if (!isPendingACPStageCurrent(snapshot)) return undefined
        throw error
      })
      .finally(() => {
        if (pendingACPCreateRequest === request) {
          clearPendingACPCreateTracking()
        }
      })
    pendingACPCreateRequest = request
    pendingACPCreateKey = stagingKey
    return request
  }

  async function setPendingACPModel(modelId: string) {
    if (!pendingACPSessionInput.value) return
    const mid = modelId.trim()
    const previousModelId = pendingACPSessionInput.value.modelId?.trim() ?? ''
    if (mid === previousModelId) return

    pendingACPSessionInput.value = {
      ...pendingACPSessionInput.value,
      modelId: mid,
    }

    const initialSnapshot = capturePendingACPStage()
    if (!initialSnapshot) return
    const requestVersion = ++pendingACPModelRequestVersion

    try {
      const runtimeId = await pendingACPModelRuntime(initialSnapshot, mid, requestVersion)
      if (!runtimeId) return
      await setPendingACPModelOnRuntime(initialSnapshot, runtimeId, mid, requestVersion)
    } catch (error) {
      if (!isPendingACPStageCurrent(initialSnapshot, mid, requestVersion)) return
      if (pendingACPSessionInput.value?.modelId?.trim() === mid) {
        pendingACPSessionInput.value = {
          ...pendingACPSessionInput.value,
          modelId: previousModelId,
        }
      }
      throw error
    }
  }

  async function pendingACPModelRuntime(snapshot: PendingACPStageSnapshot, modelId: string, requestVersion: number): Promise<string> {
    const current = capturePendingACPStage()
    if (!current || !isPendingACPStageCurrent(snapshot, modelId, requestVersion)) return ''
    if (current.runtimeId || !modelId) return current.runtimeId
    await ensurePendingACPRuntime()
    if (!isPendingACPStageCurrent(snapshot, modelId, requestVersion)) return ''
    return capturePendingACPStage()?.runtimeId ?? ''
  }

  async function setPendingACPModelOnRuntime(snapshot: PendingACPStageSnapshot, runtimeId: string, modelId: string, requestVersion: number) {
    try {
      const runtime = await requestSetACPRuntimeModelByID(snapshot.botId, runtimeId, modelId)
      if (!isPendingACPStageCurrent(snapshot, modelId, requestVersion)) return
      setACPRuntimeStatus(snapshot.botId, runtimeId, runtime)
    } catch (error) {
      if (!isPendingACPStageCurrent(snapshot, modelId, requestVersion)) return
      if (!isRuntimeNotFoundError(error)) throw error
      if (pendingACPRuntimeId.value !== runtimeId) return

      clearACPRuntimeStatus(snapshot.botId, runtimeId)
      pendingACPRuntimeId.value = ''

      const freshId = await pendingACPModelRuntime(snapshot, modelId, requestVersion)
      if (!freshId) return
      const runtime = await requestSetACPRuntimeModelByID(snapshot.botId, freshId, modelId)
      if (!isPendingACPStageCurrent(snapshot, modelId, requestVersion)) return
      setACPRuntimeStatus(snapshot.botId, freshId, runtime)
    }
  }

  // The runtime endpoints fail closed with this fixed message when the
  // referenced runtime is gone (idle-reaped or never existed).
  function isRuntimeNotFoundError(error: unknown): boolean {
    if (!error || typeof error !== 'object') return false
    const message = (error as { message?: unknown }).message
    return typeof message === 'string' && message.includes('runtime not found')
  }

  function clearPendingACPSession() {
    const bid = pendingACPBotId.value
    const runtimeId = pendingACPRuntimeId.value
    nextPendingACPGeneration()
    pendingACPModelRequestVersion += 1
    clearPendingACPCreateTracking()
    closeStagedRuntime(bid, runtimeId)
    pendingACPSessionInput.value = null
    pendingACPRuntimeId.value = ''
    pendingACPBotId.value = ''
  }

  // Detaches the staged ACP session without closing its warm runtime, so the
  // first send can bind the runtime to the real session.
  function detachPendingACPSession(): { input: ACPAgentSessionInput; runtimeId: string; botId: string } | null {
    const pending = pendingACPSessionInput.value
    if (!pending) return null
    const runtimeId = pendingACPRuntimeId.value
    const botId = pendingACPBotId.value
    nextPendingACPGeneration()
    pendingACPModelRequestVersion += 1
    clearPendingACPCreateTracking()
    pendingACPSessionInput.value = null
    pendingACPRuntimeId.value = ''
    pendingACPBotId.value = ''
    return { input: { ...pending }, runtimeId, botId }
  }

  function restorePendingACPSession(input: ACPAgentSessionInput, runtimeId: string, botId: string) {
    pendingACPSessionInput.value = { ...input }
    pendingACPRuntimeId.value = runtimeId.trim()
    pendingACPBotId.value = botId.trim()
  }

  function releasePendingACPSession() {
    nextPendingACPGeneration()
    pendingACPModelRequestVersion += 1
    clearPendingACPCreateTracking()
    pendingACPSessionInput.value = null
    pendingACPRuntimeId.value = ''
    pendingACPBotId.value = ''
  }

  function pendingACPMatchesInput(input: ACPAgentSessionInput): boolean {
    const pending = pendingACPSessionInput.value
    if (!pending || sessionId.value) return false
    const metadata = acpSessionMetadata(input)
    return pending.agentId === metadata.acp_agent_id
      && (pending.sessionMode || 'chat') === (input.sessionMode || 'chat')
      && (pending.projectPath || ACP_DEFAULT_PROJECT_PATH) === metadata.project_path
      && (pending.projectMode || ACP_DEFAULT_PROJECT_MODE) === metadata.acp_project_mode
  }

  return {
    pendingACPSessionInput,
    pendingACPRuntimeId,
    pendingACPSessionMetadata,
    pendingACPModelId,
    pendingACPRuntimeStatus,
    pendingACPRuntimeEnsuring,
    rememberDefaultACPInput,
    cachedDefaultACPInput,
    cacheDefaultACPSession,
    stageACPSession,
    stageDefaultACPSession,
    stageNewACPSession,
    resetToEmptyComposer,
    ensurePendingACPRuntime,
    setPendingACPModel,
    clearPendingACPSession,
    detachPendingACPSession,
    restorePendingACPSession,
    releasePendingACPSession,
    pendingACPMatchesInput,
  }
}
