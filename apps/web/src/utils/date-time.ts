interface FormatDateOptions {
  fallback?: string
  invalidFallback?: string
  locale?: string
}

function parseDate(value: string | null | undefined): Date | null {
  if (!value) {
    return null
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

/**
 * Resolves the display string when a date value is non-null but could not be
 * parsed.  `invalidFallback` takes precedence over `fallback`; when neither is
 * supplied the raw input value is returned so callers can see what arrived.
 */
function resolveInvalid(value: string, options: FormatDateOptions): string {
  if (options.invalidFallback !== undefined) return options.invalidFallback
  if (options.fallback !== undefined) return options.fallback
  return value
}

export function formatDateTime(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const parsed = parseDate(value)
  if (!parsed) return resolveInvalid(value, options)
  return parsed.toLocaleString(options.locale)
}

export function formatDate(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const parsed = parseDate(value)
  if (!parsed) return resolveInvalid(value, options)
  return parsed.toLocaleDateString(options.locale)
}

export function formatDateTimeSeconds(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const parsed = parseDate(value)
  if (!parsed) return resolveInvalid(value, options)

  const year = parsed.getFullYear()
  const month = String(parsed.getMonth() + 1).padStart(2, '0')
  const day = String(parsed.getDate()).padStart(2, '0')
  const hours = String(parsed.getHours()).padStart(2, '0')
  const minutes = String(parsed.getMinutes()).padStart(2, '0')
  const seconds = String(parsed.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

/**
 * Compact, locale-aware "month day, time" stamp for dense data tables — e.g.
 * "Jun 11, 6:43 PM" (en) or "6月11日 下午6:43" (zh). Year is omitted (usage logs
 * are recent), and seconds are dropped so rows read at a glance instead of as a
 * full ISO timestamp.
 */
export function formatDateTimeShort(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const parsed = parseDate(value)
  if (!parsed) return resolveInvalid(value, options)
  return parsed.toLocaleString(options.locale, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Calendar-anchored stamp for a single item's detail (e.g. a chat turn's "more"
 * menu): "Today 10:11 PM" / "Yesterday 10:11 PM" for the last two days, then
 * "Jun 14, 10:11 PM" within the year and "Jun 14, 2025, 10:11 PM" beyond it.
 * Unlike `formatRelativeTime` it never decays to a vague "3 hours ago" — the
 * point here is a precise, readable time the reader can trust at a glance.
 * Today/Yesterday are produced via `Intl.RelativeTimeFormat` so the words are
 * localized (zh → 今天 / 昨天) without a translation table.
 */
export function formatCalendarTime(
  value: string | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const parsed = parseDate(value)
  if (!parsed) return resolveInvalid(value, options)

  const time = parsed.toLocaleTimeString(options.locale, {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })

  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const dayDiff = Math.round((startOfDay(parsed) - startOfDay(new Date())) / 86_400_000)

  if (dayDiff === 0 || dayDiff === -1) {
    const rtf = new Intl.RelativeTimeFormat(options.locale, { numeric: 'auto' })
    const day = rtf.format(dayDiff, 'day')
    return `${day.charAt(0).toUpperCase()}${day.slice(1)} ${time}`
  }

  const sameYear = parsed.getFullYear() === new Date().getFullYear()
  return parsed.toLocaleString(options.locale, {
    year: sameYear ? undefined : 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Returns a locale-aware relative time string such as "3 minutes ago" or
 * "in 2 days".  Falls back to `toLocaleDateString()` for dates older than a
 * week.  Accepts either an ISO string or a `Date` object.
 *
 * Uses `Intl.RelativeTimeFormat` so callers can align the output language with
 * the app locale instead of relying on the browser's preferred locale.
 */
export function formatRelativeTime(
  value: string | Date | null | undefined,
  options: FormatDateOptions = {},
): string {
  if (!value) return options.fallback ?? ''
  const date = value instanceof Date ? value : parseDate(value)
  if (!date) return resolveInvalid(value as string, options)

  const diffMs = date.getTime() - Date.now()
  const absDiffSec = Math.abs(diffMs) / 1000
  const rtf = new Intl.RelativeTimeFormat(options.locale, { numeric: 'auto' })

  if (absDiffSec < 60) return rtf.format(Math.round(diffMs / 1000), 'second')
  if (absDiffSec < 3_600) return rtf.format(Math.round(diffMs / 60_000), 'minute')
  if (absDiffSec < 86_400) return rtf.format(Math.round(diffMs / 3_600_000), 'hour')
  if (absDiffSec < 604_800) return rtf.format(Math.round(diffMs / 86_400_000), 'day')

  // Beyond a week: absolute date is more readable than "34 days ago"
  return date.toLocaleDateString(options.locale)
}
