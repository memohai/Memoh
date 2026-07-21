import { ref, type Ref } from 'vue'
import type { AcpagentRuntimeStatus } from '@memohai/sdk'
import {
  ensureACPRuntime as requestEnsureACPRuntime,
  setACPRuntimeModel as requestSetACPRuntimeModel,
  setACPRuntimeReasoning as requestSetACPRuntimeReasoning,
} from '@/composables/api/useChat'

export interface ACPRuntimeStatusRegistry {
  acpRuntimeStatuses: Ref<Record<string, AcpagentRuntimeStatus | undefined>>
  acpRuntimeKey: (botId: string, sessionId: string) => string
  setACPRuntimeStatus: (botId: string, sessionId: string, runtime: AcpagentRuntimeStatus | undefined) => void
  clearACPRuntimeStatus: (botId: string, sessionId: string) => void
}

export interface ACPRuntimeRegistryTransport {
  ensureACPRuntime: typeof requestEnsureACPRuntime
  setACPRuntimeModel: typeof requestSetACPRuntimeModel
  setACPRuntimeReasoning: typeof requestSetACPRuntimeReasoning
}

interface ACPRuntimeRegistryDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
}

const defaultTransport: ACPRuntimeRegistryTransport = {
  ensureACPRuntime: requestEnsureACPRuntime,
  setACPRuntimeModel: requestSetACPRuntimeModel,
  setACPRuntimeReasoning: requestSetACPRuntimeReasoning,
}

export function createACPRuntimeRegistry(
  { currentBotId, sessionId }: ACPRuntimeRegistryDeps,
  transport: ACPRuntimeRegistryTransport = defaultTransport,
) {
  const acpRuntimeStatuses = ref<Record<string, AcpagentRuntimeStatus | undefined>>({})
  const acpRuntimePending = ref<Record<string, boolean>>({})
  const requests = new Map<string, Promise<AcpagentRuntimeStatus>>()
  const statusVersions = new Map<string, number>()
  let registryGeneration = 0

  function acpRuntimeKey(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    return bid && sid ? `${bid}:${sid}` : ''
  }

  function setACPRuntimeStatus(botId: string, targetSessionId: string, runtime: AcpagentRuntimeStatus | undefined) {
    const key = acpRuntimeKey(botId, targetSessionId)
    if (!key) return
    const next = { ...acpRuntimeStatuses.value }
    if (runtime) next[key] = runtime
    else delete next[key]
    acpRuntimeStatuses.value = next
  }

  function setACPRuntimePending(botId: string, targetSessionId: string, pending: boolean) {
    const key = acpRuntimeKey(botId, targetSessionId)
    if (!key) return
    const next = { ...acpRuntimePending.value }
    if (pending) next[key] = true
    else delete next[key]
    acpRuntimePending.value = next
  }

  function clearACPRuntimeStatus(botId: string, targetSessionId: string) {
    const key = acpRuntimeKey(botId, targetSessionId)
    if (!key) return
    requests.delete(key)
    statusVersions.set(key, (statusVersions.get(key) ?? 0) + 1)
    setACPRuntimeStatus(botId, targetSessionId, undefined)
    setACPRuntimePending(botId, targetSessionId, false)
  }

  async function ensureACPRuntimeFor(botID: string, sessionID: string): Promise<AcpagentRuntimeStatus> {
    const bid = botID.trim()
    const sid = sessionID.trim()
    if (!bid || !sid) throw new Error('ACP session is not selected')
    const key = acpRuntimeKey(bid, sid)
    const existing = requests.get(key)
    if (existing) return existing

    const generation = registryGeneration
    const statusVersion = statusVersions.get(key) ?? 0
    setACPRuntimePending(bid, sid, true)
    const request = transport.ensureACPRuntime(bid, sid)
      .then((runtime) => {
        if (
          registryGeneration === generation
          && (statusVersions.get(key) ?? 0) === statusVersion
          && requests.get(key) === request
        ) {
          setACPRuntimeStatus(bid, sid, runtime)
        }
        return runtime
      })
      .finally(() => {
        if (requests.get(key) !== request) return
        requests.delete(key)
        setACPRuntimePending(bid, sid, false)
      })
    requests.set(key, request)
    return request
  }

  async function ensureACPRuntime(sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value?.trim() ?? ''
    const sid = sessionID?.trim() || sessionId.value?.trim() || ''
    return ensureACPRuntimeFor(bid, sid)
  }

  async function updateACPRuntimeFor(
    botID: string,
    sessionID: string,
    update: (botId: string, sessionId: string) => Promise<AcpagentRuntimeStatus>,
  ): Promise<AcpagentRuntimeStatus> {
    const bid = botID.trim()
    const sid = sessionID.trim()
    if (!bid || !sid) throw new Error('ACP session is not selected')
    const key = acpRuntimeKey(bid, sid)
    const generation = registryGeneration
    const statusVersion = (statusVersions.get(key) ?? 0) + 1
    statusVersions.set(key, statusVersion)
    const runtime = await update(bid, sid)
    if (registryGeneration === generation && statusVersions.get(key) === statusVersion) {
      setACPRuntimeStatus(bid, sid, runtime)
    }
    return runtime
  }

  async function setACPRuntimeModelFor(botID: string, sessionID: string, modelID: string): Promise<AcpagentRuntimeStatus> {
    const mid = modelID.trim()
    if (!mid) throw new Error('ACP model is not selected')
    return updateACPRuntimeFor(botID, sessionID, (bid, sid) => transport.setACPRuntimeModel(bid, sid, mid))
  }

  async function setACPRuntimeModel(modelID: string, sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value?.trim() ?? ''
    const sid = sessionID?.trim() || sessionId.value?.trim() || ''
    return setACPRuntimeModelFor(bid, sid, modelID)
  }

  async function setACPRuntimeReasoningFor(botID: string, sessionID: string, effort: string): Promise<AcpagentRuntimeStatus> {
    const value = effort.trim()
    if (!value) throw new Error('ACP reasoning effort is not selected')
    return updateACPRuntimeFor(botID, sessionID, (bid, sid) => transport.setACPRuntimeReasoning(bid, sid, value))
  }

  async function setACPRuntimeReasoning(effort: string, sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value?.trim() ?? ''
    const sid = sessionID?.trim() || sessionId.value?.trim() || ''
    return setACPRuntimeReasoningFor(bid, sid, effort)
  }

  function resetACPRuntimeRegistry() {
    registryGeneration += 1
    requests.clear()
    statusVersions.clear()
    acpRuntimeStatuses.value = {}
    acpRuntimePending.value = {}
  }

  return {
    acpRuntimeStatuses,
    acpRuntimePending,
    acpRuntimeKey,
    setACPRuntimeStatus,
    clearACPRuntimeStatus,
    ensureACPRuntimeFor,
    ensureACPRuntime,
    setACPRuntimeModelFor,
    setACPRuntimeModel,
    setACPRuntimeReasoningFor,
    setACPRuntimeReasoning,
    resetACPRuntimeRegistry,
  }
}
