import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'display-pane.vue'), 'utf8')

function connectSource() {
  const start = source.indexOf('async function connect()')
  const end = source.indexOf('function handleVisibilityChange()')
  if (start < 0 || end < 0) {
    throw new Error('display-pane.vue connect() source not found')
  }
  return source.slice(start, end)
}

describe('display pane connection timeout', () => {
  it('starts the 15s WebRTC timeout only after display preparation finishes', () => {
    const connect = connectSource()
    const timeoutIndex = connect.indexOf('startConnectTimeout(attempt)')
    const prepareIndex = connect.indexOf('if (canPrepareDisplay(info))')
    const peerIndex = connect.indexOf('const next = new RTCPeerConnection()')

    expect(prepareIndex).toBeGreaterThan(-1)
    expect(timeoutIndex).toBeGreaterThan(prepareIndex)
    expect(timeoutIndex).toBeLessThan(peerIndex)
    expect(connect.slice(0, timeoutIndex)).not.toContain('startConnectTimeout(attempt)')
  })
})
