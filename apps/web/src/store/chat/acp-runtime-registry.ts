import { ref, type Ref } from 'vue'
import type { AcpagentRuntimeStatus } from '@memohai/sdk'
import {
  ensureACPRuntime as requestEnsureACPRuntime,
  setACPRuntimeModel as requestSetACPRuntimeModel,
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
}

interface ACPRuntimeRegistryDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
}

const defaultTransport: ACPRuntimeRegistryTransport = {
  ensureACPRuntime: requestEnsureACPRuntime,
  setACPRuntimeModel: requestSetACPRuntimeModel,
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

  async function ensureACPRuntime(sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value?.trim() ?? ''
    const sid = sessionID?.trim() || sessionId.value?.trim() || ''
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

  async function setACPRuntimeModel(modelID: string, sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value?.trim() ?? ''
    const sid = sessionID?.trim() || sessionId.value?.trim() || ''
    const mid = modelID.trim()
    if (!bid || !sid || !mid) throw new Error('ACP model is not selected')
    const key = acpRuntimeKey(bid, sid)
    const generation = registryGeneration
    const statusVersion = (statusVersions.get(key) ?? 0) + 1
    statusVersions.set(key, statusVersion)
    const runtime = await transport.setACPRuntimeModel(bid, sid, mid)
    if (registryGeneration === generation && statusVersions.get(key) === statusVersion) {
      setACPRuntimeStatus(bid, sid, runtime)
    }
    return runtime
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
    ensureACPRuntime,
    setACPRuntimeModel,
    resetACPRuntimeRegistry,
  }
}
