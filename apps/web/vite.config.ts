import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { createRequire } from 'module'
import { fileURLToPath } from 'url'

// https://vite.dev/config/
export default defineConfig(({ command }) => {
  const require = createRequire(import.meta.url)
  const defaultPort = 8082
  const defaultHost = '127.0.0.1'
  const defaultApiBaseUrl = process.env.VITE_API_URL ?? 'http://localhost:8080'
  const configuredProxyTarget = process.env.MEMOH_WEB_PROXY_TARGET?.trim()
  const configuredPath = process.env.MEMOH_CONFIG_PATH?.trim() || process.env.CONFIG_PATH?.trim()
  const configPath = configuredPath && configuredPath.length > 0 ? configuredPath : '../../config.toml'

  let port = defaultPort
  let host = defaultHost
  let baseUrl = configuredProxyTarget || defaultApiBaseUrl

  if (command !== 'build') {
    try {
      const { loadConfig, getBaseUrl } = require('@memoh/config') as {
        loadConfig: (path: string) => {
          web?: { port?: number; host?: string }
        }
        getBaseUrl: (config: unknown) => string
      }
      let config
      try {
        config = loadConfig(configPath)
      } catch {
        config = loadConfig('../../conf/app.docker.toml')
      }
      port = config.web?.port ?? defaultPort
      host = config.web?.host ?? defaultHost
      baseUrl = configuredProxyTarget || getBaseUrl(config)
    } catch {
      // Fall back to env/default values when config.toml is unavailable.
    }
  }

  return {
    plugins: [vue(), tailwindcss()],
    optimizeDeps: {
      // Pre-bundle deps for route pages to avoid slow first load / navigation
      entries: ['src/main.ts', 'src/pages/**/*.vue'],
    },
    server: {
      port,
      host,
      proxy: {
        '/api': {
          target: baseUrl,
          changeOrigin: true,
          rewrite: (path: string) => path.replace(/^\/api/, ''),
          ws: true,
        }
      },
    },
    preview: {
      port,
      host: '0.0.0.0',
      proxy: {
        '/api': {
          target: baseUrl,
          changeOrigin: true,
          rewrite: (path: string) => path.replace(/^\/api/, ''),
          ws: true,
        }
      },
      allowedHosts: true,
    },
    resolve: {
      alias: {
        '#': fileURLToPath(new URL('../../packages/ui/src', import.meta.url)),
        '@': fileURLToPath(new URL('./src', import.meta.url))
      },
    },
  }
})
