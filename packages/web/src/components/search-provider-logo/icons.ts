/**
 * Search provider icon registry (FontAwesome).
 *
 * To add a new provider icon:
 * 1. Find the icon in FontAwesome (https://fontawesome.com/icons)
 *    or add a custom definition in custom-icons.ts
 * 2. Import it in `main.ts` and add to `library.add()`
 * 3. Add the [prefix, iconName] tuple to PROVIDER_ICONS below
 *
 * The key must match the `provider` field stored in the database (lowercase).
 */

const PROVIDER_ICONS: Record<string, [string, string]> = {
  brave: ['fab', 'brave'],
  bing: ['fab', 'microsoft'],
  google: ['fab', 'google'],
  yandex: ['fab', 'yandex'],
  tavily: ['fac', 'tavily'],
  jina: ['fac', 'jina'],
  exa: ['fac', 'exa'],
  bocha: ['fac', 'bocha'],
  duckduckgo: ['fac', 'duckduckgo'],
  searxng: ['fac', 'searxng'],
  sogou: ['fac', 'sogou'],
  serper: ['fac', 'serper'],
}

const DEFAULT_ICON: [string, string] = ['fas', 'globe']

export function getSearchProviderIcon(provider: string): [string, string] {
  if (!provider) return DEFAULT_ICON
  return PROVIDER_ICONS[provider.trim().toLowerCase()] ?? DEFAULT_ICON
}
