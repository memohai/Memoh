#!/usr/bin/env node
import { startBridge } from '../src/bridge.mjs'

startBridge().catch((error) => {
  process.stdout.write(`${JSON.stringify({ type: 'error', error: error?.stack || String(error) })}\n`)
  process.exitCode = 1
})
