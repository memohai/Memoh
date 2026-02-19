import { createOpenAI } from '@ai-sdk/openai'
import { createAnthropic } from '@ai-sdk/anthropic'
import { createGoogleGenerativeAI } from '@ai-sdk/google'
import { ClientType, ModelConfig } from './types'

export const createModel = (model: ModelConfig) => {
  const apiKey = model.apiKey.trim()
  const baseURL = model.baseUrl.trim()
  const modelId = model.modelId.trim()

  switch (model.clientType) {
    case ClientType.OpenAIResponses:
      return createOpenAI({ apiKey, baseURL })(modelId)
    case ClientType.OpenAICompletions: {
      return createOpenAI({ apiKey, baseURL }).chat(modelId)
    }
    case ClientType.AnthropicMessages:
      return createAnthropic({ apiKey, baseURL })(modelId)
    case ClientType.GoogleGenerativeAI:
      return createGoogleGenerativeAI({ apiKey, baseURL })(modelId)
    default:
      return createOpenAI({ apiKey, baseURL }).chat(modelId)
  }
}
