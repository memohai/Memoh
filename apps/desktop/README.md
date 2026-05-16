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

Packaged macOS and Windows x64 builds include the display GStreamer runtime used
by the local server. Linux builds continue to use system GStreamer when
available.

## Icons

All app icons are checked in under `build/` and `resources/`. They are generated
from `apps/web/public/logo.svg` (the brand mark) by `icon-tools/build-icons.mjs`,
but the generator's image-processing dependencies are not part of the default
workspace install because normal development and packaging only consume the
checked-in assets. Re-run the generator after the logo changes; this installs
the generator dependencies in `apps/desktop/icon-tools/`:

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
| `resources/tray-icon.png` | Runtime tray/menu bar icon |

`build/icon.icns` requires macOS (`iconutil`); the script skips it on other
platforms.
