export interface BaseMCPConnection {
  type: string
  name: string
}

export interface StdioMCPConnection extends BaseMCPConnection {
  type: 'stdio'
  command: string
  args: string[]
  env?: Record<string, string>
  cwd?: string
}

export interface HTTPMCPConnection extends BaseMCPConnection {
  type: 'http'
  url: string
  headers?: Record<string, string>
}

export interface SSEMCPConnection extends BaseMCPConnection {
  type: 'sse'
  url: string
  headers?: Record<string, string>
}

export type MCPConnection =
  | StdioMCPConnection
  | HTTPMCPConnection
  | SSEMCPConnection