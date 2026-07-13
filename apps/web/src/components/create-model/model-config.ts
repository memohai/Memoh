import type { ModelsModelConfig } from '@memohai/sdk'

interface BuildModelConfigInput {
  type: string
  description?: string
  dimensions?: number
  contextWindow?: number
  compatibilities: string[]
  existing?: ModelsModelConfig
}

export function buildModelConfig(input: BuildModelConfigInput): ModelsModelConfig {
  const config: ModelsModelConfig = { ...(input.existing ?? {}) }
  const description = input.description?.trim() ?? ''

  if (input.existing) config.description = description
  else if (description) config.description = description
  else delete config.description

  if (input.type === 'embedding') {
    config.dimensions = input.dimensions
    delete config.compatibilities
    delete config.context_window
    delete config.reasoning_efforts
    delete config.thinking_mode
    return config
  }

  delete config.dimensions
  config.compatibilities = input.compatibilities
  if (input.contextWindow) config.context_window = input.contextWindow
  else delete config.context_window
  return config
}
