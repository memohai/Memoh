import { tool } from 'ai'
import { z } from 'zod'
import { Readability } from '@mozilla/readability'
import { JSDOM } from 'jsdom'
import TurndownService from 'turndown'
import { BraveConfig } from '../types'

const turndownService = new TurndownService()

interface WebToolParams {
  brave: BraveConfig
}

interface BraveSearchResult {
  type: string
  title: string
  url: string
  description?: string
  age?: string
}

interface BraveSearchResponse {
  web?: {
    results: BraveSearchResult[]
  }
}

export const getWebTools = ({ brave }: WebToolParams) => {
  const { apiKey, baseUrl = 'https://api.search.brave.com/res/v1/' } = brave
  const webSearch = tool({
    description: 'Search the web for information using Brave Search API. Use this when you need current information, facts, news, or any web content.',
    inputSchema: z.object({
      query: z.string().describe('The search query to look up on the web'),
      count: z.number().optional().describe('Number of results to return (default: 10, max: 20)'),
    }),
    execute: async ({ query, count = 10 }) => {
      try {
        const url = new URL('web/search', baseUrl)
        url.searchParams.append('q', query)
        url.searchParams.append('count', Math.min(count, 20).toString())
        
        const response = await fetch(url.toString(), {
          method: 'GET',
          headers: {
            'Accept': 'application/json',
            'Accept-Encoding': 'gzip',
            'X-Subscription-Token': apiKey,
          },
        })

        if (!response.ok) {
          const errorText = await response.text()
          console.error('[Web Search] error', {
            type: 'web_search',
            query,
            count,
            status: response.status,
            statusText: response.statusText,
            error: errorText,
          })
          throw new Error(`Brave Search API error: ${response.status} ${response.statusText}`)
        }

        const data: BraveSearchResponse = await response.json()
        
        const results = data.web?.results || []

        if (results.length === 0) {
          return {
            success: false,
            message: 'No results found for the query',
            query,
          }
        }

        return {
          success: true,
          query,
          results: results.map((result) => ({
            title: result.title,
            url: result.url,
            description: result.description,
            age: result.age,
          })),
        }
      } catch (error) {
        return {
          success: false,
          error: error instanceof Error ? error.message : 'Unknown error occurred',
          query,
        }
      }
    },
  })

  const webFetch = tool({
    description: 'Fetch a URL and convert the response to readable content. Supports HTML (converts to Markdown), JSON, XML, and plain text formats.',
    inputSchema: z.object({
      url: z.string().describe('The URL to fetch'),
      format: z.enum(['auto', 'markdown', 'json', 'xml', 'text']).optional().describe('Output format (default: auto - detects from content type)'),
    }),
    execute: async ({ url, format = 'auto' }) => {
      try {
        const response = await fetch(url, {
          headers: {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36',
          },
        })

        if (!response.ok) {
          throw new Error(`HTTP error: ${response.status} ${response.statusText}`)
        }

        const contentType = response.headers.get('content-type') || ''
        let detectedFormat = format

        // Auto-detect format from content type
        if (format === 'auto') {
          if (contentType.includes('application/json')) {
            detectedFormat = 'json'
          } else if (contentType.includes('application/xml') || contentType.includes('text/xml')) {
            detectedFormat = 'xml'
          } else if (contentType.includes('text/html')) {
            detectedFormat = 'markdown'
          } else {
            detectedFormat = 'text'
          }
        }

        const content = await response.text()

        // Process based on format
        switch (detectedFormat) {
          case 'json': {
            try {
              const jsonData = JSON.parse(content)
              return {
                success: true,
                url,
                format: 'json',
                contentType,
                data: jsonData,
              }
            } catch {
              return {
                success: false,
                error: 'Failed to parse JSON',
                url,
              }
            }
          }

          case 'xml': {
            return {
              success: true,
              url,
              format: 'xml',
              contentType,
              content,
            }
          }

          case 'markdown': {
            try {
              const dom = new JSDOM(content, { url })
              const reader = new Readability(dom.window.document)
              const article = reader.parse()

              if (!article || !article.content) {
                return {
                  success: false,
                  error: 'Failed to extract readable content from HTML',
                  url,
                }
              }

              const markdown = turndownService.turndown(article.content)

              return {
                success: true,
                url,
                format: 'markdown',
                contentType,
                title: article.title,
                byline: article.byline,
                excerpt: article.excerpt,
                content: markdown,
                textContent: article.textContent?.substring(0, 500), // First 500 chars as preview
                length: article.length,
              }
            } catch (error) {
              return {
                success: false,
                error: error instanceof Error ? error.message : 'Failed to process HTML',
                url,
              }
            }
          }

          case 'text':
          default: {
            return {
              success: true,
              url,
              format: 'text',
              contentType,
              content: content.substring(0, 10000), // Limit to 10KB
              length: content.length,
            }
          }
        }
      } catch (error) {
        return {
          success: false,
          error: error instanceof Error ? error.message : 'Unknown error occurred',
          url,
        }
      }
    },
  })

  return {
    'web_search': webSearch,
    'web_fetch': webFetch,
  }
}