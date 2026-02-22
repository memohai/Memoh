import { tool } from 'ai'
import { z } from 'zod'
import { Readability } from '@mozilla/readability'
import { JSDOM } from 'jsdom'
import TurndownService from 'turndown'

const turndownService = new TurndownService()

export const getWebTools = () => {
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
    'web_fetch': webFetch,
  }
}
