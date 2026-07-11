import { describe, expect, it } from 'vitest'
// This generated helper is intentionally not part of the SDK's public surface.
import { buildClientParams } from '../../../../packages/sdk/src/core/params.gen'

describe('generated SDK parameter isolation', () => {
  it('keeps slot parameters on a null prototype object', () => {
    const injectedPrototype = { isAdmin: true }
    const result = buildClientParams(
      [{ q: 'hello', $query___proto__: injectedPrototype }],
      [{ args: [{ in: 'query', key: 'q' }] }],
    )
    const query = result.query as Record<string, unknown>

    expect(Object.getPrototypeOf(query)).toBeNull()
    expect(Object.hasOwn(query, '__proto__')).toBe(true)
    expect(query.__proto__).toBe(injectedPrototype)
    expect(query.isAdmin).toBeUndefined()
    expect(Object.keys(query)).toEqual(['q', '__proto__'])
  })
})
