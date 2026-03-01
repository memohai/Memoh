interface FormatDateOptions {
  fallback?: string
  invalidFallback?: string
}

function parseDate(value: string | null | undefined): Date | null {
  if (!value) {
    return null
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

export function formatDateTime(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  const parsed = parseDate(value)
  if (!parsed) {
    return options.fallback ?? ''
  }
  return parsed.toLocaleString()
}

export function formatDate(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  const parsed = parseDate(value)
  if (!parsed) {
    return options.fallback ?? ''
  }
  return parsed.toLocaleDateString()
}

export function formatDateTimeSeconds(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) {
    return options.fallback ?? ''
  }
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return options.invalidFallback ?? value
  }

  const year = parsed.getFullYear()
  const month = String(parsed.getMonth() + 1).padStart(2, '0')
  const day = String(parsed.getDate()).padStart(2, '0')
  const hours = String(parsed.getHours()).padStart(2, '0')
  const minutes = String(parsed.getMinutes()).padStart(2, '0')
  const seconds = String(parsed.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

/**
 * Returns a locale-aware relative time string such as "3 minutes ago" or
 * "in 2 days".  Falls back to `toLocaleDateString()` for dates older than a
 * week.  Accepts either an ISO string or a `Date` object.
 *
 * Uses `Intl.RelativeTimeFormat` so the output language follows the browser
 * locale automatically — no hardcoded English strings.
 */
export function formatRelativeTime(
  value: string | Date | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const date = value instanceof Date ? value : parseDate(value)
  if (!date) return options.fallback ?? ''

  const diffMs = date.getTime() - Date.now()
  const absDiffSec = Math.abs(diffMs) / 1000
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

  if (absDiffSec < 60) return rtf.format(Math.round(diffMs / 1000), 'second')
  if (absDiffSec < 3_600) return rtf.format(Math.round(diffMs / 60_000), 'minute')
  if (absDiffSec < 86_400) return rtf.format(Math.round(diffMs / 3_600_000), 'hour')
  if (absDiffSec < 604_800) return rtf.format(Math.round(diffMs / 86_400_000), 'day')

  return date.toLocaleDateString()
}
