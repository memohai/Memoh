import { chmod, mkdir, rm } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { build } from 'esbuild'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const outdir = resolve(root, 'dist')

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
  packages: 'external',
  outdir,
  outExtension: { '.js': '.mjs' },
})
await chmod(resolve(outdir, 'cli.mjs'), 0o755)
