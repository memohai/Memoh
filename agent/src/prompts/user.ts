import { ContainerFileAttachment } from '../types'

export interface UserParams {
  contactId: string
  contactName: string
  platform: string
  date: Date
  attachments: ContainerFileAttachment[]
}

export const user = (
  query: string,
  { contactId, contactName, platform, date, attachments }: UserParams
) => {
  const headers = {
    'contact-id': contactId,
    'contact-name': contactName,
    'platform': platform,
    'time': date.toISOString(),
    'attachments': attachments.map(attachment => attachment.path),
  }
  return `
---
${Bun.YAML.stringify(headers)}
---
${query}
  `.trim()
}