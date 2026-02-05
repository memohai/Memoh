export interface UserParams {
  contactId: string
  contactName: string
  platform: string
  date: Date
}

export const user = (
  query: string,
  { contactId, contactName, platform, date }: UserParams
) => {
  const headers = {
    'contact-id': contactId,
    'contact-name': contactName,
    'platform': platform,
    'time': date.toISOString(),
  }
  return `
---
${Bun.YAML.stringify(headers)}
---
${query}
  `.trim()
}