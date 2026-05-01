export type ConfigSchemaFieldType =
  | 'string'
  | 'secret'
  | 'number'
  | 'bool'
  | 'enum'
  | 'textarea'

export interface ConfigSchemaConstraint {
  min?: number
  max?: number
  step?: number
}

export interface ConfigSchemaField {
  key: string
  type: ConfigSchemaFieldType
  required?: boolean
  title?: string
  description?: string
  placeholder?: string
  default?: unknown
  example?: unknown
  order?: number
  enum?: string[]
  multiline?: boolean
  readonly?: boolean
  secret?: boolean
  collapsed?: boolean
  constraint?: ConfigSchemaConstraint | null
}

export interface ConfigSchema {
  version?: number
  title?: string
  fields: ConfigSchemaField[]
}

export interface ConfigActionStatus {
  enabled: boolean
  reason?: string
}

export interface ConfigAction {
  id: string
  type: string
  label: string
  description?: string
  primary?: boolean
  status?: ConfigActionStatus | null
}

export interface ConfigStatus {
  state: string
  title?: string
  description?: string
  details?: Record<string, unknown> | null
}

