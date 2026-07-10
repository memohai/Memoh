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
}

const DEFAULT_TTL_MS = 2 * 60 * 1000

export function createApprovalResponseTracker({
  rollbackApproval,
  now = Date.now,
  ttlMs = DEFAULT_TTL_MS,
}: ApprovalResponseTrackerDeps) {
  const responses = new Map<string, PendingApprovalResponse>()

  function expireStaleResponses() {
    const currentTime = now()
    for (const [streamId, response] of responses) {
      if (currentTime - response.startedAt < ttlMs) continue
      rollbackApproval(response.approvalId)
      responses.delete(streamId)
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
    if (responses.has(streamId) || hasPendingApprovalResponse(approvalId)) return false
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
    if (outcome === 'failed') rollbackApproval(response.approvalId)
    responses.delete(id)
    return response
  }

  function clearApprovalResponses() {
    responses.clear()
  }

  return {
    hasPendingApprovalResponse,
    beginApprovalResponse,
    getApprovalResponse,
    settleApprovalResponse,
    clearApprovalResponses,
  }
}
