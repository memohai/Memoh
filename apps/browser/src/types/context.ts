import type { BrowserContext, Page } from 'playwright'
import type { BrowserContextConfig } from '../models'

export interface GatewayBrowserContext {
  id: string
  name: string
  context: BrowserContext
  config: BrowserContextConfig
  activePage?: Page
}
