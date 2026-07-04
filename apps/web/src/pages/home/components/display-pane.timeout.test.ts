import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../../../composables/useBotDisplayConnection.ts'),
  'utf8',
)

function acquireSource() {
  const start = source.indexOf('private async doAcquire()')
  const end = source.indexOf('private async createPeer()')
  if (start < 0 || end < 0) {
    throw new Error('useBotDisplayConnection.ts doAcquire() source not found')
  }
  return source.slice(start, end)
}

describe('display pane connection timeout', () => {
  it('creates the WebRTC peer only after display preparation finishes', () => {
    const acquire = acquireSource()
    const ensureIndex = acquire.indexOf('const ready = await this.ensureReady()')
    const peerIndex = acquire.indexOf('await this.createPeer()')

    expect(ensureIndex).toBeGreaterThan(-1)
    expect(peerIndex).toBeGreaterThan(ensureIndex)
  })
})
