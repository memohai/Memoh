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
    expect(tracker.settleApprovalResponse('stream-success', 'failed')).toBeUndefined()
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
    expect(tracker.beginApprovalResponse(input('stream-2'))).toBe(true)
  })

  it('rejects incomplete registrations and clears terminal bookkeeping', () => {
    const tracker = createApprovalResponseTracker({ rollbackApproval: vi.fn() })

    expect(tracker.beginApprovalResponse(input(' '))).toBe(false)
    expect(tracker.beginApprovalResponse({ ...input('stream-1'), sessionId: '' })).toBe(false)
    expect(tracker.beginApprovalResponse(input('stream-2'))).toBe(true)
    tracker.clearApprovalResponses()
    expect(tracker.hasPendingApprovalResponse('approval-1')).toBe(false)
  })
})
