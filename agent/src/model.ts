import { createGateway as createAiGateway } from 'ai'
import { createOpenAI } from '@ai-sdk/openai'
import { createAnthropic } from '@ai-sdk/anthropic'
import { createGoogleGenerativeAI } from '@ai-sdk/google'
import { createAzure } from '@ai-sdk/azure'
import { createAmazonBedrock } from '@ai-sdk/amazon-bedrock'
import { createMistral } from '@ai-sdk/mistral'
import { createXai } from '@ai-sdk/xai'
import { ClientType, ModelConfig } from './types'

export const createModel = (model: ModelConfig) => {
  const apiKey = model.apiKey.trim()
  const baseURL = model.baseUrl.trim()
  const modelId = model.modelId.trim()

  switch (model.clientType) {
    case ClientType.OpenAI:
    case ClientType.OpenAICompat:
    case ClientType.Ollama:
    case ClientType.Dashscope: {
      // All OpenAI-compatible providers use .chat() for /chat/completions
      const provider = createOpenAI({ apiKey, baseURL })
      return provider.chat(modelId)
    }
    case ClientType.Anthropic:
      return createAnthropic({ apiKey, baseURL })(modelId)
    case ClientType.Google:
      return createGoogleGenerativeAI({ apiKey, baseURL })(modelId)
    case ClientType.Azure:
      return createAzure({ apiKey, baseURL })(modelId)
    case ClientType.Bedrock: {
      // Bedrock uses AWS credentials; apiKey as accessKeyId, metadata for secretAccessKey
      // Falls back to AWS default credential chain if not provided
      const opts: Record<string, string> = {}
      if (baseURL) opts.region = baseURL
      if (apiKey) opts.accessKeyId = apiKey
      return createAmazonBedrock(opts)(modelId)
    }
    case ClientType.Mistral:
      return createMistral({ apiKey, baseURL: baseURL || undefined })(modelId)
    case ClientType.XAI:
      return createXai({ apiKey, baseURL: baseURL || undefined })(modelId)
    default:
      return createAiGateway({ apiKey, baseURL })(modelId)
  }
}