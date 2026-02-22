interface RetryingStreamOptions {
  initialBackoffMs?: number
  maxBackoffMs?: number
  reconnectDelayMs?: number
}

type StreamAttempt = (signal: AbortSignal) => Promise<void>

const DEFAULT_INITIAL_BACKOFF_MS = 1000
const DEFAULT_MAX_BACKOFF_MS = 5000
const DEFAULT_RECONNECT_DELAY_MS = 300

function sleep(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms))
}

export function useRetryingStream(options: RetryingStreamOptions = {}) {
  const initialBackoffMs = options.initialBackoffMs ?? DEFAULT_INITIAL_BACKOFF_MS
  const maxBackoffMs = options.maxBackoffMs ?? DEFAULT_MAX_BACKOFF_MS
  const reconnectDelayMs = options.reconnectDelayMs ?? DEFAULT_RECONNECT_DELAY_MS

  let controller: AbortController | null = null
  let loopVersion = 0

  function stop() {
    loopVersion += 1
    if (controller) {
      controller.abort()
      controller = null
    }
  }

  function start(runAttempt: StreamAttempt) {
    stop()
    const nextController = new AbortController()
    controller = nextController
    const version = loopVersion

    const run = async () => {
      let delay = initialBackoffMs
      while (!nextController.signal.aborted && loopVersion === version) {
        try {
          await runAttempt(nextController.signal)
          delay = initialBackoffMs
          if (!nextController.signal.aborted && loopVersion === version) {
            await sleep(reconnectDelayMs)
          }
        } catch {
          if (nextController.signal.aborted || loopVersion !== version) return
          await sleep(delay)
          delay = Math.min(delay * 2, maxBackoffMs)
        }
      }
    }

    void run()
  }

  return {
    start,
    stop,
  }
}
