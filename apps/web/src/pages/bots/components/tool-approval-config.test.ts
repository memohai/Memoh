import { describe, expect, it } from 'vitest'
import {
  cloneToolApprovalConfig,
  defaultToolApprovalConfig,
  dirtyToolApprovalTargetIds,
  normalizeToolApprovalConfig,
  parseToolApprovalRules,
  saveDirtyToolApprovalTargets,
} from './tool-approval-config'

describe('normalizeToolApprovalConfig', () => {
  it('uses target-specific recommended defaults', () => {
    expect(normalizeToolApprovalConfig(undefined)).toEqual(defaultToolApprovalConfig('native'))
    expect(normalizeToolApprovalConfig(undefined, {}, 'remote')).toEqual(defaultToolApprovalConfig('remote'))
  })

  it('preserves the target-level enabled switch while reading effective modes', () => {
    const normalized = normalizeToolApprovalConfig({
      enabled: false,
      write: { require_approval: true },
    }, {
      read: 'allow',
      write: 'allow',
      exec: 'allow',
    })

    expect(normalized.read.mode).toBe('allow')
    expect(normalized.write.mode).toBe('allow')
    expect(normalized.exec.mode).toBe('allow')
    expect(normalized.enabled).toBe(false)
  })

  it('merges legacy edit policy without dropping advanced rules', () => {
    const normalized = normalizeToolApprovalConfig({
      write: {
        mode: 'allow',
        bypass_globs: ['/data/**', '/workspace/cache/**'],
        force_review_globs: ['/workspace/secrets/**'],
      },
      edit: {
        mode: 'ask',
        bypass_globs: ['/data/**', '/tmp/**'],
        force_review_globs: ['.env*'],
      },
    })

    expect(normalized.write).toEqual({
      mode: 'ask',
      require_approval: true,
      bypass_globs: ['/data/**', '/workspace/cache/**', '/tmp/**'],
      force_review_globs: ['/workspace/secrets/**', '.env*'],
    })
    expect('edit' in normalized).toBe(false)
  })
})

describe('tool approval drafts', () => {
  it('splits advanced rules by newline or comma without gitignore extensions', () => {
    expect(parseToolApprovalRules('src/**, README.md\n#literal\n!literal')).toEqual([
      'src/**',
      'README.md',
      '#literal',
      '!literal',
    ])
  })

  it('saves only dirty targets and reports partial failures', async () => {
    const native = defaultToolApprovalConfig('native')
    const remote = defaultToolApprovalConfig('remote')
    const drafts = {
      native: cloneToolApprovalConfig(native),
      remote: cloneToolApprovalConfig(remote),
    }
    drafts.native.read.force_review_globs = ['/etc/**']
    drafts.remote.exec.bypass_commands = ['git status']
    expect(dirtyToolApprovalTargetIds(drafts, { native, remote })).toEqual(['native', 'remote'])

    const attempted: string[] = []
    const result = await saveDirtyToolApprovalTargets(drafts, { native, remote }, async (targetId) => {
      attempted.push(targetId)
      if (targetId === 'remote') throw new Error('offline')
    })

    expect(attempted).toEqual(['native', 'remote'])
    expect(result.savedTargetIds).toEqual(['native'])
    expect(result.failedTargets.map(item => item.targetId)).toEqual(['remote'])
  })
})
