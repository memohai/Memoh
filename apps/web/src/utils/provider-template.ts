import type { ProvidersGetResponse, ProvidertemplatesGetResponse } from '@memohai/sdk'

export interface ProviderConfigField {
  key: string
  type: string
  title: string
  description: string
  required: boolean
  secret: boolean
  example?: unknown
  enum: string[]
  order: number
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function normalizeField(key: string, value: unknown, index: number): ProviderConfigField {
  const field = asRecord(value) ?? {}
  const type = typeof field.type === 'string' ? field.type : 'string'
  return {
    key,
    type,
    title: typeof field.title === 'string' ? field.title : key,
    description: typeof field.description === 'string' ? field.description : '',
    required: field.required === true,
    secret: field.secret === true || type === 'secret' || type === 'password',
    example: field.example,
    enum: Array.isArray(field.enum) ? field.enum.filter((item): item is string => typeof item === 'string') : [],
    order: typeof field.order === 'number' ? field.order : index,
  }
}

export function normalizeProviderConfigFields(schema: unknown): ProviderConfigField[] {
  const fields = asRecord(schema)?.fields
  if (Array.isArray(fields)) {
    return fields
      .map((field, index) => {
        const record = asRecord(field)
        const key = typeof record?.key === 'string' ? record.key : ''
        return key ? normalizeField(key, record, index) : null
      })
      .filter((field): field is ProviderConfigField => field !== null)
      .sort((a, b) => a.order - b.order)
  }

  const record = asRecord(fields)
  if (!record) return []
  return Object.entries(record)
    .map(([key, field], index) => normalizeField(key, field, index))
    .sort((a, b) => a.order - b.order)
}

export function templateConfigFields(template?: ProvidertemplatesGetResponse | null): ProviderConfigField[] {
  return normalizeProviderConfigFields(template?.config_schema)
}

export function templateDefaultConfig(template?: ProvidertemplatesGetResponse | null): Record<string, unknown> {
  return { ...((asRecord(template?.default_config) ?? {})) }
}

export function providerConfigDefaults(schema: unknown): Record<string, unknown> {
  return Object.fromEntries(normalizeProviderConfigFields(schema)
    .filter(field => !field.secret && field.example !== undefined)
    .map(field => [field.key, field.example]))
}

export function providerDraftFromTemplate(template: ProvidertemplatesGetResponse): ProvidersGetResponse {
  return {
    provider_template_id: template.id,
    name: template.name ?? template.key ?? '',
    client_type: template.driver ?? '',
    icon: template.icon,
    enable: false,
    config: templateDefaultConfig(template),
    metadata: { ...((asRecord(template.metadata) ?? {})) },
  }
}

export function isTemplateConfigured(template: ProvidertemplatesGetResponse): boolean {
  if (template.configured === true) return true
  return asRecord(template.metadata)?.configured === true
}
