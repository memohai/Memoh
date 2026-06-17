import { describe, expect, it } from 'vitest'
import {
  applyMermaidThemeToSource,
  DEFAULT_MERMAID_THEME,
  isMermaidTheme,
  MERMAID_THEMES,
  resolveMermaidIsDark,
} from './mermaid'

describe('mermaid theme settings', () => {
  it('defaults to following the interface theme', () => {
    expect(DEFAULT_MERMAID_THEME).toBe('auto')
    expect(MERMAID_THEMES).toContain('auto')
  })

  it('validates known theme ids', () => {
    expect(isMermaidTheme('dark')).toBe(true)
    expect(isMermaidTheme('forest')).toBe(true)
    expect(isMermaidTheme('chartreuse')).toBe(false)
    expect(isMermaidTheme(null)).toBe(false)
  })

  it('resolves isDark from the chosen theme, falling back to the interface flag', () => {
    expect(resolveMermaidIsDark('auto', false)).toBe(false)
    expect(resolveMermaidIsDark('auto', true)).toBe(true)
    expect(resolveMermaidIsDark('dark', false)).toBe(true)
    expect(resolveMermaidIsDark('default', true)).toBe(false)
    expect(resolveMermaidIsDark('forest', true)).toBe(false)
    expect(resolveMermaidIsDark('neutral', true)).toBe(false)
  })

  it('passes source through untouched for auto', () => {
    const source = 'graph TD\n  A-->B'
    expect(applyMermaidThemeToSource(source, 'auto')).toBe(source)
  })

  it('prepends an init directive when the user has not set one', () => {
    expect(applyMermaidThemeToSource('graph TD\n  A-->B', 'forest'))
      .toBe('%%{init: {"theme":"forest"}}%%\ngraph TD\n  A-->B')
  })

  it('keeps a user-authored init directive untouched', () => {
    const source = '%%{init: {"theme":"base","themeVariables":{"primaryColor":"#fff"}}}%%\ngraph TD\n  A-->B'
    expect(applyMermaidThemeToSource(source, 'dark')).toBe(source)
  })

  it('treats leading whitespace before %%{ as already-configured', () => {
    const source = '   \n%%{init: {"theme":"base"}}%%\ngraph TD'
    expect(applyMermaidThemeToSource(source, 'dark')).toBe(source)
  })
})
