import type { Meta, StoryObj } from '@storybook/vue3-vite'
import { ArrowRight, Plus, RefreshCw } from 'lucide-vue-next'
import { Button, buttonSizeKeys, buttonVariantKeys } from '.'

// Stories render the REAL <Button> from the library (never a re-creation) and
// pull the variant/size axes from the same single-source key arrays the dev
// component wall uses — so Storybook can never drift from the shipped component.
const meta = {
  title: 'Atoms/Button',
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: [...buttonVariantKeys] },
    size: { control: 'select', options: [...buttonSizeKeys] },
    loadingMode: { control: 'inline-radio', options: ['overlay', 'icon', 'leading'] },
    loading: { control: 'boolean' },
    disabled: { control: 'boolean' },
    block: { control: 'boolean' },
    as: { control: false },
    asChild: { control: false },
  },
  args: {
    variant: 'default',
    size: 'default',
    loadingMode: 'overlay',
  },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

export const Playground: Story = {
  render: args => ({
    components: { Button },
    setup() {
      return { args }
    },
    template: '<Button v-bind="args">Button</Button>',
  }),
}

export const Variants: Story = {
  render: () => ({
    components: { Button },
    setup() {
      // Links live in their own row (they are inline text, not button boxes).
      const boxes = buttonVariantKeys.filter(v => !v.startsWith('link'))
      const links = buttonVariantKeys.filter(v => v.startsWith('link'))
      return { boxes, links }
    },
    template: `
      <div style="display:flex;flex-direction:column;gap:1.25rem;">
        <div style="display:flex;flex-wrap:wrap;gap:0.75rem;align-items:center;">
          <Button v-for="v in boxes" :key="v" :variant="v">{{ v }}</Button>
        </div>
        <div style="display:flex;flex-wrap:wrap;gap:1.5rem;align-items:center;">
          <Button v-for="v in links" :key="v" :variant="v" as="a" href="#">{{ v }}</Button>
        </div>
      </div>`,
  }),
}

export const Sizes: Story = {
  render: () => ({
    components: { Button, Plus },
    setup() {
      return { sizes: ['lg', 'default', 'sm'] }
    },
    template: `
      <div style="display:flex;flex-direction:column;gap:1rem;">
        <div style="display:flex;gap:0.75rem;align-items:center;">
          <Button v-for="s in sizes" :key="s" :size="s">{{ s }}</Button>
        </div>
        <div style="display:flex;gap:0.75rem;align-items:center;">
          <Button size="icon-lg" aria-label="Add"><Plus /></Button>
          <Button size="icon" aria-label="Add"><Plus /></Button>
          <Button size="icon-sm" aria-label="Add"><Plus /></Button>
        </div>
      </div>`,
  }),
}

export const WithIcons: Story = {
  render: () => ({
    components: { Button, Plus, ArrowRight, RefreshCw },
    template: `
      <div style="display:flex;gap:0.75rem;align-items:center;">
        <Button><Plus />New</Button>
        <Button variant="secondary">Continue<ArrowRight /></Button>
        <Button variant="ghost" size="icon" aria-label="Refresh"><RefreshCw /></Button>
      </div>`,
  }),
}

export const Loading: Story = {
  render: () => ({
    components: { Button, RefreshCw },
    template: `
      <div style="display:flex;gap:0.75rem;align-items:center;">
        <Button :loading="true">Save changes</Button>
        <Button variant="secondary" :loading="true" loading-mode="icon"><RefreshCw />Sync</Button>
        <Button :loading="true" loading-mode="leading">Continue</Button>
      </div>`,
  }),
}

export const Disabled: Story = {
  render: () => ({
    components: { Button },
    setup() {
      return { variants: buttonVariantKeys.filter(v => !v.startsWith('link')) }
    },
    template: `
      <div style="display:flex;flex-wrap:wrap;gap:0.75rem;align-items:center;">
        <Button v-for="v in variants" :key="v" :variant="v" :disabled="true">{{ v }}</Button>
      </div>`,
  }),
}

export const Block: Story = {
  render: () => ({
    components: { Button },
    template: `
      <div style="max-width:24rem;display:flex;flex-direction:column;gap:0.75rem;">
        <Button :block="true">Continue</Button>
        <Button variant="secondary" :block="true">Cancel</Button>
      </div>`,
  }),
}
