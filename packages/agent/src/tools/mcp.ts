import { HTTPMCPConnection, MCPConnection, SSEMCPConnection, StdioMCPConnection } from '../types'
import { createMCPClient } from '@ai-sdk/mcp'
import { AuthFetcher } from '../types'
import type { AgentAuthContext } from '../types/agent'

type MCPToolOptions = {
  botId?: string
  auth?: AgentAuthContext
  fetch?: AuthFetcher
}

export const getMCPTools = async (connections: MCPConnection[], options: MCPToolOptions = {}) => {
  const closeCallbacks: Array<() => Promise<void>> = []

  const getHTTPTools = async (connection: HTTPMCPConnection) => {
    const client = await createMCPClient({
      transport: {
        type: 'http',
        url: connection.url,
        headers: connection.headers,
      }
    })
    closeCallbacks.push(() => client.close())
    const tools = await client.tools()
    return tools
  }

  const getSSETools = async (connection: SSEMCPConnection) => {
    const client = await createMCPClient({
      transport: {
        type: 'sse',
        url: connection.url,
        headers: connection.headers,
      }
    })
    closeCallbacks.push(() => client.close())
    const tools = await client.tools()
    return tools
  }

  const getStdioTools = async (connection: StdioMCPConnection) => {
    if (!options.fetch || !options.botId || !options.auth) {
      throw new Error('stdio mcp requires auth fetcher and bot id')
    }
    const response = await options.fetch(`/bots/${options.botId}/mcp-stdio`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        name: connection.name,
        command: connection.command,
        args: connection.args ?? [],
        env: connection.env ?? {},
        cwd: connection.cwd ?? ''
      })
    })
    if (!response.ok) {
      const text = await response.text().catch(() => '')
      throw new Error(`mcp-stdio failed: ${response.status} ${text}`)
    }
    const data = await response.json().catch(() => ({})) as { url?: string }
    const rawUrl = typeof data?.url === 'string' ? data.url : ''
    if (!rawUrl) {
      throw new Error('mcp-stdio response missing url')
    }
    const baseUrl = options.auth.baseUrl ?? ''
    const url = rawUrl.startsWith('http')
      ? rawUrl
      : `${baseUrl.replace(/\/$/, '')}/${rawUrl.replace(/^\//, '')}`
    return await getHTTPTools({
      type: 'http',
      name: connection.name,
      url,
      headers: {
        'Authorization': `Bearer ${options.auth.bearer}`
      }
    })
  }

  const toolSets = await Promise.all(connections.map(async (connection) => {
    switch (connection.type) {
      case 'http':
        return getHTTPTools(connection)
      case 'sse':
        return getSSETools(connection)
      case 'stdio':
        return getStdioTools(connection)
      default:
        console.warn('unknown mcp connection type', connection)
        return {}
    }
  }))

  return {
    tools: Object.assign({}, ...toolSets),
    close: async () => {
      await Promise.all(closeCallbacks.map(callback => callback()))
    }
  }
}