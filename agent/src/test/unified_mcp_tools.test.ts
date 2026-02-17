import { describe, expect, test } from 'bun:test'
import { getMCPTools } from '../tools/mcp'

describe('getMCPTools (unified endpoint)', () => {
  test('loads tools from unified MCP HTTP endpoint', async () => {
    const seenMethods: string[] = []
    const seenAuthHeaders: string[] = []

    const server = Bun.serve({
      port: 0,
      async fetch(request) {
        seenAuthHeaders.push(request.headers.get('authorization') ?? '')
        const body = await request.json().catch(() => ({} as Record<string, unknown>))
        const method = typeof body?.method === 'string' ? body.method : ''
        seenMethods.push(method)

        if (method === 'initialize') {
          return Response.json({
            jsonrpc: '2.0',
            id: body.id ?? null,
            result: {
              protocolVersion: '2025-06-18',
              capabilities: {
                tools: {
                  listChanged: false,
                },
              },
              serverInfo: {
                name: 'test-mcp',
                version: '1.0.0',
              },
            },
          })
        }

        if (method === 'notifications/initialized') {
          return new Response(null, { status: 202 })
        }

        if (method === 'tools/list') {
          return Response.json({
            jsonrpc: '2.0',
            id: body.id ?? null,
            result: {
              tools: [
                {
                  name: 'search_memory',
                  description: 'Search memory',
                  inputSchema: {
                    type: 'object',
                    properties: {
                      query: { type: 'string' },
                    },
                    required: ['query'],
                  },
                },
              ],
            },
          })
        }

        return Response.json({
          jsonrpc: '2.0',
          id: body.id ?? null,
          error: {
            code: -32601,
            message: 'method not found',
          },
        })
      },
    })

    try {
      const endpoint = `http://127.0.0.1:${server.port}/bots/bot-1/tools`
      const { tools, close } = await getMCPTools([{
        type: 'http',
        name: 'builtin',
        url: endpoint,
        headers: {
          Authorization: 'Bearer test-token',
        },
      }])

      expect(Object.keys(tools)).toContain('search_memory')
      expect(seenMethods).toContain('initialize')
      expect(seenMethods).toContain('tools/list')
      expect(seenAuthHeaders.some(value => value === 'Bearer test-token')).toBe(true)

      await close()
    } finally {
      server.stop(true)
    }
  })
})
