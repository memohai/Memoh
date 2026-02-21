interface FormatDateOptions {
  fallback?: string
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
