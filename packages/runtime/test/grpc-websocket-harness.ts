import { createServer, type Socket } from 'node:net'

import WebSocket, { createWebSocketStream, WebSocketServer } from 'ws'

import { bridgeWebSocketToGrpc } from '../src/pipe/grpc-websocket'
import type { RunningRuntimeGrpcServer } from '../src/service'
import { runtimeProtocolGrpc } from '../src/session'

export interface GrpcWebSocketTestHarness {
  target: string
  disconnect(): void
  close(): Promise<void>
}

/**
 * Exposes the server side of the real Runtime WebSocket to a grpc-js client.
 * The TCP listener exists only in tests, where it stands in for Memoh's
 * server-side gRPC client. Production runtime code never opens a listener.
 */
export async function createGrpcWebSocketTestHarness(
  grpc: RunningRuntimeGrpcServer,
): Promise<GrpcWebSocketTestHarness> {
  const websocketServer = new WebSocketServer({
    port: 0,
    handleProtocols: protocols => protocols.has(runtimeProtocolGrpc) ? runtimeProtocolGrpc : false,
  })
  await onceListening(websocketServer)
  const websocketPort = (websocketServer.address() as { port: number }).port
  const serverWebSocketPromise = new Promise<WebSocket>(resolve => websocketServer.once('connection', resolve))
  const runtimeWebSocket = new WebSocket(`ws://127.0.0.1:${websocketPort}`, runtimeProtocolGrpc)
  runtimeWebSocket.on('error', () => undefined)
  await onceOpen(runtimeWebSocket)
  const serverWebSocket = await serverWebSocketPromise
  serverWebSocket.on('error', () => undefined)
  const bridge = bridgeWebSocketToGrpc(runtimeWebSocket, grpc)

  const websocketStream = createWebSocketStream(serverWebSocket, { encoding: undefined })
  websocketStream.on('error', () => undefined)
  const sockets = new Set<Socket>()
  const relay = createServer(socket => {
    sockets.add(socket)
    socket.once('close', () => sockets.delete(socket))
    socket.on('error', () => undefined)
    socket.pipe(websocketStream)
    websocketStream.pipe(socket)
  })
  relay.listen(0, '127.0.0.1')
  await onceListening(relay)
  const relayPort = (relay.address() as { port: number }).port
  let closed = false
  const disconnect = () => {
    for (const socket of sockets) {
      socket.destroy()
    }
    websocketStream.destroy()
    serverWebSocket.terminate()
    runtimeWebSocket.terminate()
  }

  return {
    target: `127.0.0.1:${relayPort}`,
    disconnect,
    async close() {
      if (closed) {
        return
      }
      closed = true
      disconnect()
      await Promise.allSettled([
        bridge,
        closeServer(relay),
        closeServer(websocketServer),
      ])
    },
  }
}

function onceListening(server: NodeJS.EventEmitter): Promise<void> {
  return new Promise((resolve, reject) => {
    server.once('listening', resolve)
    server.once('error', reject)
  })
}

function onceOpen(websocket: WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    websocket.once('open', resolve)
    websocket.once('error', reject)
  })
}

function closeServer(server: { close(callback: (error?: Error) => void): unknown }): Promise<void> {
  return new Promise(resolve => server.close(() => resolve()))
}
