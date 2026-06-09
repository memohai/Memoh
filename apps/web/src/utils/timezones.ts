const fallbackTimezones = ['UTC']

export const timezones = typeof Intl.supportedValuesOf === 'function'
  ? Intl.supportedValuesOf('timeZone')
  : fallbackTimezones

export const emptyTimezoneValue = '__empty_timezone__'

// Building a label costs one Intl.DateTimeFormat construction (~0.1ms each), so
// across ~418 zones it adds up to ~50ms. Memoise so a given zone is only
// formatted once for the whole app lifetime.
const offsetCache = new Map<string, string>()

export function getUtcOffsetLabel(tz: string): string {
  const cached = offsetCache.get(tz)
  if (cached !== undefined) return cached
  let label = ''
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      timeZoneName: 'shortOffset',
    }).formatToParts(new Date())
    label = parts.find(p => p.type === 'timeZoneName')?.value ?? ''
  } catch {
    label = ''
  }
  offsetCache.set(tz, label)
  return label
}

export interface TimezoneOption {
  value: string
  label: string
  description: string
}

// Precomputed ONCE at module load (not per component instance). The old
// per-instance `computed` rebuilt all ~418 offsets on every TimezoneSelect
// mount — and it mounts behind a v-if on each visit to the profile page — so
// the ~50ms hitch landed right when the user opened the dropdown. Hoisting it
// to module scope pays the cost a single time during bundle eval and reuses the
// result across every instance, mount, and re-open.
export const timezoneOptions: TimezoneOption[] = timezones.map(tz => ({
  value: tz,
  label: tz,
  description: getUtcOffsetLabel(tz),
}))
