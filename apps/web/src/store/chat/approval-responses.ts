export type ApprovalResponseOutcome = 'succeeded' | 'failed' | 'canceled'

export interface ApprovalResponse {
  readonly streamId: string
  readonly approvalId: string
  readonly botId: string
  readonly sessionId: string
  readonly silent: boolean
}

export interface BeginApprovalResponseInput {
  streamId: string
  approvalId: string
  botId: string
  sessionId: string
  silent: boolean
}

interface PendingApprovalResponse extends ApprovalResponse {
  startedAt: number
}

interface ApprovalResponseTrackerDeps {
  rollbackApproval: (approvalId: string) => void
  now?: () => number
  ttlMs?: number
  terminalHistoryLimit?: number
}

const DEFAULT_TTL_MS = 2 * 60 * 1000
const DEFAULT_TERMINAL_HISTORY_LIMIT = 512

export function createApprovalResponseTracker({
  rollbackApproval,
  now = Date.now,
  ttlMs = DEFAULT_TTL_MS,
  terminalHistoryLimit = DEFAULT_TERMINAL_HISTORY_LIMIT,
}: ApprovalResponseTrackerDeps) {
  const responses = new Map<string, PendingApprovalResponse>()
  const terminalResponseIds = new Set<string>()

  function rememberTerminalResponse(streamId: string) {
    terminalResponseIds.add(streamId)
    if (terminalResponseIds.size <= terminalHistoryLimit) return
    const oldest = terminalResponseIds.values().next().value
    if (oldest) terminalResponseIds.delete(oldest)
  }

  function expireStaleResponses() {
    const currentTime = now()
    for (const [streamId, response] of responses) {
      if (currentTime - response.startedAt < ttlMs) continue
      settleApprovalResponse(streamId, 'failed')
    }
  }

  function hasPendingApprovalResponse(approvalId: string): boolean {
    const id = approvalId.trim()
    if (!id) return false
    expireStaleResponses()
    for (const response of responses.values()) {
      if (response.approvalId === id) return true
    }
    return false
  }

  function beginApprovalResponse(input: BeginApprovalResponseInput): boolean {
    const streamId = input.streamId.trim()
    const approvalId = input.approvalId.trim()
    const botId = input.botId.trim()
    const sessionId = input.sessionId.trim()
    if (!streamId || !approvalId || !botId || !sessionId) return false
    expireStaleResponses()
    if (responses.has(streamId) || hasPendingApprovalResponse(approvalId)) return false
    if (terminalResponseIds.has(streamId)) return false
    responses.set(streamId, {
      streamId,
      approvalId,
      botId,
      sessionId,
      silent: input.silent,
      startedAt: now(),
    })
    return true
  }

  function getApprovalResponse(streamId: string): ApprovalResponse | undefined {
    return responses.get(streamId.trim())
  }

  function settleApprovalResponse(streamId: string, outcome: ApprovalResponseOutcome): ApprovalResponse | undefined {
    const id = streamId.trim()
    const response = responses.get(id)
    if (!response) return undefined
    responses.delete(id)
    rememberTerminalResponse(id)
    if (outcome === 'failed') rollbackApproval(response.approvalId)
    return response
  }

  function pendingApprovalResponses(): ApprovalResponse[] {
    return [...responses.values()]
  }

  function pendingApprovalResponsesForSession(botId: string, sessionId: string): ApprovalResponse[] {
    const bid = botId.trim()
    const sid = sessionId.trim()
    if (!bid || !sid) return []
    return pendingApprovalResponses().filter(response => response.botId === bid && response.sessionId === sid)
  }

  function discardAllApprovalResponses(): ApprovalResponse[] {
    const pending = pendingApprovalResponses()
    for (const response of pending) settleApprovalResponse(response.streamId, 'canceled')
    return pending
  }

  function isTerminalApprovalResponse(streamId: string | undefined): boolean {
    const id = streamId?.trim()
    return Boolean(id && terminalResponseIds.has(id))
  }

  function resetApprovalResponses() {
    responses.clear()
    terminalResponseIds.clear()
  }

  return {
    hasPendingApprovalResponse,
    beginApprovalResponse,
    getApprovalResponse,
    settleApprovalResponse,
    pendingApprovalResponses,
    pendingApprovalResponsesForSession,
    discardAllApprovalResponses,
    isTerminalApprovalResponse,
    resetApprovalResponses,
  }
}
