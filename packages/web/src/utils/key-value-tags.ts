export function tagsToRecord(tags: string[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const tag of tags) {
    // Use indexOf so that values containing ':' (e.g. URLs) are preserved intact.
    const sep = tag.indexOf(':')
    if (sep <= 0) continue
    const key = tag.slice(0, sep)
    const value = tag.slice(sep + 1)
    if (key && value) {
      out[key] = value
    }
  }
  return out
}

export function recordToTags(record: Record<string, string> | null | undefined): string[] {
  if (!record) {
    return []
  }
  return Object.entries(record).map(([key, value]) => `${key}:${value}`)
}
