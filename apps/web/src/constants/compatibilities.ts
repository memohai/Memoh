export interface CompatibilityMeta {
  value: string
  label: string
}

export const COMPATIBILITY_OPTIONS: CompatibilityMeta[] = [
  { value: 'vision', label: 'Vision' },
  { value: 'tool-call', label: 'Tool Call' },
  { value: 'image-output', label: 'Image Output' },
  { value: 'reasoning', label: 'Reasoning' },
]
