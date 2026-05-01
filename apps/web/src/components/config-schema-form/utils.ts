export function getPathValue(source: Record<string, unknown>, path: string): unknown {
  const parts = path.split('.')
  let current: unknown = source
  for (const part of parts) {
    if (!current || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[part]
  }
  return current
}

export function setPathValue(source: Record<string, unknown>, path: string, value: unknown) {
  const parts = path.split('.')
  let current: Record<string, unknown> = source
  for (const part of parts.slice(0, -1)) {
    const next = current[part]
    if (!next || typeof next !== 'object' || Array.isArray(next)) {
      current[part] = {}
    }
    current = current[part] as Record<string, unknown>
  }

  const last = parts.at(-1)
  if (!last) return
  current[last] = value
}

export function cloneConfig<T>(value: T): T {
  if (value == null) return {} as T
  return JSON.parse(JSON.stringify(value))
}

