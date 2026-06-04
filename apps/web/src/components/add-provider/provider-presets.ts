interface ProviderNameLike {
  name?: string
}

export function suggestProviderName(baseName: string, providers: ProviderNameLike[]): string {
  const trimmedBase = baseName.trim()
  if (!trimmedBase) return ''

  const existing = new Set(
    providers
      .map(provider => provider.name?.trim())
      .filter((name): name is string => Boolean(name)),
  )

  if (!existing.has(trimmedBase)) return trimmedBase

  let index = 2
  while (existing.has(`${trimmedBase} ${index}`)) {
    index++
  }
  return `${trimmedBase} ${index}`
}
