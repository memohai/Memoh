// Shared metric formatters for container/runtime surfaces (Overview runtime
// block + the Container tab). Kept in one place so both render bytes and
// percentages identically. Both return '--' for missing/invalid input so a
// not-yet-sampled metric reads as "unknown", not "0".

export function formatMetricBytes(value?: number): string {
  if (typeof value !== 'number' || Number.isNaN(value) || value < 0) return '--'
  if (value === 0) return '0 B'

  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let size = value
  let unitIndex = 0

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }

  const fractionDigits = size >= 100 || unitIndex === 0 ? 0 : 1
  return `${size.toFixed(fractionDigits)} ${units[unitIndex]}`
}

export function formatMetricPercent(value?: number): string {
  if (typeof value !== 'number' || Number.isNaN(value) || value < 0) return '--'
  const fractionDigits = value >= 100 ? 0 : 1
  return `${value.toFixed(fractionDigits)}%`
}
