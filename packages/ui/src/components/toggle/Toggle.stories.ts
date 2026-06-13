import type { Meta, StoryObj } from '@storybook/vue3-vite'
import { Bold, Italic, Strikethrough, Underline } from 'lucide-vue-next'
import { ref } from 'vue'
import { Toggle, toggleSizeKeys, toggleVariantKeys } from '.'

// Renders the REAL <Toggle> and pulls its axes from the same key arrays the dev
// wall uses. Colors come from the gray-ladder in style.css (--ui-* tokens), so the
// story is a faithful preview of the shipped press/selected feel.
const meta = {
  title: 'Atoms/Toggle',
  component: Toggle,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: [...toggleVariantKeys] },
    size: { control: 'select', options: [...toggleSizeKeys] },
    disabled: { control: 'boolean' },
  },
  args: {
    variant: 'default',
    size: 'default',
  },
} satisfies Meta<typeof Toggle>

export default meta
type Story = StoryObj<typeof meta>

export const Playground: Story = {
  render: args => ({
    components: { Toggle, Bold },
    setup() {
      const on = ref(false)
      return { args, on }
    },
    template: '<Toggle v-bind="args" v-model:pressed="on" aria-label="Bold"><Bold /></Toggle>',
  }),
}

// default = gray-ladder FILL (selection paints a 243 chip); tint = COLOR-only
// (active brightens the icon, hover stays a calm gray).
export const Variants: Story = {
  render: () => ({
    components: { Toggle, Bold, Italic, Underline, Strikethrough },
    setup() {
      const a = ref({ bold: true, italic: false, underline: false, strike: false })
      const b = ref({ bold: true, italic: false, underline: false, strike: false })
      return { a, b }
    },
    template: `
      <div style="display:flex;flex-direction:column;gap:1.25rem;">
        <div>
          <p style="margin:0 0 .5rem;font-size:.6875rem;text-transform:uppercase;letter-spacing:.04em;color:var(--muted-foreground);">default · fill</p>
          <div style="display:inline-flex;gap:.125rem;border:1px solid var(--border);border-radius:.5rem;padding:.125rem;">
            <Toggle size="sm" v-model:pressed="a.bold" aria-label="Bold"><Bold /></Toggle>
            <Toggle size="sm" v-model:pressed="a.italic" aria-label="Italic"><Italic /></Toggle>
            <Toggle size="sm" v-model:pressed="a.underline" aria-label="Underline"><Underline /></Toggle>
            <Toggle size="sm" v-model:pressed="a.strike" aria-label="Strikethrough"><Strikethrough /></Toggle>
          </div>
        </div>
        <div>
          <p style="margin:0 0 .5rem;font-size:.6875rem;text-transform:uppercase;letter-spacing:.04em;color:var(--muted-foreground);">tint · color-only</p>
          <div style="display:inline-flex;gap:.125rem;border:1px solid var(--border);border-radius:.5rem;padding:.125rem;">
            <Toggle variant="tint" size="sm" v-model:pressed="b.bold" aria-label="Bold"><Bold /></Toggle>
            <Toggle variant="tint" size="sm" v-model:pressed="b.italic" aria-label="Italic"><Italic /></Toggle>
            <Toggle variant="tint" size="sm" v-model:pressed="b.underline" aria-label="Underline"><Underline /></Toggle>
            <Toggle variant="tint" size="sm" v-model:pressed="b.strike" aria-label="Strikethrough"><Strikethrough /></Toggle>
          </div>
        </div>
      </div>`,
  }),
}

export const Sizes: Story = {
  render: () => ({
    components: { Toggle, Bold },
    setup() {
      return { sizes: [...toggleSizeKeys] }
    },
    template: `
      <div style="display:flex;gap:.75rem;align-items:center;">
        <Toggle v-for="s in sizes" :key="s" :size="s" :default-pressed="true" aria-label="Bold"><Bold /></Toggle>
      </div>`,
  }),
}

export const Disabled: Story = {
  render: () => ({
    components: { Toggle, Bold },
    template: `
      <div style="display:flex;gap:.75rem;align-items:center;">
        <Toggle :disabled="true" aria-label="Bold"><Bold /></Toggle>
        <Toggle :disabled="true" :default-pressed="true" aria-label="Bold"><Bold /></Toggle>
      </div>`,
  }),
}
