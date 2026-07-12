import { afterEach, describe, expect, it } from 'vitest'
import WebSocket, { WebSocketServer } from 'ws'

import { bridgeWebSocketToGrpc } from '../src/pipe/grpc-websocket'

const cleanups: Array<() => Promise<void>> = []

afterEach(async () => {
  await Promise.allSettled(cleanups.splice(0).reverse().map(cleanup => cleanup()))
})

describe('gRPC over WebSocket pipe', () => {
  it('attaches before awaited state I/O and preserves the peer first frame', async () => {
    const firstFrame = Buffer.from('immediate-http2-preface-fixture')
    const websocketServer = new WebSocketServer({ port: 0 })
    await new Promise<void>(resolve => websocketServer.once('listening', resolve))
    cleanups.push(() => new Promise(resolve => websocketServer.close(() => resolve())))
    const port = (websocketServer.address() as { port: number }).port
    websocketServer.once('connection', peer => peer.send(firstFrame))

    let resolveFrame!: (value: Buffer) => void
    const received = new Promise<Buffer>(resolve => { resolveFrame = resolve })
    const runtimeWebSocket = new WebSocket(`ws://127.0.0.1:${port}`)
    runtimeWebSocket.on('error', () => undefined)
    cleanups.push(async () => runtimeWebSocket.terminate())
    const bridge = bridgeWebSocketToGrpc(runtimeWebSocket, {
      acceptConnection(connection) {
        connection.once('data', chunk => resolveFrame(Buffer.from(chunk)))
      },
    })
    void bridge.catch(() => undefined)
    await new Promise<void>((resolve, reject) => {
      runtimeWebSocket.once('open', resolve)
      runtimeWebSocket.once('error', reject)
    })

    // This stands in for the connected-state file write in RuntimeSession.
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(await received).toEqual(firstFrame)
    runtimeWebSocket.terminate()
    await bridge
  })
})
