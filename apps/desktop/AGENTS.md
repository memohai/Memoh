# Desktop App (apps/desktop)

## Overview

`@memohai/desktop` is the Memoh Electron desktop application. It does **not**
re-implement the UI — it reuses Vue components, stores, router pieces, and
styles from `@memohai/web` and assembles its own multi-window Electron shell
on top.

The app boots two independent `BrowserWindow`s:

| Window | Renderer entry | HTML | Router routes |
|--------|----------------|------|---------------|
| **Chat** (primary) | `src/renderer/src/main.ts` | `index.html` | `/`, `/chat/:botId?/:sessionId?`, `/login`, `/oauth/mcp/callback` |
| **Settings** (satellite) | `src/renderer/src/settings.ts` | `settings.html` | `/settings/*` (bots, providers, memory, …) |

The two windows are isolated renderer processes — separate Pinia, separate
Vue Router, separate Vite chunks — but share user state via the
`pinia-plugin-persistedstate` localStorage stores (chat-selection, user
token, settings, etc.). Settings is a satellite of chat: chat hosts login,
settings closes itself on 401.

## Tech Stack

| Category | Technology |
|----------|-----------|
| Shell | [Electron](https://www.electronjs.org/) (^34) |
| Bundler | [electron-vite](https://electron-vite.github.io/) (^4) — orchestrates main + preload + renderer Vite builds |
| Renderer build | Vite 8 + `@vitejs/plugin-vue` + `@tailwindcss/vite` |
| Packager | electron-builder (^26) → `.dmg` / `.zip` (mac), `.AppImage` / `.deb` / `.rpm` (linux), NSIS (win) |
| Vue ecosystem | Vue 3, Vue Router 4 (`createMemoryHistory`), Pinia 3, `@pinia/colada`, vue-i18n, vue-sonner |
| Reused workspace packages | `@memohai/web`, `@memohai/ui`, `@memohai/sdk`, `@memohai/icon`, `@memohai/config` |
| Preload helpers | `@electron-toolkit/preload`, `@electron-toolkit/utils` |
| Icon pipeline | `sharp` (PNG / resize) + `png-to-ico` (Windows) + `iconutil` (macOS, system tool) |
| Type checking | TypeScript ~5.9 strict + `vue-tsc` for the renderer |

## Directory Structure

```
apps/desktop/
├── electron.vite.config.ts        # Single config file: main / preload / renderer Vite configs
├── electron-builder.yml            # Packager config (appId, targets, icons, asarUnpack)
├── package.json
├── tsconfig.json                   # Solution file (references node + web)
├── tsconfig.node.json              # Main + preload typecheck (NodeNext-bundler)
├── tsconfig.web.json               # Renderer typecheck (DOM, with @memohai/* path stubs)
├── README.md                       # User-facing dev/build guide
├── AGENTS.md                       # This file
│
├── src/
│   ├── main/
│   │   └── index.ts                # Main process: BrowserWindow factories, IPC, app lifecycle
│   ├── preload/
│   │   ├── index.ts                # Preload bridge — exposes `window.electron` + `window.api`
│   │   └── global.d.ts             # Window augmentation for renderer typecheck
│   └── renderer/
│       ├── index.html              # Chat window root
│       ├── settings.html           # Settings window root
│       ├── src/
│       │   ├── main.ts             # Chat renderer bootstrap (createApp, plugins, router, i18n)
│       │   ├── settings.ts         # Settings renderer bootstrap (parallel chain) + onSettingsNavigate listener
│       │   ├── env.d.ts            # vite/client + *.vue ambient module
│       │   ├── chat/
│       │   │   ├── App.vue         # Chat root (provides DesktopShellKey, Toaster)
│       │   │   └── router.ts       # Chat routes + settings name stubs + /settings/* IPC interception + auth guard
│       │   ├── settings/
│       │   │   ├── App.vue         # Settings root (provides DesktopShellKey, MainLayout)
│       │   │   └── router.ts       # Settings routes (built from shared spec; mirrors web's /settings/* paths)
│       │   └── shared/
│       │       └── settings-routes.ts  # Single source of truth for settings name+path+loader, consumed by both routers
│       └── types/
│           ├── web-stubs.d.ts      # Path-mapped stub for @memohai/web/* — see "Type Stubbing"
│           └── ui-stubs.d.ts       # Path-mapped stub for @memohai/ui (Toaster, SidebarInset)
│
├── scripts/
│   └── build-icons.mjs             # Regenerates icns / ico / png from apps/web/public/logo.svg
│
├── resources/
│   └── icon.png                    # 512×512 — runtime BrowserWindow.icon + macOS dock.setIcon
│
├── build/                          # Packager input assets (gitignored at root, re-included here)
│   ├── icon.icns                   # macOS bundle icon
│   ├── icon.ico                    # Windows installer icon
│   └── icon.png                    # 1024×1024 source for Linux + ico master
│
├── out/                            # electron-vite output (main, preload, renderer bundles) — gitignored
└── dist/                           # electron-builder output (installers / unpacked apps) — gitignored
```

## Reuse from @memohai/web

The renderer is **not** a thin shell that imports `@memohai/web/main` —
desktop owns its own bootstrap. It reuses building blocks via subpath
exports declared in `apps/web/package.json`:

| Subpath | Purpose |
|---------|---------|
| `@memohai/web/style.css` | Tailwind + design tokens |
| `@memohai/web/i18n` | vue-i18n instance |
| `@memohai/web/api-client` | `setupApiClient({ onUnauthorized })` SDK setup |
| `@memohai/web/store/settings` | Theme + locale store (registered for side effects) |
| `@memohai/web/lib/desktop-shell` | `DesktopShellKey` injection key |
| `@memohai/web/layout/main-layout/index.vue` | Outer sidebar layout |
| `@memohai/web/components/sidebar/index.vue` | Bot list sidebar (chat shell) |
| `@memohai/web/components/settings-sidebar/index.vue` | Settings nav (settings shell) |
| `@memohai/web/pages/**/*.vue` | Routed pages (home, bots, providers, …) |

Vite resolves these at bundle time via the package's `exports` field.
**TypeScript does not** — see [Type Stubbing](#type-stubbing) below.

### Why managed bootstrap, not full reuse

Desktop needs to do things `@memohai/web/main.ts` cannot: provide an
`InjectionKey` so reusable components know they're in the Electron shell,
swap the 401 handler (settings closes itself; chat redirects to `/login`),
own its own router with memory history and the `/settings/*` IPC hijack,
and be free to register desktop-only Pinia plugins without polluting the
web bundle.

`@memohai/web/api-client`'s `setupApiClient(options)` accepts an
`onUnauthorized` callback for exactly this reason — never hard-code redirect
behaviour into `@memohai/web`.

## Type Stubbing

`vue-tsc` follows symlinks. Without intervention, typechecking the desktop
renderer would descend into `apps/web/src/` and `packages/ui/src/`, surfacing
strict-template warnings in code that isn't desktop's responsibility (each
of those packages has its own CI scope).

Solution: `tsconfig.web.json` sets `paths` to redirect `@memohai/web/*` to
`src/renderer/types/web-stubs.d.ts` and `@memohai/ui` to
`src/renderer/types/ui-stubs.d.ts`. The stubs declare just enough surface
(component shape, store shape, router/i18n types) for desktop's own code to
typecheck.

**Vite ignores `tsconfig` `paths`** — at runtime it follows the package's
real `exports` field. So bundle behaviour is unchanged; only `vue-tsc`
follows the stubs.

The typecheck import graph must still mirror the runtime import graph. When
desktop reuses workspace package exports, source files must import through
the same public module specifiers that Vite bundles at runtime, and the
renderer stubs must model that same surface. Do not paper over `vue-tsc`
errors by swapping imports to aliases or private source paths unless both
Vite and `tsconfig.web.json` intentionally resolve that specifier to the
same module. A resolver mismatch can make one pipeline pass while the other
fails, or make typecheck validate a different module than the one packaged
at runtime.

When you add a new `@memohai/web/*` import in the desktop renderer, add a
matching `declare module` to `web-stubs.d.ts`. The wildcard
`declare module '@memohai/web/*.vue'` already covers any `.vue` SFC reached
through the wildcard `./*` export.

When you import a new component directly from `@memohai/ui`, add that
component to `ui-stubs.d.ts` as well. The package may export it correctly,
but desktop's renderer typecheck only sees the stubbed surface.

## Multi-Window Lifecycle

### Main process (`src/main/index.ts`)

- `chatWindow: BrowserWindow | null` is the persistent primary window. `app.on('activate')` recreates it on macOS dock click.
- `settingsWindow: BrowserWindow | null` is created lazily by IPC and is parented to the chat window (not modal).
- Both windows share `webPreferences` (sandbox: false, contextIsolation: true, nodeIntegration: false) and the same preload script. The renderer is therefore strictly browser-grade — anything that needs node/Electron APIs must go through IPC.

### IPC surface (`src/preload/index.ts`)

The preload bridge exposes a small, fixed surface to renderers via
`contextBridge.exposeInMainWorld('api', api)`:

```ts
window.api = {
  window: {
    openSettings(target?: string): Promise<void>          // ipc → main:'window:open-settings'
    closeSelf(): Promise<void>                            // ipc → main:'window:close-self'
    onSettingsNavigate(cb: (target: string) => void): void // settings-window subscription for IPC 'settings:navigate'
  },
}
```

Plus `window.electron` (from `@electron-toolkit/preload`) for the standard
toolkit utilities. Keep this surface intentionally tiny — every entry is
part of the security boundary.

### Cross-window navigation

Settings actions invoked from the chat window's reused @memohai/web
components — the gear footer link, the sidebar `+` button, the bot-item
"Details" menu, the chat sidebar MCP/Schedule panels' `+` icons, the
schedule/heartbeat trigger blocks, etc. — all eventually call either
`router.push('/settings/...')` or `router.push({ name: 'bot-detail', ... })`.

Both shapes are handled in the chat router. Name-based navigation works
because the chat router registers no-op stub routes for every settings
`name` from `shared/settings-routes.ts`. That lets vue-router resolve
`{ name: 'bot-detail', params, query }` into a concrete `/settings/...`
fullPath without warnings before the guard fires:

```ts
if (to.path === '/settings' || to.path.startsWith('/settings/')) {
  void window.api?.window?.openSettings(to.fullPath)
  return false   // abort in-place navigation
}
```

The main process handler then:

1. Creates the settings `BrowserWindow` if it doesn't exist, otherwise
   restores/focuses the existing one (`focusWindow`).
2. Sends `settings:navigate` with the requested path. If the renderer
   isn't ready yet (cold start or in-flight reload), the target is
   buffered in a Map keyed by webContents id and drained from the
   per-window `did-finish-load` listener attached at creation time.

The settings renderer subscribes via `onSettingsNavigate` before mount
(in `src/renderer/src/settings.ts`) and pushes the path through its own
router. A guard skips the push when the requested path equals the
current `fullPath` so no spurious navigation is generated.

When you add a new settings page in `@memohai/web`, also add an entry to
`src/renderer/src/shared/settings-routes.ts`. Both routers consume it,
so cross-window jumps stay correct without manual sync.

### Settings 401 handling

The chat renderer's `setupApiClient` calls `router.replace({ name: 'Login' })`
on 401. The settings renderer instead calls `window.api.window.closeSelf()`.
The chat window's own auth guard then takes over and routes to `/login`.

## Desktop Shell Awareness — `DesktopShellKey`

Reusable web layouts/components need to know when they're hosted inside the
Electron shell — to reserve space for the macOS traffic lights, disable
small-screen auto-collapse, etc. — without depending on Electron at runtime.

Pattern:

1. `apps/web/src/lib/desktop-shell.ts` defines and exports
   `DesktopShellKey: InjectionKey<boolean>`.
2. Web (`apps/web/src/main.ts`) does **not** provide it → `inject(...,
   false)` falls back to `false`.
3. Desktop renderer roots (`chat/App.vue`, `settings/App.vue`) call
   `provide(DesktopShellKey, true)`.
4. Consumers (`components/sidebar`, `components/settings-sidebar`,
   `layout/main-layout`, `pages/home/components/chat-area`) inject and gate
   their desktop affordances on the result.

Adding a new desktop affordance to a web component is a four-step
checklist: `inject(DesktopShellKey, false)` → conditional template branch →
add a `declare module '@memohai/web/...'` stub if it's a new export →
update both Web/CI and desktop typecheck.

## macOS Chrome

On `process.platform === 'darwin'` only, both `BrowserWindow`s opt in to:

```ts
{ titleBarStyle: 'hidden', trafficLightPosition: { x: 14, y: 12 } }
```

The native titlebar is hidden but the traffic lights remain at a fixed
position. To let the user grab the **entire** top of each window —
not just the sidebar corner — the two windows take different paths,
chosen so the chrome looks intentional rather than introducing an
empty grey gap above the page content:

- **Both sidebars** (`components/sidebar/index.vue`,
  `components/settings-sidebar/index.vue`) render their existing
  36px-tall (`h-9`) `position:fixed` drag header above the sidebar
  body when `topInset` is true, marked `[-webkit-app-region:drag]`
  with `pl-[78px]` to clear the traffic lights. Interactive elements
  inside (e.g. the chat sidebar `+` button) opt out with explicit
  `[-webkit-app-region:no-drag]` wrappers.

- **Chat right side** reuses the existing 48px tab bars instead of
  reserving a separate strip. The two chat-window tab-bar rows —
  `pages/home/components/chat-sidebar.vue` (activity tabs) and
  `pages/home/components/workspace-tab-bar.vue` (workspace tabs +
  toolkit) — both carry `[-webkit-app-region:drag]` on the row root,
  while every interactive child (tab button, close `×`, terminal
  button, dropdown trigger) carries `[-webkit-app-region:no-drag]`.
  Empty space within the bars is therefore draggable; clicks on
  buttons still work. No extra height is added to the chat window.

- **Settings right side** can't reuse the chat trick — settings
  pages vary too much (master/detail layouts, full-page tables,
  forms) and a 36px chrome strip above the page looks out of place.
  Instead, `apps/desktop/src/renderer/src/settings/App.vue` paints
  an invisible 16px-tall (`h-4`) `position:fixed` drag strip across
  the very top edge of the window at `z-10`, sized to match the
  routed sections' `p-4` top padding so it lives in the page's
  existing dead space. The SettingsSidebar's own drag header sits
  at `z-20` and covers the strip on the left half; on the right
  half the strip is the topmost layer and becomes a thin
  transparent grab zone. On `MasterDetailSidebarLayout` pages the
  inner sidebar's `SidebarMenu` only has `p-2` (8px), so the
  strip's lower 8px clips the very top of the first sidebar item —
  acceptable because those rows are `py-5` and the bulk of each
  hit area remains clickable. No visible chrome is added.

## electron-vite Configuration

`electron.vite.config.ts` defines three Vite configs in one file:

| Section | Notable settings |
|---------|------------------|
| `main` | `externalizeDepsPlugin()` (don't bundle node_modules into the main bundle) |
| `preload` | Same `externalizeDepsPlugin()` |
| `renderer` | `root: src/renderer`, `publicDir: ../web/public` (so `/logo.svg` resolves), `resolve.alias` mirrors `apps/web/vite.config.ts` (`@` → `apps/web/src`, `#` → `packages/ui/src`), two HTML entries (`index` + `settings`), `optimizeDeps.entries` includes web sources, dev-only `/api` proxy reading port + base URL from `@memohai/config` |

### Important runtime gotcha — preload extension

`apps/desktop/package.json` has `"type": "module"`, so electron-vite emits
the preload as `out/preload/index.mjs` (not `.js`). The main process **must**
load `../preload/index.mjs`. Loading the wrong extension does not throw —
Electron silently fails to attach the preload, and `window.api` ends up
`undefined` in the renderer. This is captured in:

```ts
const PRELOAD_FILE = '../preload/index.mjs'
```

If you ever change the package's module type, also update this constant.

### Dev-server proxy

In dev (`pnpm --filter @memohai/desktop dev`), the renderer Vite server
listens on the port configured in `config.toml` (default 8082) and proxies
`/api/*` to the backend's `getBaseUrl(config)`. `MEMOH_WEB_PROXY_TARGET` env
var overrides the proxy target.

## Routing

Both windows use `createMemoryHistory()` — the `file://` runtime makes
`createWebHistory()` impractical.

### Chat router (`src/renderer/src/chat/router.ts`)

| Path | Name | Component |
|------|------|-----------|
| `/` | `home` | `@memohai/web/pages/home/index.vue` |
| `/chat/:botId?/:sessionId?` | `chat` | `@memohai/web/pages/home/index.vue` |
| `/login` | `Login` | `@memohai/web/pages/login/index.vue` |
| `/oauth/mcp/callback` | `oauth-mcp-callback` | `@memohai/web/pages/oauth/mcp-callback.vue` |
| `/settings/...` (every entry from `shared/settings-routes.ts`) | mirror of settings name | no-op stub component |

The settings rows above are placeholders — the guard intercepts them
before they ever render. They exist so that `router.push({ name: 'bots' })`
and friends from reused @memohai/web components resolve to a concrete
`/settings/...` fullPath that gets forwarded to the settings window.

Three guards in `beforeEach`:

1. `/settings*` → IPC `openSettings(to.fullPath)` → return `false`.
2. `/login` while already logged in → redirect to `/`.
3. Any other route without `localStorage.token` → redirect to `Login`.

Plus an `onError` handler that reloads the window on dynamic-import chunk
load failures (covers the case where the dev server restarts mid-session).

### Settings router (`src/renderer/src/settings/router.ts`)

Built from `shared/settings-routes.ts` (the same spec the chat router
uses for its stubs). Path layout mirrors `@memohai/web/router`'s
`/settings/*` children so the reused `SettingsSidebar`'s
`route.path.startsWith('/settings/...')` active-state checks keep
working. Route names mirror web exactly: `bots`, `bot-detail`,
`providers`, `web-search`, `memory`, `speech`, `transcription`, `email`,
`browser`, `usage`, `profile`, `platform`, `supermarket`, `about`.
Default redirect: `/` → `/settings/bots`.

The settings window has **no auth guard** — by design. If the chat window
isn't authenticated yet, it owns login. Any 401 returned to a settings
request closes the settings window (see "Settings 401 handling" above).

## Icon Pipeline

`scripts/build-icons.mjs` rasterizes `apps/web/public/logo.svg` into every
icon asset the packager needs. Run via:

```bash
pnpm --filter @memohai/desktop icons
```

Outputs (all derived from a single 1024×1024 master with 14% safe-area
padding to clear macOS Big Sur+ squircle masks):

| File | Used by |
|------|---------|
| `build/icon.png` (1024) | electron-builder Linux (`.AppImage` / `.deb` / `.rpm`) + ico master |
| `build/icon.icns` | electron-builder macOS bundle |
| `build/icon.ico` | electron-builder Windows installer |
| `resources/icon.png` (512) | Runtime `BrowserWindow.icon` + macOS `app.dock.setIcon` |

`build/icon.icns` requires `iconutil` (macOS only); the script logs and
skips it on other platforms. `resources/` is `asarUnpack`ed by
electron-builder so the runtime icon is dereferenceable from disk.

## Build & Distribution

| Command | Output | Notes |
|---------|--------|-------|
| `pnpm --filter @memohai/desktop dev` | dev server + main process watch | Renderer hot-reload; main needs window restart on changes |
| `pnpm --filter @memohai/desktop build` | `dist/` installers (current platform) | Runs electron-vite build, then electron-builder |
| `pnpm --filter @memohai/desktop build:dir` | `dist/<platform>-unpacked/` | Skip installer; smoke-test packaged app |
| `pnpm --filter @memohai/desktop build:mac` | DMG + ZIP (arm64 + x64) | Requires darwin |
| `pnpm --filter @memohai/desktop build:linux` | AppImage + deb + rpm | x64 |
| `pnpm --filter @memohai/desktop build:win` | NSIS installer | x64 |
| `pnpm --filter @memohai/desktop typecheck` | (no output) | Runs `typecheck:node` then `typecheck:web` |

Ports / hosts during dev come from the same `config.toml` the rest of the
stack reads (via `@memohai/config`). The repo-level `mise run desktop:dev`
task is the recommended entrypoint when contributing.

## Bundled CLI

The Memoh CLI (Go, source at `cmd/memoh/`) ships inside the desktop
app bundle alongside the server. It is the **desktop edition** of the
CLI — there are no docker-compose / login subcommands; everything
operates on the local server the desktop already manages.

### Layout in the packaged app

```
Memoh.app/Contents/Resources/
├── server/memoh-server     # backend binary spawned by main process or `memoh start`
├── cli/memoh               # CLI binary; PATH symlink resolves here
├── runtime/                # bridge binary + templates
└── …
```

### Build pipeline

`scripts/prepare-local-server.mjs` runs three `go build` invocations:
`./cmd/agent` → `resources/server/memoh-server`, `./cmd/memoh` →
`resources/cli/memoh`, and `./cmd/bridge` → `resources/runtime/bridge`
(linux/$arch). `electron-builder.yml`'s `extraResources` block then
maps each directory into `Contents/Resources/` of the app bundle.

### Shared filesystem contract

CLI and main process cooperate purely through files under
`app.getPath('userData')` (= `~/Library/Application Support/Memoh` on
macOS — see `productName` pinning below):

| File | Owner | Used by |
|------|-------|---------|
| `config.toml` | desktop main (renders from `resources/config/app.local.toml` on first launch) | both — server's `CONFIG_PATH`, CLI's `[admin]` source |
| `local-server.pid.json` | whoever spawned the server (desktop or CLI) | both — graceful stop / liveness probe |
| `local-server.log` | server itself (stdout/stderr append) | both — `memoh logs`, desktop log dump |
| `qdrant/qdrant.pid.json` | desktop main (CLI never spawns qdrant on its own) | desktop main only |
| `cli-token.json` | CLI (after self-login against `[admin]`) | CLI only |
| `cli-prefs.json` | desktop main (records `dontAskAgain` for the install prompt) | desktop main only |

The pid JSON shape is deliberately identical between
`apps/desktop/src/main/daemon.ts` and
`internal/tui/local/service.go`, so `memoh stop` can kill a server
that desktop spawned and vice versa.

### productName pinning

`apps/desktop/package.json` sets `"productName": "Memoh"`, and
`src/main/index.ts` calls `app.setName('Memoh')` *and* runs a
one-shot `migrateLegacyUserDataDirectory()` before any path is
resolved (older builds defaulted to `@memohai/desktop`). The Go CLI
hard-codes the same product name in
`internal/tui/local/paths.go::productName`. **If you ever rename the
app, both sides must change in lockstep** — otherwise CLI and
desktop will read/write different userData directories and silently
diverge.

### PATH integration (`src/main/cli-integration.ts`)

`detectCliState()` runs `which memoh` (`where` on Windows) plus
`fs.realpathSync` to bucket the install into one of:
`installed-current` (symlink resolves to this app's `Resources/cli`),
`installed-stale` (resolves to a different `Memoh.app`), `installed-foreign`
(resolves to some other `memoh` — homebrew, manual go build, etc.),
or `not-installed`.

`installCli()` is platform-specific:

- **macOS**: `osascript … with administrator privileges` to create
  `/usr/local/bin/memoh` symlink. Triggers the system password prompt.
- **Linux**: writes `~/.local/bin/memoh` symlink (no admin); if that
  dir is not on PATH the renderer / first-launch prompt surfaces a
  hint via `linuxPathHint()`.
- **Windows**: `setx Path …` updates `HKCU\Environment\Path` (no
  admin required) — Windows broadcasts `WM_SETTINGCHANGE`
  automatically so newly opened shells pick it up. Existing terminals
  must be restarted.

`runCliInstallCheck()` runs once after `app.whenReady()`:
`installed-current` → no-op; `installed-stale` → silent reinstall;
otherwise (and not `dontAskAgain`) → three-button dialog `Install /
Skip / Don't ask again`. The Tools / app menu item
`Install Command Line Tool…` (label flips to `Reinstall…` once
installed) always provides a manual path; an `Uninstall Command Line
Tool` sibling is enabled when the symlink is current.

### IPC surface

```
window.api.desktop.getCliStatus(): Promise<CliStatusPayload>
window.api.desktop.installCli(): Promise<CliStatusPayload>
window.api.desktop.uninstallCli(): Promise<CliStatusPayload>
```

Reserved for a future settings-page "CLI integration" card; not yet
consumed by the renderer.

### CLI command surface

| Command | Channel | Notes |
|---------|---------|-------|
| `memoh` (default) / `memoh tui` | HTTP + WebSocket | Bubble Tea TUI; auto self-login |
| `memoh chat --bot --message [--session]` | HTTP + WebSocket | One-shot streaming chat |
| `memoh bots create / delete` | HTTP REST | |
| `memoh start [--wait]` | spawn binary | Resolves bundled `memoh-server`, fails with a hint if `userData/config.toml` is missing |
| `memoh stop` | SIGTERM via pid file | |
| `memoh restart` | stop + start | |
| `memoh status` | HTTP `GET /ping` + pid liveness | |
| `memoh logs [-f] [-n N]` | tail `local-server.log` | |
| `memoh version` | local | |

`--server URL` overrides the default `http://127.0.0.1:18731` and
disables auto self-login (advanced; the user is expected to manage
auth out-of-band).

## Native Dependencies

`electron`, `electron-winstaller`, `esbuild`, and `sharp` all require
`postinstall` scripts to be allowed (they install or compile native
binaries). They are listed in the **root** `pnpm-workspace.yaml` under
`onlyBuiltDependencies` — if `pnpm install` ever fails with `Error:
Electron uninstall` or `sharp` missing its prebuilt binary, that's the
list to check.

## Development Rules

- **Do not edit `@memohai/web` to make desktop work.** If web doesn't
  expose what you need, add a new subpath export to
  `apps/web/package.json` and a matching stub in
  `src/renderer/types/web-stubs.d.ts`. Web should remain shippable as a
  pure browser app.
- **Path-based navigation in shared components.** Any reusable
  chat-sidebar (or other web component embedded in the chat window) that
  navigates to settings must use `router.push('/settings/...')`, never
  `router.push({ name: '...' })` — the chat router only knows the chat
  routes.
- **Provide `DesktopShellKey` at the renderer App root, not deeper.** Web
  must keep injecting `false` (the default fallback) — never provide it
  from any web component.
- **All renderer code is browser-grade.** Need a node/Electron API in the
  renderer? Add an IPC handler in `src/main/index.ts`, a passthrough in
  `src/preload/index.ts`, then update `MemohApi` (the type derived from
  `api`) and `src/preload/global.d.ts`. Don't reach for `nodeIntegration:
  true`.
- **Persist user state through the existing web Pinia stores** (chat-
  selection, user, settings) — they're already configured with
  `pinia-plugin-persistedstate` and shared across both windows via
  localStorage. Don't add desktop-only persistence layers without a
  compelling reason.
- **Update both `tsconfig.web.json` paths and `web-stubs.d.ts`** when adding
  a new `@memohai/web/foo` import. Forgetting the stub yields
  `TS2307: Cannot find module` even though the bundle works.
- **Keep typecheck resolution and runtime resolution isomorphic.** Desktop
  renderer code must import reused workspace modules through the same public
  specifiers that Vite bundles at runtime, then model that surface in the
  stubs. Do not mix in alternate aliases or private source paths unless both
  Vite and `tsconfig.web.json` intentionally resolve them to the same target.
- **Update `ui-stubs.d.ts` for direct `@memohai/ui` imports.** A component
  exported by the real UI package still needs to exist in the desktop stub
  if desktop imports it directly.
- **Run `pnpm --filter @memohai/desktop typecheck` after every renderer
  change.** It's fast (only types desktop's own code thanks to the stubs)
  and catches the common drift cases (missing stub, wrong store/component
  shape, untyped IPC arg).
- **Update this file** when you add a new window, IPC handler, subpath
  reuse, build target, or platform-specific affordance — the desktop
  shell is small enough that out-of-date docs become obviously wrong
  fast.

## Cross-References

- Repo root: `/AGENTS.md` (overall architecture, server-side packages, db conventions).
- Web: `apps/web/AGENTS.md` (component / store / page conventions; consumed wholesale here).
- Design system: `packages/ui/DESIGN.md` (tokens, elevation, spacing — applies to anything rendered in either desktop window).
