export function getModelDescription(config: unknown): string | undefined {
  if (!config || typeof config !== 'object') return undefined
  const description = (config as { description?: unknown }).description
  if (typeof description !== 'string') return undefined
  return description.trim() || undefined
}

export function matchesModelSearch(query: string, values: Array<string | undefined>): boolean {
  const keyword = query.trim().toLowerCase()
  if (!keyword) return true
  return values.some(value => value?.toLowerCase().includes(keyword))
}
