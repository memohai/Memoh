# @memohai/desktop

Memoh desktop application built with [electron-vite](https://electron-vite.github.io/).

The renderer owns its own `src/renderer/src/main.ts` — it imports the reusable
building blocks from `@memohai/web` (`App.vue`, `router`, `i18n`, `api-client`,
`style.css`) but assembles the Vue app locally. Desktop-only customization (extra
Pinia plugins, Electron-specific stores / providers, alternate routers, etc.)
belongs in this `main.ts`, not in `@memohai/web`.

### How the reuse is wired

- `@memohai/web/package.json` exposes `App.vue`, `router`, `i18n`, `api-client`,
  and `style.css` through its `exports` field.
- Vite (via `electron.vite.config.ts`) resolves those subpaths to the real
  files in `apps/web/src/` at bundle time.
- `vue-tsc` is pointed at local type stubs in `src/renderer/types/web-stubs.d.ts`
  via tsconfig `paths`, so desktop's typecheck does **not** descend into
  `apps/web/src/` or `packages/ui/src/` (those have their own CI).

## Development

```bash
# from repo root
pnpm --filter @memohai/desktop dev
# or via mise
mise run desktop:dev
```

`MEMOH_WEB_PROXY_TARGET` overrides the backend that the renderer's `/api` proxy points
at (defaults to whatever `config.toml` / `conf/app.docker.toml` declares).

## Build

```bash
pnpm --filter @memohai/desktop build           # full platform installer
pnpm --filter @memohai/desktop build:dir       # unpacked app dir (CI smoke test)
```

Output goes to `apps/desktop/dist/`.

## Icons

All app icons are generated from `apps/web/public/logo.svg` (the brand mark) by
`scripts/build-icons.mjs`. Re-run after the logo changes:

```bash
pnpm --filter @memohai/desktop icons
```

Outputs:

| File | Purpose |
|---|---|
| `build/icon.icns` | macOS bundle icon (16…1024 + @2x) — packaged into `.app/Contents/Resources/` |
| `build/icon.ico` | Windows installer / `.exe` icon (16/24/32/48/64/128/256) |
| `build/icon.png` | Linux `.deb`/`.rpm`/`.AppImage` icon (1024×1024) |
| `resources/icon.png` | Runtime `BrowserWindow.icon` + macOS dev `app.dock.setIcon` (512×512); bundled via `asarUnpack` |

`build/icon.icns` requires macOS (`iconutil`); the script skips it on other
platforms.
