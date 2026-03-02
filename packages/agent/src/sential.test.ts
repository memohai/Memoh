import { describe, expect, it } from 'vitest'
import {
  createSential,
  createTextLoopGuard,
  createToolLoopGuard,
} from './sential'

describe('sential', () => {
  it('does not hit when overlap stays low', () => {
    const sential = createSential()
    sential.inspect('ABCDEFGHIJKLMNO')

    const result = sential.inspect('qrstuvwxyz12345')

    expect(result.hit).toBe(false)
    expect(result.overlap).toBe(0)
  })

  it('hits when overlap is above threshold', () => {
    const sential = createSential()
    sential.inspect('0123456789abcdefghij0123456789abcdefghij')

    const result = sential.inspect('0123456789abcdefghij')

    expect(result.hit).toBe(true)
    expect(result.overlap).toBeGreaterThan(0.75)
  })

  it('does not hit when overlap is exactly threshold', () => {
    const sential = createSential({
      ngramSize: 1,
      overlapThreshold: 0.75,
    })
    sential.inspect('aaaaaaaaaa')

    const result = sential.inspect(`${'a'.repeat(15)}bcdef`)

    expect(result.newGrams).toBe(20)
    expect(result.matchedGrams).toBe(15)
    expect(result.overlap).toBeCloseTo(0.75, 10)
    expect(result.hit).toBe(false)
  })

  it('evicts old grams with sliding window', () => {
    const sential = createSential({
      windowSize: 20,
    })
    sential.inspect('abcdefghijabcdefghij')
    sential.inspect('KLMNOPQRST')
    sential.inspect('UVWXYZ1234')

    const result = sential.inspect('abcdefghij')

    expect(result.hit).toBe(false)
    expect(result.matchedGrams).toBe(0)
  })

  it('aborts only after 10 consecutive hits', () => {
    const guard = createTextLoopGuard({
      ngramSize: 1,
      overlapThreshold: 0.5,
      consecutiveHitsToAbort: 10,
    })

    const seeded = guard.inspect('aaaaaaaaaa')
    expect(seeded.hit).toBe(false)
    expect(seeded.streak).toBe(0)
    expect(seeded.abort).toBe(false)

    for (let i = 1; i <= 9; i += 1) {
      const result = guard.inspect('aaaaaaaaaa')
      expect(result.hit).toBe(true)
      expect(result.streak).toBe(i)
      expect(result.abort).toBe(false)
    }

    const tenth = guard.inspect('aaaaaaaaaa')
    expect(tenth.hit).toBe(true)
    expect(tenth.streak).toBe(10)
    expect(tenth.abort).toBe(true)
  })

  it('resets streak when a non-hit chunk appears', () => {
    const guard = createTextLoopGuard({
      ngramSize: 1,
      overlapThreshold: 0.5,
      consecutiveHitsToAbort: 10,
    })

    guard.inspect('aaaaaaaaaa')
    for (let i = 0; i < 5; i += 1) {
      guard.inspect('aaaaaaaaaa')
    }

    const miss = guard.inspect('bcdefghijk')
    expect(miss.hit).toBe(false)
    expect(miss.streak).toBe(0)
    expect(miss.abort).toBe(false)

    const hitAgain = guard.inspect('aaaaaaaaaa')
    expect(hitAgain.hit).toBe(true)
    expect(hitAgain.streak).toBe(1)
    expect(hitAgain.abort).toBe(false)
  })

  it('only updates streak when chunk has enough new grams', () => {
    const guard = createTextLoopGuard({
      ngramSize: 1,
      overlapThreshold: 0.5,
      minNewGramsPerChunk: 5,
    })

    guard.inspect('aaaaaaaaaa')

    const smallHit = guard.inspect('aaaa')
    expect(smallHit.hit).toBe(true)
    expect(smallHit.newGrams).toBe(4)
    expect(smallHit.streak).toBe(0)
    expect(smallHit.abort).toBe(false)

    const countedHit = guard.inspect('aaaaa')
    expect(countedHit.hit).toBe(true)
    expect(countedHit.newGrams).toBe(5)
    expect(countedHit.streak).toBe(1)
    expect(countedHit.abort).toBe(false)

    const smallMiss = guard.inspect('cccc')
    expect(smallMiss.hit).toBe(false)
    expect(smallMiss.newGrams).toBe(4)
    expect(smallMiss.streak).toBe(1)

    const countedMiss = guard.inspect('ddddd')
    expect(countedMiss.hit).toBe(false)
    expect(countedMiss.newGrams).toBe(5)
    expect(countedMiss.streak).toBe(0)
  })

  it('warns on first tool-loop breach and aborts on second breach', () => {
    const guard = createToolLoopGuard({
      repeatThreshold: 5,
      warningsBeforeAbort: 1,
    })
    const payload = {
      toolName: 'web_fetch',
      input: { url: 'https://example.com', requestId: 'r-1' },
    }

    for (let i = 1; i <= 5; i += 1) {
      const result = guard.inspect(payload)
      expect(result.warn).toBe(false)
      expect(result.abort).toBe(false)
      expect(result.repeatCount).toBe(i)
    }

    const firstBreach = guard.inspect(payload)
    expect(firstBreach.warn).toBe(true)
    expect(firstBreach.abort).toBe(false)
    expect(firstBreach.breachCount).toBe(1)
    expect(firstBreach.repeatCount).toBe(0)

    for (let i = 1; i <= 5; i += 1) {
      const result = guard.inspect(payload)
      expect(result.warn).toBe(false)
      expect(result.abort).toBe(false)
      expect(result.repeatCount).toBe(i)
    }

    const secondBreach = guard.inspect(payload)
    expect(secondBreach.warn).toBe(false)
    expect(secondBreach.abort).toBe(true)
    expect(secondBreach.breachCount).toBe(2)
  })

  it('resets tool-loop repeat count when hash changes', () => {
    const guard = createToolLoopGuard({
      repeatThreshold: 5,
      warningsBeforeAbort: 1,
    })

    const first = guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://a.example.com' },
    })
    expect(first.repeatCount).toBe(1)

    const second = guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://a.example.com' },
    })
    expect(second.repeatCount).toBe(2)

    const changed = guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://b.example.com' },
    })
    expect(changed.repeatCount).toBe(1)
    expect(changed.warn).toBe(false)
    expect(changed.abort).toBe(false)
  })

  it('resets tool-loop breach count when hash changes', () => {
    const guard = createToolLoopGuard({
      repeatThreshold: 1,
      warningsBeforeAbort: 1,
    })

    // First fingerprint reaches warning phase.
    guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://a.example.com' },
    })
    const warned = guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://a.example.com' },
    })
    expect(warned.warn).toBe(true)
    expect(warned.breachCount).toBe(1)

    // Switching fingerprint should restart warning/abort phase.
    const switched = guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://b.example.com' },
    })
    expect(switched.warn).toBe(false)
    expect(switched.abort).toBe(false)
    expect(switched.breachCount).toBe(0)

    const warnedAgain = guard.inspect({
      toolName: 'web_fetch',
      input: { url: 'https://b.example.com' },
    })
    expect(warnedAgain.warn).toBe(true)
    expect(warnedAgain.abort).toBe(false)
    expect(warnedAgain.breachCount).toBe(1)
  })

  it('ignores volatile keys when computing tool-loop hash', () => {
    const guard = createToolLoopGuard({
      repeatThreshold: 1,
      warningsBeforeAbort: 1,
    })

    const first = guard.inspect({
      toolName: 'web_fetch',
      input: {
        url: 'https://example.com',
        request_id: 'req-1',
        updatedAt: '2026-02-28T00:00:00.000Z',
      },
    })
    expect(first.warn).toBe(false)
    expect(first.abort).toBe(false)

    const second = guard.inspect({
      toolName: 'web_fetch',
      input: {
        url: 'https://example.com',
        request_id: 'req-2',
        updatedAt: '2026-02-28T00:01:00.000Z',
      },
    })
    expect(second.warn).toBe(true)
    expect(second.abort).toBe(false)
  })
})
