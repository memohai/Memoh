export type ApprovalTool = 'read' | 'write' | 'exec'

export interface ToolApprovalFilePolicy {
  require_approval: boolean
  bypass_globs: string[]
  force_review_globs: string[]
}

export interface ToolApprovalExecPolicy {
  require_approval: boolean
  bypass_commands: string[]
  force_review_commands: string[]
}

export interface ToolApprovalConfig {
  enabled: boolean
  read: ToolApprovalFilePolicy
  write: ToolApprovalFilePolicy
  exec: ToolApprovalExecPolicy
}

interface RawFilePolicy {
  require_approval?: unknown
  bypass_globs?: unknown
  force_review_globs?: unknown
}

interface RawExecPolicy {
  require_approval?: unknown
  bypass_commands?: unknown
  force_review_commands?: unknown
}

interface RawToolApprovalConfig {
  enabled?: unknown
  read?: RawFilePolicy
  write?: RawFilePolicy
  edit?: RawFilePolicy
  exec?: RawExecPolicy
}

export const defaultToolApprovalConfig = (): ToolApprovalConfig => ({
  enabled: false,
  read: {
    require_approval: false,
    bypass_globs: [],
    force_review_globs: [],
  },
  write: {
    require_approval: true,
    bypass_globs: ['/data/**', '/tmp/**'],
    force_review_globs: [],
  },
  exec: {
    require_approval: false,
    bypass_commands: [],
    force_review_commands: [],
  },
})

function normalizeStringList(raw: unknown, fallback: string[]): string[] {
  if (!Array.isArray(raw)) return [...fallback]
  return raw.filter((item): item is string => typeof item === 'string')
}

function mergeStringLists(...lists: string[][]): string[] {
  const seen = new Set<string>()
  const merged: string[] = []
  for (const list of lists) {
    for (const item of list) {
      if (seen.has(item)) continue
      seen.add(item)
      merged.push(item)
    }
  }
  return merged
}

function normalizeFilePolicy(raw: unknown, defaults: ToolApprovalFilePolicy): ToolApprovalFilePolicy {
  const value = raw && typeof raw === 'object' ? raw as RawFilePolicy : {}
  return {
    require_approval: typeof value.require_approval === 'boolean'
      ? value.require_approval
      : defaults.require_approval,
    bypass_globs: normalizeStringList(value.bypass_globs, defaults.bypass_globs),
    force_review_globs: normalizeStringList(value.force_review_globs, defaults.force_review_globs),
  }
}

function normalizeExecPolicy(raw: unknown, defaults: ToolApprovalExecPolicy): ToolApprovalExecPolicy {
  const value = raw && typeof raw === 'object' ? raw as RawExecPolicy : {}
  return {
    require_approval: typeof value.require_approval === 'boolean'
      ? value.require_approval
      : defaults.require_approval,
    bypass_commands: normalizeStringList(value.bypass_commands, defaults.bypass_commands),
    force_review_commands: normalizeStringList(value.force_review_commands, defaults.force_review_commands),
  }
}

function mergeFilePolicies(base: ToolApprovalFilePolicy, legacy: ToolApprovalFilePolicy): ToolApprovalFilePolicy {
  return {
    require_approval: base.require_approval || legacy.require_approval,
    bypass_globs: mergeStringLists(base.bypass_globs, legacy.bypass_globs),
    force_review_globs: mergeStringLists(base.force_review_globs, legacy.force_review_globs),
  }
}

export function normalizeToolApprovalConfig(raw: unknown): ToolApprovalConfig {
  const defaults = defaultToolApprovalConfig()
  if (!raw || typeof raw !== 'object') return defaults
  const value = raw as RawToolApprovalConfig
  const write = normalizeFilePolicy(value.write, defaults.write)
  return {
    enabled: typeof value.enabled === 'boolean' ? value.enabled : defaults.enabled,
    read: normalizeFilePolicy(value.read, defaults.read),
    write: value.edit
      ? mergeFilePolicies(write, normalizeFilePolicy(value.edit, defaults.write))
      : write,
    exec: normalizeExecPolicy(value.exec, defaults.exec),
  }
}
