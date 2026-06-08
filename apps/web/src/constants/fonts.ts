const CJK_SYS = '"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", "Noto Sans SC", sans-serif'

export interface FontOption {
  id: FontId
  name: string
  family: string
  href?: string
  /** Whether this font natively covers CJK glyphs */
  cjk: boolean
  /**
   * Brief evaluation note shown on the font card.
   * Keep to one short line — it's for screenshot evaluation, not documentation.
   */
  note: string
}

export const fontIds = [
  'system',
  'noto-sans-sc',
  'misans',
  'harmony-sans',
  'oppo-sans',
  'inter',
  'geist',
  'plus-jakarta-sans',
  'dm-sans',
  'outfit',
] as const

export type FontId = typeof fontIds[number]

export const fonts: FontOption[] = [
  {
    id: 'system',
    name: 'System',
    family: `system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", ${CJK_SYS}`,
    cjk: false,
    note: 'SF Pro (macOS) · Segoe UI (Windows). The baseline — everything else is judged against this.',
  },
  {
    id: 'noto-sans-sc',
    name: 'Noto Sans SC',
    family: '"Noto Sans SC", sans-serif',
    href: 'https://fonts.googleapis.com/css2?family=Noto+Sans+SC:wght@300;400;500;600;700&display=swap',
    cjk: true,
    note: 'Google\'s universal humanist sans. Balanced weight, zero personality. Best guaranteed CJK cross-platform consistency.',
  },
  {
    id: 'misans',
    name: 'MiSans',
    family: '"MiSans", "Mi Sans", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", sans-serif',
    href: 'https://fonts.cdnfonts.com/css/misans',
    cjk: true,
    note: 'Xiaomi HyperOS font. Latin weight runs lighter than System — Chinese character is where it earns its keep.',
  },
  {
    id: 'harmony-sans',
    name: 'HarmonyOS Sans',
    family: '"HarmonyOS Sans SC", "HarmonyOS Sans", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", sans-serif',
    href: 'https://fonts.cdnfonts.com/css/harmonyos-sans',
    cjk: true,
    note: 'Huawei\'s system font. Slightly rounder, friendlier. Latin nearly identical to MiSans; CJK is subtly warmer.',
  },
  {
    id: 'oppo-sans',
    name: 'OPPO Sans',
    family: '"OPPO Sans", "OPPOSans", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", sans-serif',
    href: 'https://fonts.cdnfonts.com/css/oppo-sans-4',
    cjk: true,
    note: 'Elegant proportions. Latin glyphs near-identical to MiSans and HarmonyOS — the three diverge mainly in CJK strokes.',
  },
  {
    id: 'inter',
    name: 'Inter',
    family: `"Inter", ${CJK_SYS}`,
    // Loaded eagerly in index.html — no runtime href needed
    cjk: false,
    note: 'The web standard. Default weight runs heavier than System. Neutral to a fault — almost invisible as a design choice.',
  },
  {
    id: 'geist',
    name: 'Geist',
    family: `"Geist", ${CJK_SYS}`,
    href: 'https://cdn.jsdelivr.net/npm/geist@1.3.0/dist/fonts/geist-sans/style.css',
    cjk: false,
    note: 'Vercel\'s UI font — not a code font (that\'s Geist Mono). Same geometric base as Inter, more personality in numerals and punctuation.',
  },
  {
    id: 'plus-jakarta-sans',
    name: 'Plus Jakarta Sans',
    family: `"Plus Jakarta Sans", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:ital,wght@0,300;0,400;0,500;0,600;0,700;1,400&display=swap',
    cjk: false,
    note: 'Humanist with clear personality. Double-story a, distinct counters. Warm and approachable — product tone alignment matters here.',
  },
  {
    id: 'dm-sans',
    name: 'DM Sans',
    family: `"DM Sans", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300;0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700;1,9..40,400&display=swap',
    cjk: false,
    note: 'Inter-adjacent but measurably different. Slightly wider, more relaxed. The g and s carry mild editorial character.',
  },
  {
    id: 'outfit',
    name: 'Outfit',
    family: `"Outfit", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap',
    cjk: false,
    note: 'Near-perfect geometric circles. Strongly designed — excellent for display headings, can overpower dense UI text.',
  },
]

export function isFontId(value: string): value is FontId {
  return fontIds.includes(value as FontId)
}

export function getFontById(id: string): FontOption | undefined {
  return fonts.find(f => f.id === id)
}
