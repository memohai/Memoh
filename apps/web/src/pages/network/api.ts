import { client } from '@memohai/sdk/client'

export interface OverlayConfigSchemaField {
  key: string
  type: 'string' | 'secret' | 'number' | 'bool' | 'enum' | 'textarea'
  required?: boolean
  title?: string
  description?: string
  placeholder?: string
  default?: unknown
  example?: unknown
  order?: number
  enum?: string[]
  multiline?: boolean
  readonly?: boolean
  secret?: boolean
  collapsed?: boolean
  constraint?: { min?: number; max?: number; step?: number } | null
}

export interface OverlayConfigSchema {
  version?: number
  title?: string
  fields?: OverlayConfigSchemaField[]
}

export interface OverlayProviderAction {
  id: string
  type: string
  label: string
  description?: string
  primary?: boolean
  status?: { enabled: boolean; reason?: string } | null
}

export interface WorkspaceRuntimeStatus {
  state: string
  container_id?: string
  task_status?: string
  pid?: number
  netns_path?: string
  message?: string
}

export interface NetworkBotStatus {
  provider?: string
  attached?: boolean
  state: string
  title?: string
  description?: string
  message?: string
  network_ip?: string
  proxy_address?: string
  details?: Record<string, unknown> | null
  workspace?: WorkspaceRuntimeStatus | null
}

export interface OverlayProviderMeta {
  kind: string
  display_name: string
  description?: string
  config_schema?: OverlayConfigSchema
  binding_config_schema?: OverlayConfigSchema
  capabilities?: Record<string, boolean>
  actions?: OverlayProviderAction[]
}

export interface OverlayProviderActionExecution {
  action_id: string
  status: NetworkBotStatus
  output?: Record<string, unknown>
}

export interface NetworkNodeOption {
  id: string
  value: string
  display_name: string
  description?: string
  online?: boolean
  addresses?: string[]
  can_exit_node?: boolean
  selected?: boolean
  details?: Record<string, unknown> | null
}

export interface NetworkNodeListResponse {
  provider?: string
  items?: NetworkNodeOption[]
  message?: string
}

async function request<T>(method: 'GET' | 'POST', path: string, body?: unknown): Promise<T> {
  const { data, response, error } = await client[method === 'GET' ? 'get' : 'post']({
    url: path,
    ...(body !== undefined ? { body: body as Record<string, unknown> } : {}),
  })
  if (error || !response.ok) {
    const text = typeof error === 'string' ? error : (error as Record<string, unknown>)?.message as string | undefined
    throw new Error(text || `request failed: ${response.status}`)
  }
  return data as T
}

export function listOverlayProviderMeta() {
  return request<OverlayProviderMeta[]>('GET', '/network/meta')
}

export function getBotNetworkStatus(botID: string) {
  return request<NetworkBotStatus>('GET', `/bots/${botID}/network/status`)
}

export function listBotNetworkNodes(botID: string) {
  return request<NetworkNodeListResponse>('GET', `/bots/${botID}/network/nodes`)
}

export function executeBotNetworkAction(botID: string, actionID: string, body: Record<string, unknown>) {
  return request<OverlayProviderActionExecution>('POST', `/bots/${botID}/network/actions/${actionID}`, body)
}
