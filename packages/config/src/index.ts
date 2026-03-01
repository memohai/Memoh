import { parse } from 'toml'
import { readFileSync } from 'fs'
import type { Config } from './types.ts'

export const loadConfig = (path: string = './config.toml'): Config => {
  const config = parse(readFileSync(path, 'utf-8'))
  return config satisfies Config
}

export const getBaseUrl = (config: Config) => {
  const rawAddr = (config.agent_gateway?.server_addr || config.server?.addr || '').trim()

  if (!rawAddr) {
    return 'http://127.0.0.1'
  }

  if (rawAddr.startsWith('http://') || rawAddr.startsWith('https://')) {
    return rawAddr.replace(/\/+$/, '')
  }

  if (rawAddr.startsWith(':')) {
    return `http://127.0.0.1${rawAddr}`
  }

  return `http://${rawAddr}`
}

export * from './types.ts'
