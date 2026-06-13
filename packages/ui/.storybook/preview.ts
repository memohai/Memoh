import type { Preview } from '@storybook/vue3-vite'
import './tailwind.css'

// Theming is driven exactly like the app: dark mode is a `.dark` class on
// <html> (style.css `@custom-variant dark (&:is(.dark *))`) and the palette is a
// `data-color-scheme` attribute on <html> ([data-color-scheme="ocean"] …). The
// toolbar globals below flip those so every story matches the component wall.
const SCHEMES = ['memoh', 'ocean', 'forest', 'rose', 'amber'] as const

function applyTheme(theme: string, scheme: string) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  root.classList.toggle('dark', theme === 'dark')
  root.dataset.colorScheme = scheme
}

const preview: Preview = {
  parameters: {
    controls: {
      matchers: { color: /(background|color)$/i, date: /Date$/i },
    },
    // Background is supplied by the design tokens (bg-background wrapper below),
    // so disable Storybook's own background addon to avoid a clashing canvas.
    backgrounds: { disable: true },
    layout: 'fullscreen',
  },
  initialGlobals: {
    theme: 'light',
    scheme: 'memoh',
  },
  globalTypes: {
    theme: {
      description: 'Color mode',
      toolbar: {
        title: 'Theme',
        icon: 'circlehollow',
        items: [
          { value: 'light', title: 'Light', icon: 'sun' },
          { value: 'dark', title: 'Dark', icon: 'moon' },
        ],
        dynamicTitle: true,
      },
    },
    scheme: {
      description: 'Color scheme',
      toolbar: {
        title: 'Scheme',
        icon: 'paintbrush',
        items: SCHEMES.map(s => ({ value: s, title: s[0].toUpperCase() + s.slice(1) })),
        dynamicTitle: true,
      },
    },
  },
  decorators: [
    (story, context) => {
      applyTheme(context.globals.theme ?? 'light', context.globals.scheme ?? 'memoh')
      return {
        components: { story },
        template:
          '<div class="bg-background text-foreground" style="min-height:100vh;padding:2rem;"><story /></div>',
      }
    },
  ],
}

export default preview
