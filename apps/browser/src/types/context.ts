import type { BrowserContext, Page } from 'playwright'
import type { BrowserContextConfig } from '../models'
import type { BrowserCore } from '../browser'

export interface GatewayBrowserContext {
  id: string
  name: string
  core: BrowserCore
  context: BrowserContext
  config: BrowserContextConfig
  activePage?: Page
}
