import { describe, expect, it } from 'vitest'
import { getModelDescription, matchesModelSearch } from './model-description'

describe('model description helpers', () => {
  it('normalizes optional config descriptions', () => {
    expect(getModelDescription({ description: '  Fast coding model.  ' })).toBe('Fast coding model.')
    expect(getModelDescription({ description: '   ' })).toBeUndefined()
    expect(getModelDescription(undefined)).toBeUndefined()
  })

  it('matches a model by description', () => {
    expect(matchesModelSearch('coding', ['GPT', 'gpt-test', 'Fast coding model.'])).toBe(true)
    expect(matchesModelSearch('vision', ['GPT', 'gpt-test', 'Fast coding model.'])).toBe(false)
  })
})
