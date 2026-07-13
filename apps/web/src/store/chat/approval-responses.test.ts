import { describe, expect, it, vi } from 'vitest'
import { createApprovalResponseTracker } from './approval-responses'

function input(streamId: string, approvalId = 'approval-1', silent = false) {
  return {
    streamId,
    approvalId,
    botId: 'bot-1',
    sessionId: 'session-1',
    silent,
  }
}

describe('approval response tracker', () => {
  it('registers one response per approval and exposes its routing context', () => {
    const rollbackApproval = vi.fn()
    const tracker = createApprovalResponseTracker({ rollbackApproval })

    expect(tracker.beginApprovalResponse(input(' stream-1 ', ' approval-1 ', true))).toBe(true)
    expect(tracker.getApprovalResponse('stream-1')).toMatchObject({
      streamId: 'stream-1',
      approvalId: 'approval-1',
      botId: 'bot-1',
      sessionId: 'session-1',
      silent: true,
    })
    expect(tracker.hasPendingApprovalResponse('approval-1')).toBe(true)
    expect(tracker.beginApprovalResponse(input('stream-2', 'approval-1'))).toBe(false)
    expect(tracker.beginApprovalResponse(input('stream-1', 'approval-2'))).toBe(false)
    expect(rollbackApproval).not.toHaveBeenCalled()
    expect(tracker.pendingApprovalResponsesForSession('bot-1', 'session-1')).toHaveLength(1)
    expect(tracker.pendingApprovalResponsesForSession('bot-2', 'session-1')).toEqual([])
    tracker.resetApprovalResponses()
  })

  it('settles success, failure, and local cancellation through one transition', () => {
    const rollbackApproval = vi.fn()
    const tracker = createApprovalResponseTracker({ rollbackApproval })
    tracker.beginApprovalResponse(input('stream-success', 'approval-success'))
    tracker.beginApprovalResponse(input('stream-failure', 'approval-failure'))
    tracker.beginApprovalResponse(input('stream-canceled', 'approval-canceled'))

    expect(tracker.settleApprovalResponse('stream-success', 'succeeded')?.approvalId).toBe('approval-success')
    expect(tracker.settleApprovalResponse('stream-failure', 'failed')?.approvalId).toBe('approval-failure')
    expect(tracker.settleApprovalResponse('stream-canceled', 'canceled')?.approvalId).toBe('approval-canceled')
    expect(rollbackApproval).toHaveBeenCalledOnce()
    expect(rollbackApproval).toHaveBeenCalledWith('approval-failure')
    expect(tracker.getApprovalResponse('stream-success')).toBeUndefined()
    expect(tracker.isTerminalApprovalResponse('stream-success')).toBe(true)
    expect(tracker.isTerminalApprovalResponse('stream-failure')).toBe(true)
    expect(tracker.isTerminalApprovalResponse('stream-canceled')).toBe(true)
    expect(tracker.settleApprovalResponse('stream-success', 'failed')).toBeUndefined()
    expect(rollbackApproval).toHaveBeenCalledOnce()
  })

  it('uses the response-owned rollback when its transcript is no longer current', () => {
    const rollbackApproval = vi.fn()
    const rollbackResponse = vi.fn()
    const tracker = createApprovalResponseTracker({ rollbackApproval })

    tracker.beginApprovalResponse({
      ...input('stream-1'),
      rollback: rollbackResponse,
    })
    tracker.settleApprovalResponse('stream-1', 'failed')

    expect(rollbackResponse).toHaveBeenCalledOnce()
    expect(rollbackApproval).not.toHaveBeenCalled()
  })

  it('marks only responses on the disconnected bot as awaiting resync', () => {
    const tracker = createApprovalResponseTracker({ rollbackApproval: vi.fn() })
    tracker.beginApprovalResponse({
      ...input('stream-1', 'approval-1'),
      decision: 'approve',
      shortId: 7,
    })
    tracker.beginApprovalResponse({
      ...input('stream-2', 'approval-2'),
      botId: 'bot-2',
    })

    tracker.markPendingApprovalResponsesUncertain('bot-1')

    expect(tracker.getApprovalResponse('stream-1')).toMatchObject({
      awaitingResync: true,
      replaySent: false,
      decision: 'approve',
      shortId: 7,
    })
    expect(tracker.getApprovalResponse('stream-2')?.awaitingResync).toBe(false)
    expect(tracker.hasUncertainApprovalResponse('bot-1', 'session-1')).toBe(true)
    expect(tracker.hasUncertainApprovalResponse('bot-2', 'session-1')).toBe(false)
    tracker.resetApprovalResponses()
  })

  it('marks a reconnect replay only once until the next disconnect', () => {
    const tracker = createApprovalResponseTracker({ rollbackApproval: vi.fn() })
    tracker.beginApprovalResponse({ ...input('stream-1'), decision: 'approve' })
    tracker.markPendingApprovalResponsesUncertain('bot-1')

    expect(tracker.markApprovalResponseReplaySent('stream-1')).toBe(true)
    expect(tracker.markApprovalResponseReplaySent('stream-1')).toBe(false)
    expect(tracker.getApprovalResponse('stream-1')).toMatchObject({
      awaitingResync: true,
      replaySent: true,
    })

    tracker.markPendingApprovalResponsesUncertain('bot-1')
    expect(tracker.markApprovalResponseReplaySent('stream-1')).toBe(true)
  })

  it('keeps an uncertain response locked after its ordinary expiry', () => {
    let currentTime = 1_000
    const rollbackApproval = vi.fn()
    const tracker = createApprovalResponseTracker({
      rollbackApproval,
      now: () => currentTime,
      ttlMs: 100,
    })
    tracker.beginApprovalResponse({ ...input('stream-1'), decision: 'approve' })
    tracker.markPendingApprovalResponsesUncertain('bot-1')

    currentTime = 1_100
    expect(tracker.hasPendingApprovalResponse('approval-1')).toBe(true)
    expect(tracker.getApprovalResponse('stream-1')).toMatchObject({ awaitingResync: true })
    expect(rollbackApproval).not.toHaveBeenCalled()
  })

  it('expires abandoned responses, rolls them back, and allows a retry', () => {
    let currentTime = 1_000
    const rollbackApproval = vi.fn()
    const tracker = createApprovalResponseTracker({
      rollbackApproval,
      now: () => currentTime,
      ttlMs: 100,
    })
    tracker.beginApprovalResponse(input('stream-1'))

    currentTime = 1_099
    expect(tracker.hasPendingApprovalResponse('approval-1')).toBe(true)
    currentTime = 1_100
    expect(tracker.hasPendingApprovalResponse('approval-1')).toBe(false)
    expect(rollbackApproval).toHaveBeenCalledWith('approval-1')
    expect(tracker.beginApprovalResponse(input('stream-1'))).toBe(false)
    expect(tracker.beginApprovalResponse(input('stream-2'))).toBe(true)
    tracker.resetApprovalResponses()
  })

  it('rejects incomplete registrations and clears terminal bookkeeping', () => {
    const tracker = createApprovalResponseTracker({ rollbackApproval: vi.fn() })

    expect(tracker.beginApprovalResponse(input(' '))).toBe(false)
    expect(tracker.beginApprovalResponse({ ...input('stream-1'), sessionId: '' })).toBe(false)
    expect(tracker.beginApprovalResponse(input('stream-2'))).toBe(true)
    expect(tracker.discardAllApprovalResponses()).toHaveLength(1)
    expect(tracker.isTerminalApprovalResponse('stream-2')).toBe(true)
    tracker.resetApprovalResponses()
    expect(tracker.hasPendingApprovalResponse('approval-1')).toBe(false)
    expect(tracker.isTerminalApprovalResponse('stream-2')).toBe(false)
  })

  it('evicts the oldest terminal response at the configured bound', () => {
    const tracker = createApprovalResponseTracker({
      rollbackApproval: vi.fn(),
      terminalHistoryLimit: 2,
    })
    for (const id of ['stream-1', 'stream-2', 'stream-3']) {
      tracker.beginApprovalResponse(input(id, `approval-${id}`))
      tracker.settleApprovalResponse(id, 'succeeded')
    }

    expect(tracker.isTerminalApprovalResponse('stream-1')).toBe(false)
    expect(tracker.isTerminalApprovalResponse('stream-2')).toBe(true)
    expect(tracker.isTerminalApprovalResponse('stream-3')).toBe(true)
  })

  it('automatically expires an abandoned response and reports its captured context', () => {
    let currentTime = 1_000
    const scheduled: Array<{ callback: () => void; canceled: boolean; delayMs: number }> = []
    const rollbackApproval = vi.fn()
    const onExpired = vi.fn()
    const tracker = createApprovalResponseTracker({
      rollbackApproval,
      onExpired,
      now: () => currentTime,
      ttlMs: 100,
      scheduleExpiry: (callback, delayMs) => {
        const entry = { callback, delayMs, canceled: false }
        scheduled.push(entry)
        return () => { entry.canceled = true }
      },
    })
    tracker.beginApprovalResponse(input('stream-1', 'approval-1', true))

    expect(scheduled[0]?.delayMs).toBe(100)
    currentTime = 1_099
    scheduled[0]!.callback()
    expect(scheduled[1]?.delayMs).toBe(1)
    expect(rollbackApproval).not.toHaveBeenCalled()

    currentTime = 1_100
    scheduled[1]!.callback()
    expect(rollbackApproval).toHaveBeenCalledWith('approval-1')
    expect(onExpired).toHaveBeenCalledWith(expect.objectContaining({
      streamId: 'stream-1',
      approvalId: 'approval-1',
      botId: 'bot-1',
      sessionId: 'session-1',
      silent: true,
    }))
    expect(tracker.getApprovalResponse('stream-1')).toBeUndefined()
    expect(tracker.isTerminalApprovalResponse('stream-1')).toBe(true)
  })
})
