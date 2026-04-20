export interface CompatibilityMeta {
  value: string
  label: string
}

export const CHAT_COMPATIBILITY_OPTIONS: CompatibilityMeta[] = [
  { value: 'vision', label: 'Vision' },
  { value: 'tool-call', label: 'Tool Call' },
  { value: 'image-output', label: 'Image Output' },
  { value: 'reasoning', label: 'Reasoning' },
]

export const IMAGE_COMPATIBILITY_OPTIONS: CompatibilityMeta[] = [
  { value: 'generate', label: 'Generate' },
  { value: 'edit', label: 'Edit' },
]

export const COMPATIBILITY_OPTIONS: CompatibilityMeta[] = [
  ...CHAT_COMPATIBILITY_OPTIONS,
  ...IMAGE_COMPATIBILITY_OPTIONS,
]
