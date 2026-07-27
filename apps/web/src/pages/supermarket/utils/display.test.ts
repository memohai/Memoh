import { describe, expect, it } from 'vitest'
import {
  formatNamespacedSkillName,
  registryDisplayPrefix,
} from './display'

describe('registryDisplayPrefix', () => {
  it('uses the registry id, not the marketing display name', () => {
    expect(registryDisplayPrefix('openai-api-curated', 'OpenAI API Curated Skills')).toBe('openai-api-curated')
    expect(registryDisplayPrefix('memoh', 'Memoh Skills')).toBe('memoh')
  })

  it('returns empty when the registry id is missing', () => {
    expect(registryDisplayPrefix()).toBe('')
    expect(registryDisplayPrefix('  ')).toBe('')
  })
})

describe('formatNamespacedSkillName', () => {
  it('prefixes the skill name with the registry id', () => {
    expect(formatNamespacedSkillName({ name: 'xlsx', skill_id: 'xlsx' }, 'openai-api-curated'))
      .toBe('openai-api-curated/xlsx')
  })
})
