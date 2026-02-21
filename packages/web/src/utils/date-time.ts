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
