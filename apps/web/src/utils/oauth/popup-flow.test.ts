import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startOAuthPopupFlow } from './popup-flow'

function messageEvent(data: unknown, source?: MessageEventSource | null): MessageEvent {
  const event = new Event('message') as MessageEvent
  Object.defineProperty(event, 'data', { value: data })
  if (source !== undefined) Object.defineProperty(event, 'source', { value: source })
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

  it('polls once more before treating a closed popup as cancellation', async () => {
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

    // The callback page closes the popup right after posting success, so a closed
    // popup must be confirmed via one final poll before concluding cancellation.
    expect(pollStatus).toHaveBeenCalledTimes(1)
    expect(onAborted).toHaveBeenCalledWith('cancelled')
    expect(closePopup).not.toHaveBeenCalled()

    // ...and once cancelled it stops polling.
    await vi.advanceTimersByTimeAsync(4_000)
    expect(pollStatus).toHaveBeenCalledTimes(1)
    expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('completes authorization when a closed popup already stored a token', async () => {
    const onAuthorized = vi.fn(async () => {})
    const onAborted = vi.fn()
    const pollStatus = vi.fn(async () => ({ has_token: true }))

    startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus,
      isAuthorized: status => Boolean(status?.has_token),
      onAuthorized,
      onAborted,
    })
    popup.closed = true

    await vi.advanceTimersByTimeAsync(1_000)

    expect(onAuthorized).toHaveBeenCalledTimes(1)
    expect(onAborted).not.toHaveBeenCalled()
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

  it('ignores success messages from an unexpected source', async () => {
    const onAuthorized = vi.fn(async () => {})
    const expectedSource = popup as unknown as MessageEventSource

    startOAuthPopupFlow({
      popup,
      target,
      expectedSource,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus: vi.fn(async () => null),
      isAuthorized: () => false,
      onAuthorized,
      onAborted: vi.fn(),
    })

    target.dispatchEvent(messageEvent({ type: 'memoh-oauth-success' }, {} as MessageEventSource))
    expect(onAuthorized).not.toHaveBeenCalled()

    target.dispatchEvent(messageEvent({ type: 'memoh-oauth-success' }, expectedSource))
    await vi.runAllTimersAsync()
    expect(onAuthorized).toHaveBeenCalledTimes(1)
  })

  it('reports a failing status poll via onError and keeps polling', async () => {
    const onError = vi.fn()
    const onAuthorized = vi.fn(async () => {})
    const pollStatus = vi.fn<() => Promise<{ has_token: boolean } | null>>()
    pollStatus.mockRejectedValueOnce(new Error('poll failed'))
    pollStatus.mockResolvedValue({ has_token: true })

    startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 60_000,
      pollStatus,
      isAuthorized: status => Boolean(status?.has_token),
      onAuthorized,
      onAborted: vi.fn(),
      onError,
    })

    await vi.advanceTimersByTimeAsync(1_000)
    expect(onError).toHaveBeenCalledTimes(1)
    expect(onAuthorized).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1_000)
    expect(pollStatus).toHaveBeenCalledTimes(2)
    expect(onAuthorized).toHaveBeenCalledTimes(1)
  })

  it('routes onAuthorized failures to onError', async () => {
    const onError = vi.fn()
    const onAuthorized = vi.fn(async () => {
      throw new Error('save failed')
    })

    startOAuthPopupFlow({
      popup,
      target,
      messageType: 'memoh-oauth-success',
      pollIntervalMs: 1_000,
      timeoutMs: 5_000,
      pollStatus: vi.fn(async () => null),
      isAuthorized: () => false,
      onAuthorized,
      onAborted: vi.fn(),
      onError,
    })

    target.dispatchEvent(messageEvent({ type: 'memoh-oauth-success' }))
    await vi.runAllTimersAsync()

    expect(onAuthorized).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledTimes(1)
  })
})
