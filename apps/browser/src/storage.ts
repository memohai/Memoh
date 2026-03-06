import { GatewayBrowserContext } from './types'

export type GatewayStorage = Map<string, GatewayBrowserContext>

export const storage: GatewayStorage = new Map()