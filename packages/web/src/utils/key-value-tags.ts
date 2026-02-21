export function tagsToRecord(tags: string[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const tag of tags) {
    const [key, value] = tag.split(':')
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
