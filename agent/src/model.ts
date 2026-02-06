import { createGateway as createAiGateway } from 'ai'
import { createOpenAI } from '@ai-sdk/openai'
import { createAnthropic } from '@ai-sdk/anthropic'
import { createGoogleGenerativeAI } from '@ai-sdk/google'
import { ClientType, ModelConfig } from './types'

export const createModel = (model: ModelConfig) => {
  const apiKey = model.apiKey.trim()
  const baseURL = model.baseUrl.trim()
  const modelId = model.modelId.trim()

  switch (model.clientType) {
    case ClientType.OpenAI:
    case ClientType.OpenAICompatible: {
      const provider = createOpenAI({ apiKey, baseURL })
      // Use .chat() to call /chat/completions (not /responses which only OpenAI supports)
      return provider.chat(modelId)
    }
    case ClientType.Anthropic:
      return createAnthropic({ apiKey, baseURL })(modelId)
    case ClientType.Google:
      return createGoogleGenerativeAI({ apiKey, baseURL })(modelId)
    default:
      return createAiGateway({ apiKey, baseURL })(modelId)
  }
}