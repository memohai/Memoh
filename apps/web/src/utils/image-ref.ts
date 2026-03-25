// Keep this display helper aligned with internal/config.NormalizeImageRef.
export function shortenImageRef(value: string | null | undefined): string {
  const ref = value?.trim() ?? ''
  if (!ref) return ''
  if (ref.startsWith('docker.io/library/')) return ref.slice('docker.io/library/'.length)
  if (ref.startsWith('docker.io/')) return ref.slice('docker.io/'.length)
  return ref
}
