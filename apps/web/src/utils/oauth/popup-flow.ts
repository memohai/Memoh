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
      if (options.popup.closed) {
        finishAborted('cancelled')
        return
      }
      void options.pollStatus()
        .then((status) => {
          if (completed) return
          if (options.isAuthorized(status)) {
            finishAuthorized()
            return
          }
          schedulePoll()
        })
        .catch((error: unknown) => {
          if (completed) return
          options.onError?.(error)
          schedulePoll()
        })
    }, options.pollIntervalMs)
  }

  function onMessage(event: Event) {
    const message = event as MessageEvent
    if (message.data?.type !== options.messageType) return
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
