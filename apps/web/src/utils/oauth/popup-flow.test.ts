import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startOAuthPopupFlow } from './popup-flow'

function messageEvent(data: unknown): MessageEvent {
  const event = new Event('message') as MessageEvent
  Object.defineProperty(event, 'data', { value: data })
  return event
}

describe('startOAuthPopupFlow', () => {
  let target: EventTarget
  let removeEventListenerSpy: ReturnType<typeof vi.spyOn>
  let popup: { closed: boolean, close: () => void }
  let closePopup: ReturnType<typeof vi.fn<() => void>>

  beforeEach(() => {
    vi.useFakeTimers()
    target = new EventTarget()
    removeEventListenerSpy = vi.spyOn(target, 'removeEventListener')
    closePopup = vi.fn<() => void>(() => {
      popup.closed = true
    })
    popup = { closed: false, close: closePopup }
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('times out independently of a hanging status poll and cleans up the popup flow', async () => {
    const onAuthorized = vi.fn()
    const onAborted = vi.fn()

    startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus: () => new Promise<null>(() => {}),
      isAuthorized: () => false,
      onAuthorized,
      onAborted,
    })

    await vi.advanceTimersByTimeAsync(5_000)

    expect(onAuthorized).not.toHaveBeenCalled()
    expect(onAborted).toHaveBeenCalledWith('timeout')
    expect(closePopup).toHaveBeenCalledTimes(1)
    expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('treats a closed popup as cancellation and stops polling', async () => {
    const onAborted = vi.fn()
    const pollStatus = vi.fn(async () => null)

    startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus,
      isAuthorized: () => false,
      onAuthorized: vi.fn(),
      onAborted,
    })
    popup.closed = true

    await vi.advanceTimersByTimeAsync(1_000)
    await vi.advanceTimersByTimeAsync(4_000)

    expect(onAborted).toHaveBeenCalledWith('cancelled')
    expect(pollStatus).not.toHaveBeenCalled()
    expect(closePopup).not.toHaveBeenCalled()
    expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('allows explicit cancellation before timeout', () => {
    const onAborted = vi.fn()

    const flow = startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus: vi.fn(async () => null),
      isAuthorized: () => false,
      onAuthorized: vi.fn(),
      onAborted,
    })
    flow.cancel()

    expect(onAborted).toHaveBeenCalledWith('cancelled')
    expect(closePopup).toHaveBeenCalledTimes(1)
    expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('disposes silently and ignores a pending status poll', async () => {
    const onAborted = vi.fn()
    const pollStatus = vi.fn(() => new Promise<null>(resolve => setTimeout(() => resolve(null), 2_000)))

    const flow = startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus,
      isAuthorized: () => false,
      onAuthorized: vi.fn(),
      onAborted,
    })

    await vi.advanceTimersByTimeAsync(1_000)
    flow.dispose()
    await vi.advanceTimersByTimeAsync(4_000)

    expect(onAborted).not.toHaveBeenCalled()
    expect(closePopup).not.toHaveBeenCalled()
    expect(pollStatus).toHaveBeenCalledTimes(1)
    expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('completes once when a matching success message arrives', async () => {
    const onAuthorized = vi.fn(async () => {})
    const onAborted = vi.fn()

    startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      messageMatches: event => event.data?.providerId === 'provider-1',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus: vi.fn(async () => null),
      isAuthorized: () => false,
      onAuthorized,
      onAborted,
    })

    target.dispatchEvent(messageEvent({ type: 'other' }))
    target.dispatchEvent(messageEvent({ type: 'memoh-oauth-success', providerId: 'provider-2' }))
    target.dispatchEvent(messageEvent({ type: 'memoh-oauth-success', providerId: 'provider-1' }))
    target.dispatchEvent(messageEvent({ type: 'memoh-oauth-success', providerId: 'provider-1' }))
    await vi.runAllTimersAsync()

    expect(onAuthorized).toHaveBeenCalledTimes(1)
    expect(onAborted).not.toHaveBeenCalled()
    expect(closePopup).toHaveBeenCalledTimes(1)
    expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
  })
})
