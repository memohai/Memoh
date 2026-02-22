export function useClipboard() {
  const hasNavigatorClipboard = typeof navigator !== 'undefined' && !!navigator.clipboard?.writeText
  const hasExecCommandFallback = typeof document !== 'undefined' && typeof document.execCommand === 'function'
  const isSupported = hasNavigatorClipboard || hasExecCommandFallback

  async function copyText(text: string): Promise<boolean> {
    if (!isSupported) {
      return false
    }

    try {
      if (hasNavigatorClipboard) {
        await navigator.clipboard.writeText(text)
        return true
      }

      if (!hasExecCommandFallback || typeof document === 'undefined') {
        return false
      }

      const textArea = document.createElement('textarea')
      textArea.value = text
      textArea.style.position = 'fixed'
      textArea.style.left = '-9999px'
      textArea.style.top = '0'
      document.body.appendChild(textArea)
      textArea.focus()
      textArea.select()
      const success = document.execCommand('copy')
      document.body.removeChild(textArea)
      return success
    } catch {
      return false
    }
  }

  return {
    isSupported,
    copyText,
  }
}
