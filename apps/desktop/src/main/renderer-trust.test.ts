import { describe, expect, it } from 'vitest'
import { pathToFileURL } from 'node:url'
import { isTrustedRendererUrl } from './renderer-trust'

describe('isTrustedRendererUrl', () => {
  const productionEntry = '/Applications/Memoh/resources/app.asar/out/renderer/index.html'

  it('allows only the exact development renderer entry', () => {
    const options = { devBaseUrl: 'http://127.0.0.1:8082', productionEntry }
    expect(isTrustedRendererUrl('http://127.0.0.1:8082/index.html?mode=desktop#chat', options)).toBe(true)
    expect(isTrustedRendererUrl('http://127.0.0.1:8082/attacker.html', options)).toBe(false)
    expect(isTrustedRendererUrl('https://example.com/index.html', options)).toBe(false)
  })

  it('allows only the packaged renderer file', () => {
    const options = { productionEntry }
    expect(isTrustedRendererUrl(`${pathToFileURL(productionEntry).href}?mode=desktop`, options)).toBe(true)
    expect(isTrustedRendererUrl(pathToFileURL('/tmp/untrusted.html').href, options)).toBe(false)
    expect(isTrustedRendererUrl('https://example.com/', options)).toBe(false)
  })
})
