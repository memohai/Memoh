# Desktop App (apps/desktop)

## Overview

`@memohai/desktop` is the Memoh Electron Desktop client for Memoh Cloud or a
hosted Memoh server. It reuses Vue pages, stores, i18n, API setup, and design
tokens from `@memohai/web`, but owns the native Electron shell: windows, tray,
menus, keyboard integration, cache invalidation, preload IPC, and the renderer
bootstrap.

Desktop does not start a local server, package database files, embed Qdrant, or
ship a companion CLI. Use `MEMOH_DESKTOP_BASE_URL` to point the app at the target
server. The default dev target is `http://localhost:18080`.

## Tech Stack

| Category | Technology |
|----------|-----------|
| Shell | Electron 34 |
| Bundler | electron-vite 4 |
| Renderer | Vue 3 + Vite 8 + Tailwind CSS 4 |
| Packager | electron-builder 26 |
| Reused packages | `@memohai/web`, `@memohai/ui`, `@memohai/sdk`, `@memohai/icon`, `@memohai/config` |
| Type checking | TypeScript + `vue-tsc` |

## Directory Structure

```
apps/desktop/
├── electron.vite.config.ts        # main / preload / renderer Vite config
├── electron-builder.yml           # single Desktop package config
├── package.json
├── scripts/
│   ├── build.mjs                  # electron-vite build + electron-builder
│   └── install-icon-tools.mjs      # isolated icon generator dependencies
├── src/
│   ├── main/index.ts              # Electron main process and IPC handlers
│   ├── preload/index.ts           # typed renderer bridge
│   ├── preload/global.d.ts        # renderer API typings
│   └── renderer/
│       ├── src/main.ts            # Vue renderer bootstrap
│       ├── src/chat/App.vue       # persistent shell root
│       └── types/                 # local stubs for reused web/ui exports
├── resources/
│   ├── icon.png
│   └── tray-icon.png
└── build/                         # packager input icons
```

## Reuse from @memohai/web

The renderer imports web modules through public subpath exports in
`apps/web/package.json`, including:

- `@memohai/web/style.css`
- `@memohai/web/i18n`
- `@memohai/web/api-client`
- `@memohai/web/store/settings`
- `@memohai/web/lib/desktop-shell`
- `@memohai/web/layout/main-layout/index.vue`
- `@memohai/web/components/sidebar/index.vue`
- `@memohai/web/components/settings-sidebar/index.vue`
- `@memohai/web/pages/**/*.vue`

Do not import the full web `main.ts`. Desktop has its own bootstrap so it can use
memory-history routing, provide `DesktopShellKey`, wire native menus into the
shared command registry, and keep native cache synchronization out of the web
bundle.

## Type Stubbing

`vue-tsc` follows symlinks. Desktop's renderer typecheck is intentionally scoped
with `tsconfig.web.json` path stubs:

- `src/renderer/types/web-stubs.d.ts` for `@memohai/web/*`
- `src/renderer/types/ui-stubs.d.ts` for `@memohai/ui`

When adding a new reused web or UI import, update the matching stub. Keep the
runtime import specifier and the typecheck stub aligned; do not switch to private
source aliases just to silence type errors.

## Main Process

`src/main/index.ts` owns:

- app identity and single-instance behavior
- BrowserWindow creation and macOS chrome
- tray creation and dock/menu focus behavior
- native menu accelerators
- external URL handling
- cache invalidation broadcast
- renderer `/api` proxy target via `MEMOH_DESKTOP_BASE_URL`

The preload bridge is the only renderer API surface for Electron/main-process
behavior. Keep it small and typed in both `src/preload/index.ts` and
`src/preload/global.d.ts`.

Current Desktop IPC includes:

- `desktop:server-status`
- `desktop:api-base-url`
- `desktop:set-menu-accelerators`
- `desktop:open-external-url`
- `desktop:broadcast-invalidate`
- `window:close-self`

Do not add IPC for local database auth, project-folder picking, server lifecycle,
or CLI installation.

## Renderer

`src/renderer/src/main.ts` creates the Vue app, installs the reused web plugins,
sets the SDK base URL from the main-process status, and registers desktop cache
sync. Authentication belongs to the hosted server flow; the renderer should not
inject local auto-login tokens.

`chat/App.vue` provides `DesktopShellKey` so reused web components can adapt to
Electron chrome without importing Electron.

## Commands

```bash
pnpm --filter @memohai/desktop dev
pnpm --filter @memohai/desktop typecheck
pnpm --filter @memohai/desktop build:dir
pnpm --filter @memohai/desktop build
```

`build:dir` is the CI smoke path for an unpacked app. Packaged output goes to
`apps/desktop/dist/`.

## Packaging Rules

`electron-builder.yml` is the only Desktop packaging config. The product name is
`Memoh`, and packaged resources should only include the Electron application,
icons, and compiled app bundles. Do not add server binaries, CLI binaries,
database files, provider templates, workspace runtimes, Qdrant, or media
runtimes to Desktop packaging.

## Icons

Checked-in icons live in `build/` and `resources/`. Regenerate them only when the
brand mark changes:

```bash
pnpm --filter @memohai/desktop icons
```
