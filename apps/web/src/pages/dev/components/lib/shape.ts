// Radius + elevation SCALE — the single source of truth. Each step lists the
// component ROLES that map to it. The discipline: a component picks a STEP, never
// a one-off px. Tune the few step values here by eye (Vite HMR repaints every
// role-mapped specimen in the Scale section); once happy, freeze into
// packages/ui/src/style.css as --radius-* and a new --shadow-* set. Product code
// then references step names, not raw values.

export interface RadiusStep {
  name: string
  value: string
  roles: string
}
export const radiusScale: RadiusStep[] = [
  { name: 'sm', value: '6px', roles: 'badges · tags · kbd' },
  { name: 'md', value: '8px', roles: 'buttons · inputs(rect) · menu items · rows' },
  { name: 'lg', value: '12px', roles: 'cards · popover/menu container · panels' },
  { name: 'xl', value: '16px', roles: 'dialogs · sheets' },
  { name: 'full', value: '9999px', roles: 'pills · avatars · send · pill-input icons' },
]

export interface ElevationStep {
  name: string
  value: string
  roles: string
}
// Layered + ultra-low-opacity + negative spread, tuned for a white surface; the
// element's hairline border keeps edges crisp so the shadow only lifts.
export const elevationScale: ElevationStep[] = [
  { name: '0', value: 'none', roles: 'inline · list rows · docked input (border only)' },
  { name: '1', value: '0 1px 2px 0 oklch(0 0 0 / 0.04)', roles: 'resting card' },
  {
    name: '2',
    value: '0 1px 2px 0 oklch(0 0 0 / 0.04), 0 6px 18px -6px oklch(0 0 0 / 0.08)',
    roles: 'dropdown · popover · Plus menu',
  },
  {
    name: '3',
    value: '0 2px 6px -1px oklch(0 0 0 / 0.05), 0 14px 36px -8px oklch(0 0 0 / 0.10)',
    roles: 'dialog · sheet',
  },
  {
    name: '4',
    value: '0 4px 10px -2px oklch(0 0 0 / 0.06), 0 24px 56px -12px oklch(0 0 0 / 0.14)',
    roles: 'modal over scrim',
  },
]

export const borderHairline = 'oklch(0 0 0 / 0.09)'
