// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ONBOARDING_KEYS } from '../constants'
import { clearACPSelection, readACPSelection, writeACPSelection } from './useACPSetup'

const STORAGE_KEY = ONBOARDING_KEYS.acpSelection

describe('useACPSetup', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  afterEach(() => {
    sessionStorage.clear()
  })

  describe('readACPSelection', () => {
    it('returns null when nothing is stored', () => {
      expect(readACPSelection()).toBeNull()
    })

    it('returns null for empty string', () => {
      sessionStorage.setItem(STORAGE_KEY, '')
      expect(readACPSelection()).toBeNull()
    })

    it('returns null for invalid JSON', () => {
      sessionStorage.setItem(STORAGE_KEY, '{not-json}')
      expect(readACPSelection()).toBeNull()
    })

    it('returns null when agentId is missing or empty', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ setupMode: 'api_key' }))
      expect(readACPSelection()).toBeNull()
    })

    it('returns null when agentId normalizes to empty string', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ agentId: '   ', setupMode: 'api_key' }))
      expect(readACPSelection()).toBeNull()
    })

    it('parses a valid codex selection', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
        agentId: 'codex',
        setupMode: 'api_key',
        managed: { api_key: 'sk-test' },
      }))
      const result = readACPSelection()
      expect(result).toEqual({
        agentId: 'codex',
        setupMode: 'api_key',
        managed: { api_key: 'sk-test' },
      })
    })

    it('normalizes agentId to lowercase and trimmed', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ agentId: '  CODEX  ', setupMode: 'oauth' }))
      const result = readACPSelection()
      expect(result?.agentId).toBe('codex')
    })

    it('falls back to api_key setupMode when setupMode is missing', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ agentId: 'claude-code' }))
      const result = readACPSelection()
      expect(result?.setupMode).toBe('api_key')
    })

    it('falls back to api_key setupMode when setupMode is empty string', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ agentId: 'codex', setupMode: '' }))
      const result = readACPSelection()
      expect(result?.setupMode).toBe('api_key')
    })

    it('returns empty managed object when managed is missing', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ agentId: 'codex', setupMode: 'oauth' }))
      const result = readACPSelection()
      expect(result?.managed).toEqual({})
    })

    it('coerces non-string managed values to strings', () => {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
        agentId: 'codex',
        setupMode: 'api_key',
        managed: { count: 42, flag: true, nil: null },
      }))
      const result = readACPSelection()
      expect(result?.managed).toEqual({ count: '42', flag: 'true', nil: '' })
    })
  })

  describe('writeACPSelection', () => {
    it('persists selection to sessionStorage', () => {
      writeACPSelection({ agentId: 'codex', setupMode: 'oauth', managed: {} })
      const raw = sessionStorage.getItem(STORAGE_KEY)
      expect(raw).not.toBeNull()
      const parsed = JSON.parse(raw!)
      expect(parsed.agentId).toBe('codex')
      expect(parsed.setupMode).toBe('oauth')
    })

    it('round-trips through readACPSelection', () => {
      const selection = { agentId: 'claude-code', setupMode: 'api_key', managed: { api_key: 'ant-key' } }
      writeACPSelection(selection)
      expect(readACPSelection()).toEqual(selection)
    })

    it('overwrites a previous selection', () => {
      writeACPSelection({ agentId: 'codex', setupMode: 'oauth', managed: {} })
      writeACPSelection({ agentId: 'claude-code', setupMode: 'api_key', managed: { api_key: 'x' } })
      expect(readACPSelection()?.agentId).toBe('claude-code')
    })
  })

  describe('clearACPSelection', () => {
    it('removes the stored selection', () => {
      writeACPSelection({ agentId: 'codex', setupMode: 'api_key', managed: {} })
      clearACPSelection()
      expect(readACPSelection()).toBeNull()
      expect(sessionStorage.getItem(STORAGE_KEY)).toBeNull()
    })

    it('is a no-op when nothing is stored', () => {
      expect(() => clearACPSelection()).not.toThrow()
      expect(readACPSelection()).toBeNull()
    })
  })

  describe('onboarding step skip / ACP selection cleanup', () => {
    it('clearACPSelection after write leaves no trace in sessionStorage', () => {
      writeACPSelection({ agentId: 'codex', setupMode: 'oauth', managed: {} })
      expect(readACPSelection()).not.toBeNull()
      clearACPSelection()
      expect(readACPSelection()).toBeNull()
    })

    it('write then clear then write gives the new selection', () => {
      writeACPSelection({ agentId: 'codex', setupMode: 'api_key', managed: { api_key: 'old' } })
      clearACPSelection()
      writeACPSelection({ agentId: 'claude-code', setupMode: 'oauth', managed: {} })
      const result = readACPSelection()
      expect(result?.agentId).toBe('claude-code')
      expect(result?.setupMode).toBe('oauth')
    })

    // Regression: Step 3 main CTA previously called leave(nextStep) directly,
    // bypassing clearACPSelection(). This test documents the expected contract:
    // onSkipStep must clear the selection before advancing.
    it('simulates onSkipStep: clear before advancing clears stale ACP selection', () => {
      // User selected ACP in a previous visit to Step 3
      writeACPSelection({ agentId: 'codex', setupMode: 'oauth', managed: {} })

      // onSkipStep() calls clearACPSelection() then leave(nextStep)
      clearACPSelection()

      // Step 4 reads no selection
      expect(readACPSelection()).toBeNull()
    })
  })
})
