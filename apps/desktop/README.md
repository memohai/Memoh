# @memohai/desktop

Memoh desktop application built with [electron-vite](https://electron-vite.github.io/).

The renderer is intentionally a thin shell — its `main.ts` imports `@memohai/web`'s own
bootstrap (router / Pinia / api-client / `App.vue`) so the desktop app runs the same
experience as the web app out of the box. Future desktop-only customization should
happen in this package (by replacing or composing parts of the `@memohai/web` surface),
not by forking the web app.

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
