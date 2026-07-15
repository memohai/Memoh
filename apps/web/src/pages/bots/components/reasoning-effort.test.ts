import { describe, expect, it } from 'vitest'
import { resolveEffortLevels } from './reasoning-effort'

describe('resolveEffortLevels', () => {
  it('preserves max for Codex and filters client-only efforts', () => {
    expect(resolveEffortLevels({
      reasoning_efforts: ['low', 'xhigh', 'max', 'ultra'],
    }, 'openai-codex')).toEqual(['low', 'xhigh', 'max'])
  })

  it('filters max for generic OpenAI-format clients', () => {
    expect(resolveEffortLevels({
      reasoning_efforts: ['low', 'xhigh', 'max'],
    }, 'openai-responses')).toEqual(['low', 'xhigh'])
  })
})
