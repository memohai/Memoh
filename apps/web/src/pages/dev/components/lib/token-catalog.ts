// Catalog of every semantic color token declared in `@theme inline`
// (packages/ui/src/style.css), grouped by family. Single source of truth for
// the token swatch wall. Names are the base token name WITHOUT the `--color-`
// prefix; the swatch renders `var(--color-${name})`.
//
// Keep in sync with style.css when tokens are added/removed. Intentionally
// excludes non-color tokens (--radius-*, --scrollbar-*, --terminal-*).

export interface TokenGroup {
  id: string
  label: string
  /** Token base names (no `--color-` prefix). */
  tokens: string[]
}

export const tokenGroups: TokenGroup[] = [
  {
    id: 'surface',
    label: 'Surface (background · hover · selected)',
    tokens: [
      'background', 'foreground',
      'accent', 'accent-foreground',
      'muted', 'muted-foreground',
      'border', 'input', 'ring',
    ],
  },
  {
    id: 'core',
    label: 'Core',
    tokens: [
      'card', 'card-foreground',
      'popover', 'popover-foreground',
      'primary', 'primary-foreground',
      'secondary', 'secondary-foreground',
      'destructive', 'destructive-foreground',
    ],
  },
  {
    id: 'brand',
    label: 'Brand',
    tokens: ['brand', 'brand-foreground', 'brand-soft', 'brand-border', 'brand-hover'],
  },
  {
    id: 'status',
    label: 'Status (success / warning / info)',
    tokens: [
      'success', 'success-foreground', 'success-solid-foreground', 'success-soft', 'success-border',
      'warning', 'warning-foreground', 'warning-solid-foreground', 'warning-soft', 'warning-border',
      'info', 'info-foreground', 'info-soft', 'info-border',
    ],
  },
  {
    id: 'chart',
    label: 'Chart',
    tokens: ['chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5'],
  },
  {
    id: 'sidebar',
    label: 'Sidebar',
    tokens: [
      'sidebar', 'sidebar-foreground',
      'sidebar-primary', 'sidebar-primary-foreground',
      'sidebar-accent', 'sidebar-accent-foreground',
      'sidebar-border', 'sidebar-ring',
    ],
  },
  {
    id: 'event',
    label: 'Event',
    tokens: [
      'event-schedule', 'event-schedule-foreground', 'event-schedule-soft', 'event-schedule-border',
      'event-heartbeat', 'event-heartbeat-foreground', 'event-heartbeat-soft', 'event-heartbeat-border',
      'event-subagent', 'event-subagent-foreground', 'event-subagent-soft', 'event-subagent-border',
      'event-discuss', 'event-discuss-foreground', 'event-discuss-soft', 'event-discuss-border',
    ],
  },
  {
    id: 'capability',
    label: 'Capability',
    tokens: [
      'capability-tool', 'capability-tool-foreground', 'capability-tool-soft',
      'capability-vision', 'capability-vision-foreground', 'capability-vision-soft',
      'capability-image', 'capability-image-foreground', 'capability-image-soft',
      'capability-reasoning', 'capability-reasoning-foreground', 'capability-reasoning-soft',
    ],
  },
  {
    id: 'context-window',
    label: 'Context window',
    tokens: [
      'context-window-xs', 'context-window-sm', 'context-window-md',
      'context-window-lg', 'context-window-xl', 'context-window-foreground',
    ],
  },
  {
    id: 'diff',
    label: 'Diff',
    tokens: ['diff-add', 'diff-add-border', 'diff-remove', 'diff-remove-border'],
  },
]

/** A token name is a "foreground" token (rendered as text-on-surface sample). */
export function isForeground(name: string): boolean {
  return name.endsWith('-foreground')
}
