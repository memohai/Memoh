import { createGateway as createAiGateway } from 'ai'
import { createOpenAI } from '@ai-sdk/openai'
import { createAnthropic } from '@ai-sdk/anthropic'
import { createGoogleGenerativeAI } from '@ai-sdk/google'
import { ClientType, ModelConfig } from './types'

export const createModel = (model: ModelConfig) => {
  const apiKey = model.apiKey.toLowerCase().trim()
  const baseURL = model.baseUrl.toLowerCase().trim()
  const modelId = model.modelId.toLowerCase().trim()
  const clients = {
    [ClientType.OpenAI]: createOpenAI,
    [ClientType.OpenAICompatible]: createOpenAI,
    [ClientType.Anthropic]: createAnthropic,
    [ClientType.Google]: createGoogleGenerativeAI,
  }
  return (clients[model.clientType] ?? createAiGateway)({
    apiKey,
    baseURL,
  })(modelId)
}