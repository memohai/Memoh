// The LOCKED neutral base for the Memoh UI revamp.
//
// Decided by eye in the Scene below: a clean UI consumes only a handful of neutral
// roles, reused with discipline. Surfaces collapse — background == card == popover
// == input == sidebar == one pure-white `surface`; hierarchy comes from a hairline
// border + hover/selected gray, NOT from a different fill per element. Dead-neutral
// (0 chroma); brand purple is used sparingly (solid actions + the user bubble).
//
// This is the single source of truth while tuning lives in the dev wall (no
// style.css edits yet). When the shape scale + states are also locked, these
// values get baked into packages/ui/src/style.css and this file goes away.
//
// Tune by editing the numbers below — Vite HMR repaints the Scene + Scale instantly.

export interface NeutralRamp {
  /** background / card / popover / input fill — the one base surface. */
  surface: string
  /** sidebar + secondary panels — kept == surface here (white), border-separated. */
  surfaceSunken: string
  /** interactive hover background, reused everywhere. */
  hover: string
  /** current/selected row background. */
  selected: string
  /** hairline border (an alpha-black/white rather than a gray). */
  border: string
  /** primary text. */
  foreground: string
  /** secondary text, placeholders, icons. */
  mutedForeground: string
}

export interface BrandRamp {
  /** solid accent — send button, primary action. */
  brand: string
  brandHover: string
  /** tinted fill — user message bubble. The ONE place tint is allowed. */
  brandSoft: string
  /** text/icon on a solid `brand` fill. */
  brandForeground: string
}

export interface Palette {
  id: string
  label: string
  /** one-line description of the direction, shown in the scene. */
  note: string
  light: NeutralRamp
  dark: NeutralRamp
  brand: { light: BrandRamp; dark: BrandRamp }
}

// Memoh brand purple. Mirrors style.css :root / .dark `--brand*`.
const memohBrand: { light: BrandRamp; dark: BrandRamp } = {
  light: {
    brand: 'oklch(0.55 0.22 290)',
    brandHover: 'oklch(0.49 0.23 290)',
    brandSoft: 'oklch(0.96 0.025 290)',
    brandForeground: 'oklch(0.985 0.001 286)',
  },
  dark: {
    brand: 'oklch(0.72 0.16 290)',
    brandHover: 'oklch(0.78 0.15 290)',
    brandSoft: 'oklch(0.30 0.07 290 / 0.55)',
    brandForeground: 'oklch(0.985 0.001 286)',
  },
}

export const palettes: Palette[] = [
  {
    id: 'base',
    label: 'Memoh · neutral',
    note: 'Locked base: surfaces collapse to pure white (255), separated only by a hairline border (not fill). Hover 249, selected 243. Dead-neutral, 0 chroma. Purple used sparingly.',
    light: {
      surface: 'oklch(1 0 0)', // rgb(255,255,255)
      surfaceSunken: 'oklch(1 0 0)', // 255 — sidebar is white too, split by hairline only
      hover: 'oklch(0.982 0 0)', // rgb(249,249,249)
      selected: 'oklch(0.964 0 0)', // rgb(243,243,243)
      border: 'oklch(0 0 0 / 0.09)',
      foreground: 'oklch(0.21 0 0)', // ~#0d0d0d
      mutedForeground: 'oklch(0.55 0 0)', // ~#737373
    },
    dark: {
      surface: 'oklch(0.21 0 0)', // ~#212121
      surfaceSunken: 'oklch(0.17 0 0)', // ~#171717 — dark sidebar a touch darker
      hover: 'oklch(0.27 0 0)',
      selected: 'oklch(0.33 0 0)',
      border: 'oklch(1 1 1 / 0.10)',
      foreground: 'oklch(0.95 0 0)',
      mutedForeground: 'oklch(0.65 0 0)',
    },
    brand: memohBrand,
  },
]

export const defaultPaletteId = 'base'
