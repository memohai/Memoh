import { chromium, firefox } from 'playwright'
import type { Browser } from 'playwright'

export type BrowserCore = 'chromium' | 'firefox'

export const browsers = new Map<BrowserCore, Browser>()

export const initBrowsers = async (): Promise<Map<BrowserCore, Browser>> => {
  const raw = process.env.BROWSER_CORES ?? 'chromium'
  const cores = raw.split(',').map(s => s.trim()) as BrowserCore[]

  for (const core of cores) {
    if (core === 'chromium') {
      browsers.set('chromium', await chromium.launch({ headless: true }))
    } else if (core === 'firefox') {
      browsers.set('firefox', await firefox.launch({ headless: true }))
    }
  }

  if (browsers.size === 0) {
    browsers.set('chromium', await chromium.launch({ headless: true }))
  }

  return browsers
}

export const getBrowser = (core: BrowserCore = 'chromium'): Browser => {
  const b = browsers.get(core) ?? browsers.values().next().value
  if (!b) throw new Error(`Browser core "${core}" is not available`)
  return b
}

export const getAvailableCores = (): BrowserCore[] => {
  const raw = process.env.BROWSER_CORES ?? 'chromium'
  return raw.split(',').map(s => s.trim()) as BrowserCore[]
}
