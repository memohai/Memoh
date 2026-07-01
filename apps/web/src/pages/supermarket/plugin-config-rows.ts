import type { PluginsConfigVar, PluginsManifest } from '@memohai/sdk'

export function pluginConfigRows(plugin: PluginsManifest): PluginsConfigVar[] {
  const rows: PluginsConfigVar[] = []
  const rowByKey = new Map<string, PluginsConfigVar>()
  const coveredVariableKeys = new Set<string>()

  const addRow = (item: PluginsConfigVar) => {
    const key = (item.key || '').trim()
    if (!key) return
    const existing = rowByKey.get(key)
    if (existing) {
      existing.required ||= !!item.required
      existing.secret ||= !!item.secret
      if (!existing.description) existing.description = item.description
      if (!existing.defaultValue) existing.defaultValue = item.defaultValue
      return
    }
    const row = { ...item, key }
    rows.push(row)
    rowByKey.set(key, row)
  }

  for (const item of plugin.variables ?? []) {
    const key = (item.key || '').trim()
    if (key) coveredVariableKeys.add(key)
    addRow(item)
  }
  for (const auth of plugin.auth_requirements ?? []) {
    if (auth.type !== 'user_secret') continue
    for (const variable of auth.variables ?? []) {
      const key = variable.trim()
      if (!key) continue
      coveredVariableKeys.add(key)
      addRow({ key, required: true, secret: true })
    }
  }
  for (const mcp of plugin.mcps ?? []) {
    for (const item of mcp.env ?? []) {
      if (shouldRenderResourceConfigRow(item, coveredVariableKeys)) addRow(item)
    }
    for (const item of mcp.headers ?? []) {
      if (shouldRenderResourceConfigRow(item, coveredVariableKeys)) addRow(item)
    }
  }

  return rows
}

function shouldRenderResourceConfigRow(item: PluginsConfigVar, coveredVariableKeys: Set<string>): boolean {
  const key = (item.key || '').trim()
  if (!key) return false
  const referencedKeys = templateVariableKeys(item.defaultValue || '')
  if (!referencedKeys.length) return true
  if (referencedKeys.some(reference => reference !== key && coveredVariableKeys.has(reference))) {
    return false
  }
  return true
}

export function templateVariableKeys(value: string): string[] {
  return [...value.matchAll(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g)]
    .map(match => match[1])
    .filter((key): key is string => !!key)
}
