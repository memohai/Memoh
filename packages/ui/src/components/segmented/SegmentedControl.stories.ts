import type { Meta, StoryObj } from '@storybook/vue3-vite'
import { ref } from 'vue'
import { SegmentedControl } from '.'

// Renders the REAL <SegmentedControl>. The thumb geometry is measured at runtime,
// so these stories exercise the actual slide/press feel — not a static mock.
// SegmentedControl is a GENERIC component (`<T>`); Storybook's Meta can't infer a
// concrete prop shape from a generic functional component, so we cast it to the
// expected component slot and keep stories render-only (no args binding).
const meta = {
  title: 'Atoms/SegmentedControl',
  component: SegmentedControl as unknown as Meta['component'],
  tags: ['autodocs'],
} satisfies Meta

export default meta
type Story = StoryObj

export const Default: Story = {
  render: () => ({
    components: { SegmentedControl },
    setup() {
      const value = ref('week')
      const items = [
        { value: 'day', label: 'Day' },
        { value: 'week', label: 'Week' },
        { value: 'month', label: 'Month' },
      ]
      return { value, items }
    },
    template: `
      <div style="display:flex;align-items:center;gap:1rem;">
        <SegmentedControl v-model="value" :items="items" aria-label="Range" />
        <span style="font-size:.75rem;color:var(--muted-foreground);">→ {{ value }}</span>
      </div>`,
  }),
}

// Uneven label widths prove the thumb measures + slides to each item's real box.
export const VariableWidths: Story = {
  render: () => ({
    components: { SegmentedControl },
    setup() {
      const value = ref('all')
      const items = [
        { value: 'all', label: 'All' },
        { value: 'active', label: 'Active' },
        { value: 'archived', label: 'Archived' },
      ]
      return { value, items }
    },
    template: '<SegmentedControl v-model="value" :items="items" aria-label="Filter" />',
  }),
}

export const WithDisabledOption: Story = {
  render: () => ({
    components: { SegmentedControl },
    setup() {
      const value = ref('list')
      const items = [
        { value: 'list', label: 'List' },
        { value: 'board', label: 'Board' },
        { value: 'calendar', label: 'Calendar', disabled: true },
      ]
      return { value, items }
    },
    template: '<SegmentedControl v-model="value" :items="items" aria-label="View" />',
  }),
}
