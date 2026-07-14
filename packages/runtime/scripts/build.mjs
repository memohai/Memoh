import { chmod, mkdir, rm } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { build } from 'esbuild'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const outdir = resolve(root, 'dist')
const commonJSBridge = [
  "import { createRequire as __memohCreateRequire } from 'node:module'",
  'const require = __memohCreateRequire(import.meta.url)',
].join('\n')

await rm(outdir, { recursive: true, force: true })
await mkdir(outdir, { recursive: true })
await build({
  entryPoints: {
    index: resolve(root, 'src/index.ts'),
    cli: resolve(root, 'src/cli.ts'),
  },
  bundle: true,
  platform: 'node',
  format: 'esm',
  target: 'node20',
  packages: 'bundle',
  banner: { js: commonJSBridge },
  outdir,
  outExtension: { '.js': '.mjs' },
})
await chmod(resolve(outdir, 'cli.mjs'), 0o755)

// Bundled CommonJS dependencies such as ws and grpc-js must remain loadable
// from the ESM artifact used by Electron's main process.
await import(pathToFileURL(resolve(outdir, 'index.mjs')).href)
