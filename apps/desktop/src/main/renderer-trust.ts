import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export type RendererTrustOptions = {
  devBaseUrl?: string
  productionEntry: string
}

export function isTrustedRendererUrl(rawUrl: string, options: RendererTrustOptions): boolean {
  let candidate: URL
  try {
    candidate = new URL(rawUrl)
  } catch {
    return false
  }

  const devBaseUrl = options.devBaseUrl?.trim()
  if (devBaseUrl) {
    try {
      const entry = new URL('index.html', `${devBaseUrl.replace(/\/+$/, '')}/`)
      return candidate.origin === entry.origin && candidate.pathname === entry.pathname
    } catch {
      return false
    }
  }

  if (candidate.protocol !== 'file:') return false
  try {
    return resolve(fileURLToPath(candidate)) === resolve(options.productionEntry)
  } catch {
    return false
  }
}
