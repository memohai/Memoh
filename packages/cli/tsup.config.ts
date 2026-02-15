import { defineConfig } from 'tsup'

export default defineConfig({
  entry: { cli: 'src/cli/index.ts' },
  format: ['esm'],
  target: 'node20',
  platform: 'node',
  bundle: true,
  splitting: false,
  clean: true,
  // @memoh/sdk exports raw .ts, must be bundled
  noExternal: [/^@memoh\/sdk/],
  banner: {
    js: '#!/usr/bin/env node',
  },
})
