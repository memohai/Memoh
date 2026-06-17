export const colorSchemeIds = ['memoh', 'ocean', 'forest', 'rose', 'amber'] as const

export type ColorSchemeId = typeof colorSchemeIds[number]

export interface ColorSchemeOption {
  id: ColorSchemeId
  labelKey: string
  descriptionKey: string
  swatches: string[]
  darkSwatches: string[]
}

export const colorSchemes: ColorSchemeOption[] = [
  {
    id: 'memoh',
    labelKey: 'settings.appearance.colorSchemes.memoh',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.memoh',
    swatches: ['oklch(0.984 0.0024 72)', 'rgb(255 255 255)', 'oklch(0.21 0.004 95)', 'oklch(0.52 0.006 95)', 'oklch(0.55 0.22 290)'],
    darkSwatches: ['oklch(0.152 0 0)', 'oklch(0.21 0 0)', 'oklch(0.86 0 0)', 'oklch(0.62 0 0)', 'oklch(0.72 0.16 290)'],
  },
  {
    id: 'ocean',
    labelKey: 'settings.appearance.colorSchemes.ocean',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.ocean',
    swatches: ['oklch(0.984 0.0024 230)', 'rgb(255 255 255)', 'oklch(0.21 0.004 230)', 'oklch(0.52 0.006 230)', 'oklch(0.56 0.15 230)'],
    darkSwatches: ['oklch(0.152 0.006 230)', 'oklch(0.21 0.006 230)', 'oklch(0.86 0.004 230)', 'oklch(0.62 0.006 230)', 'oklch(0.72 0.13 230)'],
  },
  {
    id: 'forest',
    labelKey: 'settings.appearance.colorSchemes.forest',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.forest',
    swatches: ['oklch(0.984 0.0024 150)', 'rgb(255 255 255)', 'oklch(0.21 0.004 150)', 'oklch(0.52 0.006 150)', 'oklch(0.50 0.14 150)'],
    darkSwatches: ['oklch(0.152 0.006 150)', 'oklch(0.21 0.006 150)', 'oklch(0.86 0.004 150)', 'oklch(0.62 0.006 150)', 'oklch(0.66 0.12 150)'],
  },
  {
    id: 'rose',
    labelKey: 'settings.appearance.colorSchemes.rose',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.rose',
    swatches: ['oklch(0.984 0.0024 355)', 'rgb(255 255 255)', 'oklch(0.21 0.004 355)', 'oklch(0.52 0.006 355)', 'oklch(0.58 0.18 355)'],
    darkSwatches: ['oklch(0.152 0.006 355)', 'oklch(0.21 0.006 355)', 'oklch(0.86 0.004 355)', 'oklch(0.62 0.006 355)', 'oklch(0.74 0.16 355)'],
  },
  {
    id: 'amber',
    labelKey: 'settings.appearance.colorSchemes.amber',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.amber',
    swatches: ['oklch(0.984 0.0024 70)', 'rgb(255 255 255)', 'oklch(0.21 0.004 70)', 'oklch(0.52 0.006 70)', 'oklch(0.62 0.15 70)'],
    darkSwatches: ['oklch(0.152 0.006 70)', 'oklch(0.21 0.006 70)', 'oklch(0.86 0.004 70)', 'oklch(0.62 0.006 70)', 'oklch(0.78 0.13 70)'],
  },
]

export function isColorSchemeId(value: string): value is ColorSchemeId {
  return colorSchemeIds.includes(value as ColorSchemeId)
}
