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
│       │   ├── settings.ts         # Settings renderer bootstrap (parallel chain)
│       │   ├── env.d.ts            # vite/client + *.vue ambient module
│       │   ├── chat/
│       │   │   ├── App.vue         # Chat root (provides DesktopShellKey, Toaster)
│       │   │   └── router.ts       # Chat routes + /settings/* IPC interception + auth guard
│       │   └── settings/
│       │       ├── App.vue         # Settings root (provides DesktopShellKey, MainLayout)
│       │       └── router.ts       # Settings routes (mirrors web's /settings/* paths)
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

When you add a new `@memohai/web/*` import in the desktop renderer, add a
matching `declare module` to `web-stubs.d.ts`. The wildcard
`declare module '@memohai/web/*.vue'` already covers any `.vue` SFC reached
through the wildcard `./*` export.

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
    openSettings(): Promise<void>   // ipc → main:'window:open-settings'
    closeSelf(): Promise<void>      // ipc → main:'window:close-self'
  },
}
```

Plus `window.electron` (from `@electron-toolkit/preload`) for the standard
toolkit utilities. Keep this surface intentionally tiny — every entry is
part of the security boundary.

### Cross-window navigation

Settings actions invoked from the chat window's reused
`@memohai/web/components/sidebar/...` (e.g. the gear footer link, the `+`
button) are intercepted in the chat router's `beforeEach`:

```ts
if (to.path === '/settings' || to.path.startsWith('/settings/')) {
  void window.api?.window?.openSettings()
  return false   // abort in-place navigation
}
```

This is why shared chat-sidebar components must navigate by **path** (e.g.
`router.push('/settings/bots')`), never by `name` — `name: 'bots'` only
exists in the settings router.

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
{ titleBarStyle: 'hiddenInset', trafficLightPosition: { x: 14, y: 12 } }
```

The native titlebar is hidden but the traffic lights remain at a fixed
position. The reusable web sidebars reserve a 36px-tall (h-9) header above
the sidebar content, marked `[-webkit-app-region:drag]` so the window can
be dragged from there. Inside that strip, interactive elements (e.g. the
chat sidebar `+` button) need an explicit
`[-webkit-app-region:no-drag]` wrapper to stay clickable.

The chat content also gets a floating "Title - BotName" header overlaid
with a bottom-fade gradient. It's `pointer-events:none` so scroll/clicks
fall through to messages, and the message list reserves the equivalent
top padding so the first message isn't obscured at rest.

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

Three guards in `beforeEach`:

1. `/settings*` → IPC `openSettings()` → return `false`.
2. `/login` while already logged in → redirect to `/`.
3. Any other route without `localStorage.token` → redirect to `Login`.

Plus an `onError` handler that reloads the window on dynamic-import chunk
load failures (covers the case where the dev server restarts mid-session).

### Settings router (`src/renderer/src/settings/router.ts`)

Mirrors the path layout under `/settings/*` from `@memohai/web/router` so
the reused `SettingsSidebar`'s `route.path.startsWith('/settings/...')`
active-state checks keep working. Route names mirror web exactly:
`bots`, `bot-detail`, `providers`, `web-search`, `memory`, `speech`,
`transcription`, `email`, `browser`, `usage`, `profile`, `platform`,
`supermarket`, `about`. Default redirect: `/` → `/settings/bots`.

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
