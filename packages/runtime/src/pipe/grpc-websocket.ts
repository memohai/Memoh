import WebSocket, { createWebSocketStream } from 'ws'

import type { RunningRuntimeGrpcServer } from '../service'

export function bridgeWebSocketToGrpc(
  websocket: WebSocket,
  grpc: Pick<RunningRuntimeGrpcServer, 'acceptConnection'>,
): Promise<void> {
  const stream = createWebSocketStream(websocket, { encoding: undefined })

  return new Promise<void>((resolve, reject) => {
    let settled = false
    const finish = (error?: Error) => {
      if (settled) {
        return
      }
      settled = true
      websocket.off('open', inject)
      stream.destroy()
      if (websocket.readyState === WebSocket.OPEN || websocket.readyState === WebSocket.CONNECTING) {
        websocket.terminate()
      }
      if (error) {
        reject(error)
      } else {
        resolve()
      }
    }
    websocket.once('close', () => finish())
    websocket.once('error', error => finish(error))
    stream.once('close', () => finish())
    stream.once('error', error => finish(error))
    const inject = () => {
      try {
        grpc.acceptConnection(stream)
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)))
      }
    }
    if (websocket.readyState === WebSocket.OPEN) {
      inject()
    } else if (websocket.readyState === WebSocket.CONNECTING) {
      // Register before RuntimeSession awaits the open promise. This listener
      // attaches/injects in the WebSocket open turn, before the ws parser can
      // deliver a peer's immediately-following first binary frame.
      websocket.once('open', inject)
    } else {
      finish(new Error('runtime WebSocket closed before gRPC injection'))
    }
  })
}
