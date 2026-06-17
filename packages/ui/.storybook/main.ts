import type { StorybookConfig } from '@storybook/vue3-vite'

// Builder-vite auto-loads packages/ui/vite.config.ts, so the @vitejs/plugin-vue,
// @tailwindcss/vite plugin, and the `#` → src alias are all already wired in —
// stories resolve internal imports (e.g. #/components/spinner) the same way the
// library build does.
const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|tsx)'],
  addons: ['@storybook/addon-docs'],
  framework: {
    name: '@storybook/vue3-vite',
    options: {},
  },
}

export default config
