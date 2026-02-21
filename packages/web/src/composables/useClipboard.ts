export function useClipboard() {
  const isSupported = typeof navigator !== 'undefined' && !!navigator.clipboard?.writeText

  async function copyText(text: string): Promise<boolean> {
    if (!isSupported) {
      return false
    }
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      return false
    }
  }

  return {
    isSupported,
    copyText,
  }
}
