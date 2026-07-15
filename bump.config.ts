import { defineConfig } from 'bumpp'

export default defineConfig({
  files: [
    'package.json',
    'packages/sdk/package.json',
    'packages/runtime/package.json',
    'packages/icons/package.json',
    'packages/config/package.json',
    'apps/web/package.json',
    'apps/desktop/package.json',
  ],
  commit: 'release: v%s',
  tag: 'v%s',
  push: true,
  all: true,
})
