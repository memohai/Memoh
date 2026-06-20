import { parseBrowserAddress, type BrowserAddress } from '@/utils/browser-address'

// Detect/normalize links that point at a container-local dev server so they can
// be opened in the workspace `browser` panel instead of the user's OS browser
// (the container's localhost is NOT the user's localhost). Reuses the strict
// `parseBrowserAddress` validator, which only admits localhost / 127.0.0.1 / ::1
// with an explicit port — so a bare `localhost` is intentionally rejected.

export function tryParseLocalhostHref(raw: string | null | undefined): BrowserAddress | null {
  if (!raw) return null
  try {
    return parseBrowserAddress(raw)
  } catch {
    return null
  }
}

// Scans free-form terminal output for local URLs. A port is required (`:\d+`) so
// prose words like "localhost" alone never match. Matches accept an optional
// http(s):// scheme and an optional path. Each hit is re-validated through
// `tryParseLocalhostHref` before use.
export const LOCALHOST_URL_REGEX
  = /(?:https?:\/\/)?(?:localhost|127\.0\.0\.1|\[::1\])(?::\d+)(?:\/[^\s)\]]*)?/gi
