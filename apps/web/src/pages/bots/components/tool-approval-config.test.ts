import { describe, expect, it } from 'vitest'
import { defaultToolApprovalConfig, normalizeToolApprovalConfig } from './tool-approval-config'

describe('normalizeToolApprovalConfig', () => {
  it('uses read/write/exec defaults for empty config', () => {
    expect(normalizeToolApprovalConfig(undefined)).toEqual(defaultToolApprovalConfig())
  })

  it('merges legacy edit policy into write policy', () => {
    const normalized = normalizeToolApprovalConfig({
      enabled: true,
      write: {
        require_approval: false,
        bypass_globs: ['/data/**', '/workspace/cache/**'],
        force_review_globs: ['/workspace/secrets/**'],
      },
      edit: {
        require_approval: true,
        bypass_globs: ['/data/**', '/tmp/**'],
        force_review_globs: ['.env*'],
      },
      exec: {
        require_approval: false,
        bypass_commands: ['pwd'],
        force_review_commands: ['sudo *'],
      },
    })

    expect(normalized.enabled).toBe(true)
    expect(normalized.write).toEqual({
      require_approval: true,
      bypass_globs: ['/data/**', '/workspace/cache/**', '/tmp/**'],
      force_review_globs: ['/workspace/secrets/**', '.env*'],
    })
    expect('edit' in normalized).toBe(false)
  })

  it('keeps read independent from write compatibility merging', () => {
    const normalized = normalizeToolApprovalConfig({
      read: {
        require_approval: true,
        bypass_globs: ['/docs/**'],
        force_review_globs: ['/private/**'],
      },
      edit: {
        require_approval: true,
        bypass_globs: ['/tmp/**'],
        force_review_globs: [],
      },
    })

    expect(normalized.read).toEqual({
      require_approval: true,
      bypass_globs: ['/docs/**'],
      force_review_globs: ['/private/**'],
    })
    expect(normalized.write.require_approval).toBe(true)
    expect(normalized.write.bypass_globs).toEqual(['/data/**', '/tmp/**'])
  })
})
