interface ResolveApiErrorMessageOptions {
  prefixFallback?: boolean
}

function pickErrorDetail(error: unknown): string {
  if (!error || typeof error !== 'object') {
    return ''
  }

  const payload = error as {
    message?: unknown
    error?: unknown
    detail?: unknown
  }

  for (const value of [payload.message, payload.error, payload.detail]) {
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }

  return ''
}

export function resolveApiErrorMessage(
  error: unknown,
  fallback: string,
  options: ResolveApiErrorMessageOptions = {},
): string {
  const detail = pickErrorDetail(error)
  if (!detail) {
    return fallback
  }

  if (options.prefixFallback) {
    return `${fallback}: ${detail}`
  }

  return detail
}
