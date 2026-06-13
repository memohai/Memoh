let warmed = false

export function warmCodeHighlightOnIdle() {
  if (warmed || typeof window === 'undefined')
    return
  warmed = true

  const warm = () => {
    void import('stream-markdown')
      .then(m => m.registerHighlight())
      .catch(() => {})
  }

  if (typeof window.requestIdleCallback === 'function')
    window.requestIdleCallback(() => warm(), { timeout: 3000 })
  else
    window.setTimeout(warm, 1500)
}
