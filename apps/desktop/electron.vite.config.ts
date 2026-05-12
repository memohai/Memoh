import { defineConfig, externalizeDepsPlugin } from 'electron-vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'

const require = createRequire(import.meta.url)

const defaultPort = 8082
const defaultHost = '127.0.0.1'
const defaultApiBaseUrl = process.env.VITE_API_URL ?? 'http://localhost:8080'

function resolveProxyTarget(command: 'build' | 'serve'): { port: number; host: string; baseUrl: string } {
  const configuredProxyTarget = process.env.MEMOH_WEB_PROXY_TARGET?.trim()
  const configuredPath = process.env.MEMOH_CONFIG_PATH?.trim() || process.env.CONFIG_PATH?.trim()
  const configPath = configuredPath && configuredPath.length > 0 ? configuredPath : '../../config.toml'

  let port = defaultPort
  let host = defaultHost
  let baseUrl = configuredProxyTarget || defaultApiBaseUrl

  if (command !== 'build') {
    try {
      const { loadConfig, getBaseUrl } = require('@memohai/config') as {
        loadConfig: (path: string) => { web?: { port?: number; host?: string } }
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
      // fall back to env/default values when config.toml is unavailable.
    }
  }

  return { port, host, baseUrl }
}

export default defineConfig(({ command }) => {
  const { port, host, baseUrl } = resolveProxyTarget(command)
  const bundledElectronToolkit = ['@electron-toolkit/preload', '@electron-toolkit/utils']

  return {
    main: {
      plugins: [externalizeDepsPlugin({ exclude: bundledElectronToolkit })],
    },
    preload: {
      plugins: [externalizeDepsPlugin({ exclude: bundledElectronToolkit })],
    },
    renderer: {
      root: resolve(__dirname, 'src/renderer'),
      // Reuse apps/web/public so absolute-path assets (e.g. /logo.svg) resolve
      // when web modules are imported directly from the desktop renderer.
      publicDir: resolve(__dirname, '../web/public'),
      plugins: [vue(), tailwindcss()],
      resolve: {
        alias: {
          '@renderer': fileURLToPath(new URL('./src/renderer/src', import.meta.url)),
          // match apps/web/vite.config.ts aliases so imported web modules resolve correctly.
          '@': fileURLToPath(new URL('../web/src', import.meta.url)),
          '#': fileURLToPath(new URL('../../packages/ui/src', import.meta.url)),
        },
      },
      optimizeDeps: {
        entries: [
          'src/renderer/src/main.ts',
          '../web/src/main.ts',
          '../web/src/pages/**/*.vue',
        ],
      },
      build: {
        rollupOptions: {
          input: {
            index: resolve(__dirname, 'src/renderer/index.html'),
            settings: resolve(__dirname, 'src/renderer/settings.html'),
          },
        },
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
          },
        },
      },
    },
  }
})
