export interface HistorySentinelReadinessInput {
  isVisible: boolean
  loadingMessages: boolean
  sessionLandingPending: boolean
  userRequestedHistory: boolean
  loadingOlder: boolean
  hasMoreOlder: boolean
  messageCount: number
}

/**
 * Pure gate for the top history sentinel. Intersection alone is not intent:
 * the sentinel starts at scrollTop===0 and can remain intersecting when the
 * first page is shorter than the viewport. Require an upward user gesture as
 * well as a completed first bottom landing so initial layout can never chain
 * before_message_id requests.
 */
export function canTriggerHistorySentinel(input: HistorySentinelReadinessInput): boolean {
  if (!input.isVisible) return false
  if (input.loadingMessages) return false
  if (input.sessionLandingPending) return false
  if (!input.userRequestedHistory) return false
  if (input.loadingOlder) return false
  if (!input.hasMoreOlder) return false
  if (input.messageCount <= 0) return false
  return true
}

export interface HistoryPrependAnchorInput {
  capturedAnchorTop: number
  currentAnchorTop: number
  userIntervenedDuringLoad: boolean
}

/**
 * Return the correction needed to keep one pre-existing row at the same
 * viewport offset after older rows are prepended. Measuring the row instead of
 * scrollHeight is load-bearing: streaming can grow the document below the
 * reader while the history request is in flight, and that growth must not move
 * the viewport. A physical gesture during the request hands ownership back to
 * the user and suppresses this correction.
 */
export function historyPrependAnchorDelta(input: HistoryPrependAnchorInput): number | null {
  if (input.userIntervenedDuringLoad) return null
  const delta = input.currentAnchorTop - input.capturedAnchorTop
  return Math.abs(delta) < 0.5 ? null : delta
}
