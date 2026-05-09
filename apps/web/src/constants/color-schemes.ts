export const colorSchemeIds = ['memoh', 'ocean', 'forest', 'rose', 'amber'] as const

export type ColorSchemeId = typeof colorSchemeIds[number]

export interface ColorSchemeOption {
  id: ColorSchemeId
  labelKey: string
  descriptionKey: string
  swatches: string[]
}

export const colorSchemes: ColorSchemeOption[] = [
  {
    id: 'memoh',
    labelKey: 'settings.appearance.colorSchemes.memoh',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.memoh',
    swatches: ['oklch(0.22 0.006 286)', 'oklch(0.985 0.001 286)', 'oklch(0.967 0.001 286.375)', 'oklch(0.92 0.004 286.32)', 'oklch(0.55 0.22 290)', 'oklch(0.62 0.16 150)', 'oklch(0.72 0.15 75)', 'oklch(0.60 0.19 355)'],
  },
  {
    id: 'ocean',
    labelKey: 'settings.appearance.colorSchemes.ocean',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.ocean',
    swatches: ['oklch(0.22 0.006 286)', 'oklch(0.985 0.001 286)', 'oklch(0.967 0.001 286.375)', 'oklch(0.92 0.004 286.32)', 'oklch(0.56 0.15 230)', 'oklch(0.62 0.14 170)', 'oklch(0.72 0.14 80)', 'oklch(0.60 0.17 345)'],
  },
  {
    id: 'forest',
    labelKey: 'settings.appearance.colorSchemes.forest',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.forest',
    swatches: ['oklch(0.22 0.006 286)', 'oklch(0.985 0.001 286)', 'oklch(0.967 0.001 286.375)', 'oklch(0.92 0.004 286.32)', 'oklch(0.50 0.14 150)', 'oklch(0.58 0.15 145)', 'oklch(0.72 0.14 80)', 'oklch(0.58 0.16 20)'],
  },
  {
    id: 'rose',
    labelKey: 'settings.appearance.colorSchemes.rose',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.rose',
    swatches: ['oklch(0.22 0.006 286)', 'oklch(0.985 0.001 286)', 'oklch(0.967 0.001 286.375)', 'oklch(0.92 0.004 286.32)', 'oklch(0.58 0.18 355)', 'oklch(0.62 0.14 155)', 'oklch(0.72 0.14 75)', 'oklch(0.58 0.14 250)'],
  },
  {
    id: 'amber',
    labelKey: 'settings.appearance.colorSchemes.amber',
    descriptionKey: 'settings.appearance.colorSchemeDescriptions.amber',
    swatches: ['oklch(0.22 0.006 286)', 'oklch(0.985 0.001 286)', 'oklch(0.967 0.001 286.375)', 'oklch(0.92 0.004 286.32)', 'oklch(0.62 0.15 70)', 'oklch(0.58 0.14 145)', 'oklch(0.70 0.15 75)', 'oklch(0.56 0.14 230)'],
  },
]

export function isColorSchemeId(value: string): value is ColorSchemeId {
  return colorSchemeIds.includes(value as ColorSchemeId)
}
