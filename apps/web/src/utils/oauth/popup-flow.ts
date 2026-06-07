export type OAuthPopupFlowAbortReason = 'timeout' | 'cancelled'

export interface OAuthPopupFlowController {
  cancel: () => void
  dispose: () => void
}

interface OAuthPopupLike {
  readonly closed?: boolean
  close?: () => void
}

interface OAuthPopupFlowOptions<TStatus> {
  popup: OAuthPopupLike
  target: Pick<EventTarget, 'addEventListener' | 'removeEventListener'>
  messageType: string
  messageMatches?: (event: MessageEvent) => boolean
  // When set, success messages must originate from this window (the popup we
  // opened). Guards against unrelated tabs/scripts spoofing the success message.
  expectedSource?: MessageEventSource | null
  // Optional origin allow-check. Left unset by default because the OAuth callback
  // page may be served from a different origin than the SPA (e.g. in dev).
  originMatches?: (event: MessageEvent) => boolean
  pollIntervalMs: number
  timeoutMs: number
  pollStatus: () => Promise<TStatus | null>
  isAuthorized: (status: TStatus | null) => boolean
  onAuthorized: () => Promise<void> | void
  onAborted?: (reason: OAuthPopupFlowAbortReason) => void
  onError?: (error: unknown) => void
}

export function startOAuthPopupFlow<TStatus>(options: OAuthPopupFlowOptions<TStatus>): OAuthPopupFlowController {
  let completed = false
  let pollTimer: ReturnType<typeof globalThis.setTimeout> | null = null
  let timeoutTimer: ReturnType<typeof globalThis.setTimeout> | null = null

  const clearPollTimer = () => {
    if (pollTimer === null) return
    globalThis.clearTimeout(pollTimer)
    pollTimer = null
  }

  const clearTimeoutTimer = () => {
    if (timeoutTimer === null) return
    globalThis.clearTimeout(timeoutTimer)
    timeoutTimer = null
  }

  const closePopup = () => {
    if (options.popup.closed) return
    options.popup.close?.()
  }

  const cleanup = () => {
    clearPollTimer()
    clearTimeoutTimer()
    options.target.removeEventListener('message', onMessage)
  }

  const finishAuthorized = () => {
    if (completed) return
    completed = true
    cleanup()
    closePopup()
    void Promise.resolve(options.onAuthorized()).catch((error: unknown) => {
      options.onError?.(error)
    })
  }

  const finishAborted = (reason: OAuthPopupFlowAbortReason) => {
    if (completed) return
    completed = true
    cleanup()
    closePopup()
    options.onAborted?.(reason)
  }

  const schedulePoll = () => {
    if (completed) return
    pollTimer = globalThis.setTimeout(() => {
      pollTimer = null
      if (completed) return
      // The callback page posts its success message and immediately calls
      // window.close(), so a closed popup does NOT necessarily mean the user
      // cancelled — the token may already be stored. Poll one final time before
      // concluding, and only abort as cancelled if it is still unauthorized.
      const popupClosed = options.popup.closed === true
      void options.pollStatus()
        .then((status) => {
          if (completed) return
          if (options.isAuthorized(status)) {
            finishAuthorized()
            return
          }
          if (popupClosed) {
            finishAborted('cancelled')
            return
          }
          schedulePoll()
        })
        .catch((error: unknown) => {
          if (completed) return
          options.onError?.(error)
          if (popupClosed) {
            finishAborted('cancelled')
            return
          }
          schedulePoll()
        })
    }, options.pollIntervalMs)
  }

  function onMessage(event: Event) {
    const message = event as MessageEvent
    if (message.data?.type !== options.messageType) return
    if (options.expectedSource !== undefined && message.source !== options.expectedSource) return
    if (options.originMatches && !options.originMatches(message)) return
    if (options.messageMatches && !options.messageMatches(message)) return
    finishAuthorized()
  }

  options.target.addEventListener('message', onMessage)
  timeoutTimer = globalThis.setTimeout(() => {
    finishAborted('timeout')
  }, options.timeoutMs)
  schedulePoll()

  return {
    cancel: () => finishAborted('cancelled'),
    dispose: () => {
      if (completed) return
      completed = true
      cleanup()
    },
  }
}
