export const MERMAID_THEMES = ['auto', 'default', 'dark', 'forest', 'neutral'] as const
export type MermaidTheme = typeof MERMAID_THEMES[number]
export const DEFAULT_MERMAID_THEME: MermaidTheme = 'auto'

export function isMermaidTheme(value: unknown): value is MermaidTheme {
  return typeof value === 'string' && (MERMAID_THEMES as readonly string[]).includes(value)
}

export function resolveMermaidIsDark(theme: MermaidTheme, fallbackIsDark: boolean): boolean {
  if (theme === 'auto') return fallbackIsDark
  return theme === 'dark'
}

// mermaid honors an inline `%%{init: ...}%%` directive at the top of the source
// for both parsing and rendering. If the user already authored one we leave it
// alone — their config wins, matching the worker's own injection logic.
const INIT_DIRECTIVE_PREFIX = /^\s*%%\{/

export function applyMermaidThemeToSource(source: string, theme: MermaidTheme): string {
  if (theme === 'auto') return source
  if (INIT_DIRECTIVE_PREFIX.test(source)) return source
  return `%%{init: {"theme":"${theme}"}}%%\n${source}`
}
