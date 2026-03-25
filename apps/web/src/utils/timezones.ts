const fallbackTimezones = ['UTC']

export const timezones = typeof Intl.supportedValuesOf === 'function'
  ? Intl.supportedValuesOf('timeZone')
  : fallbackTimezones

export const emptyTimezoneValue = '__empty_timezone__'
