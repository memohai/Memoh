import { defineConfig, externalizeDepsPlugin } from 'electron-vite'
import type { PluginOption } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'

const require = createRequire(import.meta.url)

const defaultPort = 8082
const defaultHost = '127.0.0.1'
const desktopRuntimeMode = resolveDesktopRuntimeMode(
  process.env.MEMOH_DESKTOP_RUNTIME_MODE ?? process.env.MEMOH_DESKTOP_FLAVOR,
)
const defaultApiBaseUrl = process.env.VITE_API_URL ?? (
  desktopRuntimeMode === 'remote' ? 'http://localhost:18080' : 'http://localhost:8080'
)

function resolveDesktopRuntimeMode(value: string | undefined): 'local' | 'remote' {
  switch (value?.trim()) {
    case 'remote':
    case 'online':
      return 'remote'
    case 'local':
    case 'offline':
    case undefined:
    case '':
      return 'local'
    default:
      throw new Error(`Unsupported desktop runtime mode: ${value}`)
  }
}

function resolveProxyTarget(command: 'build' | 'serve'): { port: number; host: string; baseUrl: string } {
  const configuredProxyTarget = process.env.MEMOH_WEB_PROXY_TARGET?.trim()
  const configuredPath = process.env.MEMOH_CONFIG_PATH?.trim() || process.env.CONFIG_PATH?.trim()
  const configPath = configuredPath && configuredPath.length > 0 ? configuredPath : '../../config.toml'

  let port = defaultPort
  let host = defaultHost
  let baseUrl = configuredProxyTarget || defaultApiBaseUrl

  const shouldReadConfig = command !== 'build' && (
    desktopRuntimeMode !== 'remote'
    || Boolean(configuredProxyTarget)
    || Boolean(configuredPath)
  )

  if (shouldReadConfig) {
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

  // Dev slot override: when launched via `mise run desktop:dev -- <keyword>`,
  // MEMOH_WEB_PORT pins the Vite dev server port for that slot so multiple
  // worktrees/slots can run side by side without clashing on 8082.
  const slotWebPort = Number.parseInt(process.env.MEMOH_WEB_PORT?.trim() ?? '', 10)
  if (Number.isInteger(slotWebPort) && slotWebPort > 0) {
    port = slotWebPort
  }

  return { port, host, baseUrl }
}

export default defineConfig(async ({ command }) => {
  const { port, host, baseUrl } = resolveProxyTarget(command)
  const bundledElectronToolkit = ['@electron-toolkit/preload', '@electron-toolkit/utils']

  const devtoolsPlugins: PluginOption[] = []
  if (command !== 'build' && process.env.MEMOH_VUE_DEVTOOLS !== '0') {
    try {
      const { default: vueDevTools } = await import('vite-plugin-vue-devtools')
      devtoolsPlugins.push(vueDevTools())
    } catch {
      // DevTools is optional — never block startup.
    }
  }

  return {
    main: {
      plugins: [externalizeDepsPlugin({ exclude: bundledElectronToolkit })],
      define: {
        __MEMOH_DESKTOP_RUNTIME_MODE__: JSON.stringify(desktopRuntimeMode),
      },
    },
    preload: {
      plugins: [externalizeDepsPlugin({ exclude: bundledElectronToolkit })],
      define: {
        __MEMOH_DESKTOP_RUNTIME_MODE__: JSON.stringify(desktopRuntimeMode),
      },
    },
    renderer: {
      root: resolve(__dirname, 'src/renderer'),
      define: {
        __MEMOH_DESKTOP_RUNTIME_MODE__: JSON.stringify(desktopRuntimeMode),
      },
      // Reuse apps/web/public so absolute-path assets (e.g. /logo.svg) resolve
      // when web modules are imported directly from the desktop renderer.
      publicDir: resolve(__dirname, '../web/public'),
      plugins: [...devtoolsPlugins, vue(), tailwindcss()],
      resolve: {
        alias: {
          '@renderer': fileURLToPath(new URL('./src/renderer/src', import.meta.url)),
          // match apps/web/vite.config.ts aliases so imported web modules resolve correctly.
          '@': fileURLToPath(new URL('../web/src', import.meta.url)),
          '#': fileURLToPath(new URL('../../packages/ui/src', import.meta.url)),
        },
      },
      optimizeDeps: {
        // Only pre-bundle from the renderer entry — scanning all web pages
        // forces esbuild to crawl Monaco/xterm/ECharts/Mermaid/etc. on every
        // new import, which during AI-assisted editing triggers repeated
        // full dev-server restarts and page reloads. Vite's scanner follows
        // dynamic imports in the router, so page-level deps are still
        // discovered without listing every page here.
        entries: [
          'src/renderer/src/main.ts',
        ],
        // Monaco is the one heavy dependency the scanner can't front-load from
        // the entry: the editor host (stream-monaco) pulls Monaco's many ESM
        // sub-modules lazily, so the first mount of a bot-settings tab that
        // hosts an editor (bot-network / bot-skills / bot-mcp) makes the dev
        // server discover them mid-session, re-run esbuild, and force a full
        // page reload (in-flight dynamic imports 504, caught by the router's
        // onError → window.location.reload). Pre-bundling these once at
        // startup trades a slightly slower cold start for no mid-session
        // reload. Unlike adding them to `entries`, `include` does not re-crawl
        // on every edit, so the concern above still holds.
        include: [
          'monaco-editor',
          'stream-monaco',
          'shiki',
        ],
      },
      build: {
        rollupOptions: {
          input: {
            index: resolve(__dirname, 'src/renderer/index.html'),
          },
        },
      },
      server: {
        port,
        host,
        hmr: {
          overlay: false,
        },
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
