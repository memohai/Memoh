const CJK_SYS = '"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", "Noto Sans SC", sans-serif'

export interface FontOption {
  id: FontId
  name: string
  family: string
  href?: string
  /** Whether this font natively covers CJK glyphs (no system fallback needed for Chinese) */
  cjk: boolean
}

export const fontIds = [
  'system',
  'noto-sans-sc',
  'misans',
  'harmony-sans',
  'oppo-sans',
  'geist',
  'inter',
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
  },
  {
    id: 'noto-sans-sc',
    name: 'Noto Sans SC',
    family: '"Noto Sans SC", sans-serif',
    href: 'https://fonts.googleapis.com/css2?family=Noto+Sans+SC:wght@300;400;500;600;700&display=swap',
    cjk: true,
  },
  {
    id: 'misans',
    name: 'MiSans',
    family: '"MiSans", "MiSans SC", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", sans-serif',
    // Xiaomi HyperOS open-source font — Latin + Simplified Chinese in one family
    href: 'https://cdn.jsdelivr.net/npm/misans-sc@4.0.1/dist/MiSans.css',
    cjk: true,
  },
  {
    id: 'harmony-sans',
    name: 'HarmonyOS Sans',
    family: '"HarmonyOS Sans SC", "HarmonyOS Sans", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", sans-serif',
    // Huawei HarmonyOS open-source font — Latin + CJK unified
    href: 'https://cdn.jsdelivr.net/gh/hurub/HarmonyOS_Sans@main/fonts.css',
    cjk: true,
  },
  {
    id: 'oppo-sans',
    name: 'OPPO Sans',
    family: '"OPPO Sans", "OPPOSans", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", sans-serif',
    // OPPO open-source font — Latin + Simplified Chinese in one family
    href: 'https://cdn.jsdelivr.net/gh/OPPO-OpenPlatform/OPPOSans@main/font.css',
    cjk: true,
  },
  {
    id: 'inter',
    name: 'Inter',
    family: `"Inter", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap',
    cjk: false,
  },
  {
    id: 'geist',
    name: 'Geist',
    family: `"Geist", ${CJK_SYS}`,
    href: 'https://cdn.jsdelivr.net/npm/geist@1.3.0/dist/fonts/geist-sans/style.css',
    cjk: false,
  },
  {
    id: 'plus-jakarta-sans',
    name: 'Plus Jakarta Sans',
    family: `"Plus Jakarta Sans", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:ital,wght@0,300;0,400;0,500;0,600;0,700;1,400&display=swap',
    cjk: false,
  },
  {
    id: 'dm-sans',
    name: 'DM Sans',
    family: `"DM Sans", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300;0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700;1,9..40,400&display=swap',
    cjk: false,
  },
  {
    id: 'outfit',
    name: 'Outfit',
    family: `"Outfit", ${CJK_SYS}`,
    href: 'https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap',
    cjk: false,
  },
]

export function isFontId(value: string): value is FontId {
  return fontIds.includes(value as FontId)
}

export function getFontById(id: string): FontOption | undefined {
  return fonts.find(f => f.id === id)
}
